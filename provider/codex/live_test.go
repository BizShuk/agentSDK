package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// liveHandler drives the server side of one fake Realtime API connection.
// assert (not require) throughout: it runs off the test goroutine.
type liveHandler func(t *testing.T, ctx context.Context, c *websocket.Conn, r *http.Request)

func liveTestServer(t *testing.T, handler liveHandler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		handler(t, ctx, c, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func readClientEvent(t *testing.T, ctx context.Context, c *websocket.Conn) realtimeClientEvent {
	_, data, err := c.Read(ctx)
	if !assert.NoError(t, err) {
		return realtimeClientEvent{}
	}
	var event realtimeClientEvent
	assert.NoError(t, json.Unmarshal(data, &event))
	return event
}

func writeServerEvent(t *testing.T, ctx context.Context, c *websocket.Conn, event any) {
	data, err := json.Marshal(event)
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, c.Write(ctx, websocket.MessageText, data))
}

// completeHandshake consumes the session.update and acknowledges it, with a
// session.created first the way the real endpoint greets.
func completeHandshake(t *testing.T, ctx context.Context, c *websocket.Conn) realtimeClientEvent {
	writeServerEvent(t, ctx, c, realtimeServerEvent{Type: "session.created"})
	update := readClientEvent(t, ctx, c)
	writeServerEvent(t, ctx, c, realtimeServerEvent{Type: "session.updated"})
	return update
}

func TestConnectLiveTextDialogue(t *testing.T) {
	server := liveTestServer(t, func(t *testing.T, ctx context.Context, c *websocket.Conn, r *http.Request) {
		assert.Equal(t, "Bearer secret-key", r.Header.Get("Authorization"))
		assert.Equal(t, DefaultLiveModel, r.URL.Query().Get("model"))

		update := completeHandshake(t, ctx, c)
		if !assert.NotNil(t, update.Session) {
			return
		}
		assert.Equal(t, "session.update", update.Type)
		assert.Equal(t, "realtime", update.Session.Type)
		assert.Equal(t, []string{"text"}, update.Session.OutputModalities)
		assert.Equal(t, "stay brief", update.Session.Instructions)

		item := readClientEvent(t, ctx, c)
		if !assert.NotNil(t, item.Item) {
			return
		}
		assert.Equal(t, "conversation.item.create", item.Type)
		assert.Equal(t, "user", item.Item.Role)
		assert.Equal(t, "input_text", item.Item.Content[0].Type)
		assert.Equal(t, "hello", item.Item.Content[0].Text)
		assert.Equal(t, "response.create", readClientEvent(t, ctx, c).Type)

		writeServerEvent(t, ctx, c, realtimeServerEvent{
			Type: "response.output_text.delta", Delta: "wor",
		})
		writeServerEvent(t, ctx, c, realtimeServerEvent{
			Type: "response.output_text.delta", Delta: "ld",
		})
		writeServerEvent(t, ctx, c, realtimeServerEvent{Type: "response.done"})
		_, _, _ = c.Read(ctx) // wait for the client close handshake
	})

	connector, err := NewLive(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "secret-key"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	session, err := connector.ConnectLive(ctx, provider.LiveRequest{System: "stay brief"})
	require.NoError(t, err)
	defer session.Close()

	require.NoError(t, session.SendText(ctx, "hello"))

	first, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "wor", first.Text)

	second, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ld", second.Text)

	done, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.True(t, done.TurnComplete)
}

func TestConnectLiveAudioEventMapping(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	server := liveTestServer(t, func(t *testing.T, ctx context.Context, c *websocket.Conn, r *http.Request) {
		assert.Equal(t, "gpt-realtime-mini", r.URL.Query().Get("model"))

		update := completeHandshake(t, ctx, c)
		if !assert.NotNil(t, update.Session) {
			return
		}
		assert.Equal(t, []string{"audio"}, update.Session.OutputModalities)
		assert.Equal(t, "marin", update.Session.Audio.Output.Voice)
		assert.Equal(t, DefaultInputTranscriptionModel,
			update.Session.Audio.Input.Transcription.Model)

		chunk := readClientEvent(t, ctx, c)
		assert.Equal(t, "input_audio_buffer.append", chunk.Type)
		sent, err := base64.StdEncoding.DecodeString(chunk.Audio)
		assert.NoError(t, err)
		assert.Equal(t, []byte("mic-bytes"), sent)

		writeServerEvent(t, ctx, c, realtimeServerEvent{Type: "input_audio_buffer.speech_started"})
		writeServerEvent(t, ctx, c, realtimeServerEvent{
			Type:       "conversation.item.input_audio_transcription.completed",
			Transcript: "hello there",
		})
		writeServerEvent(t, ctx, c, realtimeServerEvent{
			Type:  "response.output_audio.delta",
			Delta: base64.StdEncoding.EncodeToString(pcm),
		})
		writeServerEvent(t, ctx, c, realtimeServerEvent{
			Type: "response.output_audio_transcript.delta", Delta: "hi",
		})
		writeServerEvent(t, ctx, c, realtimeServerEvent{Type: "response.done"})
		_, _, _ = c.Read(ctx)
	})

	connector, err := NewLive(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "secret-key"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	session, err := connector.ConnectLive(ctx, provider.LiveRequest{
		Model:            "gpt-realtime-mini",
		ResponseModality: provider.LIVE_MODALITY_AUDIO,
		Voice:            "marin",
		TranscribeInput:  true,
	})
	require.NoError(t, err)
	defer session.Close()

	require.NoError(t, session.SendAudio(ctx, []byte("mic-bytes")))

	interrupted, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.True(t, interrupted.Interrupted)

	transcript, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "hello there", transcript.InputTranscript)

	audio, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, pcm, audio.Audio)
	assert.Equal(t, "audio/pcm;rate=24000", audio.AudioMIME)

	outTranscript, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "hi", outTranscript.OutputTranscript)

	done, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.True(t, done.TurnComplete)
}

