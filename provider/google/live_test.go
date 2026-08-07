package google

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

// liveHandler drives the server side of one fake BidiGenerateContent
// connection. assert (not require) throughout: it runs off the test
// goroutine.
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

func readClientMessage(t *testing.T, ctx context.Context, c *websocket.Conn) liveClientMessage {
	_, data, err := c.Read(ctx)
	if !assert.NoError(t, err) {
		return liveClientMessage{}
	}
	var msg liveClientMessage
	assert.NoError(t, json.Unmarshal(data, &msg))
	return msg
}

func writeServerMessage(t *testing.T, ctx context.Context, c *websocket.Conn, msg any) {
	data, err := json.Marshal(msg)
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, c.Write(ctx, websocket.MessageText, data))
}

func TestConnectLiveTextDialogue(t *testing.T) {
	server := liveTestServer(t, func(t *testing.T, ctx context.Context, c *websocket.Conn, r *http.Request) {
		assert.Equal(t, "secret-key", r.URL.Query().Get("key"))

		setup := readClientMessage(t, ctx, c)
		if !assert.NotNil(t, setup.Setup) {
			return
		}
		assert.Equal(t, "models/"+DefaultLiveModel, setup.Setup.Model)
		assert.Equal(t, []string{"TEXT"}, setup.Setup.GenerationConfig.ResponseModalities)
		assert.Equal(t, "high",
			setup.Setup.GenerationConfig.ThinkingConfig.ThinkingLevel)
		assert.Equal(t, "stay brief",
			setup.Setup.SystemInstruction.Parts[0].Text)
		writeServerMessage(t, ctx, c, liveServerMessage{SetupComplete: &struct{}{}})

		turn := readClientMessage(t, ctx, c)
		if !assert.NotNil(t, turn.ClientContent) {
			return
		}
		assert.True(t, turn.ClientContent.TurnComplete)
		assert.Equal(t, "hello", turn.ClientContent.Turns[0].Parts[0].Text)

		writeServerMessage(t, ctx, c, liveServerMessage{
			ServerContent: &liveServerContent{
				ModelTurn: &liveContent{Parts: []livePart{{Text: "wor"}}},
			},
		})
		writeServerMessage(t, ctx, c, liveServerMessage{
			ServerContent: &liveServerContent{
				ModelTurn:    &liveContent{Parts: []livePart{{Text: "ld"}}},
				TurnComplete: true,
			},
		})
		_, _, _ = c.Read(ctx) // wait for the client close handshake
	})

	connector, err := NewLive(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "secret-key"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	session, err := connector.ConnectLive(ctx, provider.LiveRequest{
		System:        "stay brief",
		ThinkingLevel: "high",
	})
	require.NoError(t, err)
	defer session.Close()

	require.NoError(t, session.SendText(ctx, "hello"))

	first, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "wor", first.Text)
	assert.False(t, first.TurnComplete)

	second, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ld", second.Text)
	assert.True(t, second.TurnComplete)
	assert.Equal(t, core.UnpricedCost(), second.Cost)
}

func TestConnectLiveAudioEventFolding(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	server := liveTestServer(t, func(t *testing.T, ctx context.Context, c *websocket.Conn, r *http.Request) {
		setup := readClientMessage(t, ctx, c)
		if !assert.NotNil(t, setup.Setup) {
			return
		}
		assert.Equal(t, []string{"AUDIO"}, setup.Setup.GenerationConfig.ResponseModalities)
		assert.Equal(t, "Puck",
			setup.Setup.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName)
		assert.NotNil(t, setup.Setup.OutputAudioTranscription)
		writeServerMessage(t, ctx, c, liveServerMessage{SetupComplete: &struct{}{}})

		chunk := readClientMessage(t, ctx, c)
		if !assert.NotNil(t, chunk.RealtimeInput) {
			return
		}
		assert.Equal(t, "audio/pcm;rate=16000", chunk.RealtimeInput.Audio.MIMEType)
		sent, err := base64.StdEncoding.DecodeString(chunk.RealtimeInput.Audio.Data)
		assert.NoError(t, err)
		assert.Equal(t, []byte("mic-bytes"), sent)

		writeServerMessage(t, ctx, c, liveServerMessage{
			ServerContent: &liveServerContent{
				ModelTurn: &liveContent{Parts: []livePart{{
					InlineData: &liveBlob{
						MIMEType: "audio/pcm;rate=24000",
						Data:     base64.StdEncoding.EncodeToString(pcm),
					},
				}}},
				OutputTranscription: &liveTranscription{Text: "hi there"},
				TurnComplete:        true,
			},
		})
		_, _, _ = c.Read(ctx)
	})

	connector, err := NewLive(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "secret-key"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	session, err := connector.ConnectLive(ctx, provider.LiveRequest{
		ResponseModality: provider.LIVE_MODALITY_AUDIO,
		Voice:            "Puck",
		TranscribeOutput: true,
	})
	require.NoError(t, err)
	defer session.Close()

	require.NoError(t, session.SendAudio(ctx, []byte("mic-bytes")))

	event, err := session.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, pcm, event.Audio)
	assert.Equal(t, "audio/pcm;rate=24000", event.AudioMIME)
	assert.Equal(t, "hi there", event.OutputTranscript)
	assert.True(t, event.TurnComplete)
}

func TestConnectLiveBearerUsesAuthorizationHeader(t *testing.T) {
	server := liveTestServer(t, func(t *testing.T, ctx context.Context, c *websocket.Conn, r *http.Request) {
		assert.Equal(t, "Bearer oauth-token", r.Header.Get("Authorization"))
		assert.Empty(t, r.URL.Query().Get("key"))
		readClientMessage(t, ctx, c)
		writeServerMessage(t, ctx, c, liveServerMessage{SetupComplete: &struct{}{}})
		_, _, _ = c.Read(ctx)
	})

	connector, err := NewLive(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{Bearer: "oauth-token"},
	})
	require.NoError(t, err)

	session, err := connector.ConnectLive(context.Background(), provider.LiveRequest{})
	require.NoError(t, err)
	assert.NoError(t, session.Close())
}

func TestConnectLiveRequiresCredential(t *testing.T) {
	connector, err := NewLive(provider.ResolvedConfig{BaseURL: "http://127.0.0.1:1"})
	require.NoError(t, err)
	_, err = connector.ConnectLive(context.Background(), provider.LiveRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential is required")
}

func TestRegistryNewLiveUsesLiveBaseURLEnv(t *testing.T) {
	env := map[string]string{
		APIKeyEnvVar:      "env-key",
		LiveBaseURLEnvVar: "wss://live.example",
	}
	connector, err := provider.NewLive("google", provider.Options{
		LookupEnv: func(key string) string { return env[key] },
	})
	require.NoError(t, err)

	live, ok := connector.(*LiveProvider)
	require.True(t, ok)
	assert.Equal(t, "wss://live.example", live.baseURL)
	assert.Equal(t, "env-key", live.auth.APIKey)
}
