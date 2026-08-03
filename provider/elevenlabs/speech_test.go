package elevenlabs_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/elevenlabs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpeechGeneratorMatchesRequestContract(t *testing.T) {
	var (
		capturedMethod string
		capturedPath   string
		capturedQuery  string
		capturedKey    string
		capturedAuth   string
		capturedBody   map[string]any
		decodeErr      error
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query().Get("output_format")
		capturedKey = r.Header.Get("xi-api-key")
		capturedAuth = r.Header.Get("Authorization")
		decodeErr = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3-audio-bytes"))
	}))
	t.Cleanup(server.Close)

	generator, err := elevenlabs.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)

	result, err := generator.GenerateSpeech(context.Background(), provider.SpeechRequest{
		Model:        "eleven_multilingual_v2",
		Text:         "早安，新加坡",
		Voice:        "voice-42",
		OutputFormat: "pcm_16000",
		VoiceSetting: provider.VoiceSetting{
			Stability:  0.4,
			Similarity: 0.8,
			Style:      0.2,
			Speed:      1.1,
		},
	})
	require.NoError(t, err)
	require.NoError(t, decodeErr)

	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "/v1/text-to-speech/voice-42", capturedPath)
	assert.Equal(t, "pcm_16000", capturedQuery)
	assert.Equal(t, "test-key", capturedKey)
	assert.Empty(t, capturedAuth, "ElevenLabs reads xi-api-key, not Authorization")

	assert.Equal(t, "早安，新加坡", capturedBody["text"])
	assert.Equal(t, "eleven_multilingual_v2", capturedBody["model_id"])
	settings, ok := capturedBody["voice_settings"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 0.4, settings["stability"], 0.0001)
	assert.InDelta(t, 0.8, settings["similarity_boost"], 0.0001)
	assert.InDelta(t, 0.2, settings["style"], 0.0001)
	assert.InDelta(t, 1.1, settings["speed"], 0.0001)

	assert.Equal(t, []byte("ID3-audio-bytes"), result.Audio.Bytes)
	assert.Equal(t, "pcm_16000", result.Audio.Format)
	assert.Zero(t, result.Info, "ElevenLabs reports no clip metadata on this route")
}

func TestSpeechGeneratorAppliesDefaults(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		_, _ = w.Write([]byte("audio"))
	}))
	t.Cleanup(server.Close)

	generator, err := elevenlabs.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)

	result, err := generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.NoError(t, err)

	assert.Equal(t, "/v1/text-to-speech/"+elevenlabs.DefaultVoiceID, capturedPath)
	assert.Empty(t, capturedQuery, "an unset output format sends no query parameter")
	assert.Equal(t, elevenlabs.DefaultSpeechModel, capturedBody["model_id"])
	assert.NotContains(t, capturedBody, "voice_settings",
		"a zero VoiceSetting must not send four zeros")
	assert.Equal(t, elevenlabs.DefaultSpeechOutputFormat, result.Audio.Format)
}

func TestSpeechGeneratorUsesConstructionModel(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		_, _ = w.Write([]byte("audio"))
	}))
	t.Cleanup(server.Close)

	generator, err := elevenlabs.NewSpeech(provider.ResolvedConfig{
		BaseURL: server.URL,
		Model:   "eleven_turbo_v2_5",
		Auth:    core.Auth{APIKey: "test-key"},
	})
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.NoError(t, err)
	assert.Equal(t, "eleven_turbo_v2_5", capturedBody["model_id"])
}

func TestSpeechGeneratorRejectsEmptyText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("an invalid request must not reach the upstream server")
	}))
	t.Cleanup(server.Close)

	generator, err := elevenlabs.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(context.Background(), provider.SpeechRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "speech text is required")
}

func TestSpeechGeneratorRequiresCredential(t *testing.T) {
	generator, err := elevenlabs.NewSpeech(provider.ResolvedConfig{
		BaseURL: "https://example.invalid",
	})
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential is required")
}