func TestConnectLiveServerErrorEventFailsHandshake(t *testing.T) {
	server := liveTestServer(t, func(t *testing.T, ctx context.Context, c *websocket.Conn, r *http.Request) {
		readClientEvent(t, ctx, c)
		writeServerEvent(t, ctx, c, realtimeServerEvent{
			Type:  "error",
			Error: &realtimeAPIError{Message: "invalid voice"},
		})
	})

	connector, err := NewLive(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "secret-key"},
	})
	require.NoError(t, err)

	_, err = connector.ConnectLive(context.Background(), provider.LiveRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid voice")
}

func TestConnectLiveRejectsUnsupportedFields(t *testing.T) {
	connector, err := NewLive(provider.ResolvedConfig{
		Auth: core.Auth{APIKey: "secret-key"},
	})
	require.NoError(t, err)

	_, err = connector.ConnectLive(context.Background(), provider.LiveRequest{
		ThinkingLevel: "high",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "thinking level is not supported")

	_, err = connector.ConnectLive(context.Background(), provider.LiveRequest{
		Translation: &provider.LiveTranslation{TargetLanguage: "es"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "translation is not supported")
}

func TestConnectLiveRejectsOAuthCredential(t *testing.T) {
	connector, err := NewLive(provider.ResolvedConfig{
		Auth: core.Auth{Bearer: "chatgpt-oauth-token"},
	})
	require.NoError(t, err)

	_, err = connector.ConnectLive(context.Background(), provider.LiveRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not accept ChatGPT OAuth tokens")

	missing, err := NewLive(provider.ResolvedConfig{})
	require.NoError(t, err)
	_, err = missing.ConnectLive(context.Background(), provider.LiveRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential is required")
}

func TestRegistryNewLiveUsesLiveBaseURLEnv(t *testing.T) {
	env := map[string]string{
		APIKeyEnvVar:      "env-key",
		LiveBaseURLEnvVar: "wss://realtime.example",
	}
	connector, err := provider.NewLive("codex", provider.Options{
		CredentialKind: core.CREDENTIAL_KIND_APIKEY,
		LookupEnv:      func(key string) string { return env[key] },
	})
	require.NoError(t, err)

	live, ok := connector.(*LiveProvider)
	require.True(t, ok)
	assert.Equal(t, "wss://realtime.example", live.baseURL)
	assert.Equal(t, "env-key", live.auth.APIKey)
}