func TestSpeechGeneratorReturnsTypedAPIError(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantCode    string
		wantMessage string
	}{
		{
			name:        "object detail",
			status:      http.StatusUnauthorized,
			body:        `{"detail":{"status":"invalid_api_key","message":"API key is invalid"}}`,
			wantCode:    "invalid_api_key",
			wantMessage: "API key is invalid",
		},
		{
			name:        "string detail",
			status:      http.StatusNotFound,
			body:        `{"detail":"voice not found"}`,
			wantMessage: "voice not found",
		},
		{
			name:        "unparseable body",
			status:      http.StatusBadGateway,
			body:        `<html>gateway</html>`,
			wantMessage: "upstream request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Request-ID", "request-speech-1")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(server.Close)

			generator, err := elevenlabs.NewSpeech(speechConfig(server.URL))
			require.NoError(t, err)
			_, err = generator.GenerateSpeech(
				context.Background(),
				provider.SpeechRequest{Text: "hello"},
			)
			require.Error(t, err)

			var apiErr *provider.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, "elevenlabs", apiErr.Provider)
			assert.Equal(t, "generate speech", apiErr.Operation)
			assert.Equal(t, tt.status, apiErr.StatusCode)
			assert.Equal(t, tt.wantCode, apiErr.Code)
			assert.Equal(t, tt.wantMessage, apiErr.Message)
			assert.Equal(t, "request-speech-1", apiErr.RequestID)
			assert.NotContains(t, err.Error(), "test-key",
				"an error must never carry the credential")
		})
	}
}

func TestSpeechErrorMessageIsBounded(t *testing.T) {
	// A vendor that returns a megabyte of detail must not turn one failed
	// request into a megabyte log line.
	huge, err := json.Marshal(map[string]string{"detail": strings.Repeat("x", 200000)})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(huge)
	}))
	t.Cleanup(server.Close)

	generator, err := elevenlabs.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.Error(t, err)

	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Len(t, []rune(apiErr.Message), 512)
	assert.Less(t, len(err.Error()), 1000)
}

func TestSpeechStreamReturnsReadableBody(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query().Get("output_format")
		_, _ = io.WriteString(w, "chunk-1chunk-2")
	}))
	t.Cleanup(server.Close)

	generator, err := elevenlabs.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)

	body, err := generator.StreamSpeech(context.Background(), provider.SpeechRequest{
		Text:         "hello",
		Voice:        "voice-42",
		OutputFormat: "mp3_44100_128",
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, body.Close()) })

	audio, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "chunk-1chunk-2", string(audio))
	assert.Equal(t, "/v1/text-to-speech/voice-42/stream", capturedPath)
	assert.Equal(t, "mp3_44100_128", capturedQuery)
}

func TestSpeechStreamReturnsTypedAPIErrorAndNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"detail":{"status":"too_many_requests","message":"slow down"}}`)
	}))
	t.Cleanup(server.Close)

	generator, err := elevenlabs.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)

	body, err := generator.StreamSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.Error(t, err)
	assert.Nil(t, body, "a failed stream must not hand back an error document to play")

	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "stream speech", apiErr.Operation)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, "too_many_requests", apiErr.Code)
	assert.Equal(t, "slow down", apiErr.Message)
}

func TestSpeechGeneratorRequestAuthOverridesConstructionAuth(t *testing.T) {
	var capturedKey string
	var capturedTrace string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("xi-api-key")
		capturedTrace = r.Header.Get("X-Trace-ID")
		_, _ = w.Write([]byte("audio"))
	}))
	t.Cleanup(server.Close)

	generator, err := elevenlabs.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(context.Background(), provider.SpeechRequest{
		Text: "hello",
		Auth: core.Auth{
			APIKey:  "request-key",
			Headers: map[string]string{"X-Trace-ID": "request-trace"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "request-key", capturedKey)
	assert.Equal(t, "request-trace", capturedTrace)
}

func TestSpeechGeneratorHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("canceled request must not reach the upstream server")
	}))
	t.Cleanup(server.Close)

	generator, err := elevenlabs.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = generator.GenerateSpeech(ctx, provider.SpeechRequest{Text: "hello"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestNewSpeechResolvesRegistryEnvironment(t *testing.T) {
	var capturedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("xi-api-key")
		_, _ = w.Write([]byte("audio"))
	}))
	t.Cleanup(server.Close)

	env := map[string]string{
		"ELEVENLABS_API_KEY":  "env-key",
		"ELEVENLABS_BASE_URL": server.URL,
	}
	generator, err := provider.NewSpeech("elevenlabs", provider.Options{
		LookupEnv: func(key string) string { return env[key] },
	})
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.NoError(t, err)
	assert.Equal(t, "env-key", capturedKey)
}

func speechConfig(baseURL string) provider.ResolvedConfig {
	return provider.ResolvedConfig{
		BaseURL: baseURL,
		Auth:    core.Auth{APIKey: "test-key"},
	}
}
