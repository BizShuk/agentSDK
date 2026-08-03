package minimax_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/minimax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpeechGeneratorMatchesRequestContract(t *testing.T) {
	var (
		capturedMethod string
		capturedPath   string
		capturedAuth   string
		capturedBody   map[string]any
		decodeErr      error
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		decodeErr = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"data":{"audio":"`+hex.EncodeToString([]byte("audio-bytes"))+`","status":2},
			"extra_info":{
				"audio_length":25364,
				"audio_sample_rate":16000,
				"audio_size":813651,
				"audio_bitrate":128000,
				"audio_channel":1
			},
			"trace_id":"trace-speech-1",
			"base_resp":{"status_code":0,"status_msg":"success"}
		}`)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)

	result, err := generator.GenerateSpeech(context.Background(), provider.SpeechRequest{
		Model:        "speech-02-turbo",
		Text:         "早安，新加坡",
		Voice:        "female-shaonv",
		OutputFormat: "pcm_16000",
		VoiceSetting: provider.VoiceSetting{Speed: 1.2},
	})
	require.NoError(t, err)
	require.NoError(t, decodeErr)

	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "/v1/t2a_v2", capturedPath)
	assert.Equal(t, "Bearer test-key", capturedAuth)
	assert.Equal(t, "speech-02-turbo", capturedBody["model"])
	assert.Equal(t, "早安，新加坡", capturedBody["text"])
	assert.Equal(t, false, capturedBody["stream"])

	voiceSetting, ok := capturedBody["voice_setting"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "female-shaonv", voiceSetting["voice_id"])
	assert.InDelta(t, 1.2, voiceSetting["speed"], 0.0001)

	audioSetting, ok := capturedBody["audio_setting"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "pcm", audioSetting["format"])
	assert.Equal(t, float64(16000), audioSetting["sample_rate"])

	assert.Equal(t, []byte("audio-bytes"), result.Audio.Bytes)
	assert.Equal(t, "pcm", result.Audio.Format)
	assert.Equal(t, provider.SpeechInfo{
		DurationMs: 25364,
		SampleRate: 16000,
		Channels:   1,
		Bitrate:    128000,
		SizeBytes:  813651,
	}, result.Info)
}

func TestSpeechGeneratorAppliesDefaults(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, speechSuccessBody)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	result, err := generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.NoError(t, err)

	assert.Equal(t, "speech-02-hd", capturedBody["model"])
	voiceSetting, ok := capturedBody["voice_setting"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "male-qn-qingse", voiceSetting["voice_id"])
	assert.NotContains(t, voiceSetting, "speed",
		"a zero VoiceSetting must not pin the voice to speed 0")
	assert.NotContains(t, capturedBody, "audio_setting",
		"an unset output format leaves the encoding to the provider")
	assert.Equal(t, "mp3", result.Audio.Format)
}

func TestSpeechGeneratorUsesConstructionModel(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, speechSuccessBody)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewSpeech(provider.ResolvedConfig{
		BaseURL: server.URL,
		Model:   "speech-2.5-hd-preview",
		Auth:    core.Auth{APIKey: "test-key"},
	})
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.NoError(t, err)
	assert.Equal(t, "speech-2.5-hd-preview", capturedBody["model"])
}

func TestSpeechOutputFormatParsing(t *testing.T) {
	tests := []struct {
		name           string
		outputFormat   string
		wantFormat     any
		wantSampleRate any
		wantAssetLabel string
	}{
		{
			name:           "plain container",
			outputFormat:   "mp3",
			wantFormat:     "mp3",
			wantAssetLabel: "mp3",
		},
		{
			name:           "composite splits into format and sample rate",
			outputFormat:   "pcm_16000",
			wantFormat:     "pcm",
			wantSampleRate: float64(16000),
			wantAssetLabel: "pcm",
		},
		{
			name:           "flac",
			outputFormat:   "flac",
			wantFormat:     "flac",
			wantAssetLabel: "flac",
		},
		{
			name:           "unknown label passes through verbatim",
			outputFormat:   "opus_ogg",
			wantFormat:     "opus_ogg",
			wantAssetLabel: "opus_ogg",
		},
		{
			name:           "uppercase normalizes",
			outputFormat:   "MP3",
			wantFormat:     "mp3",
			wantAssetLabel: "mp3",
		},
		{
			name:           "unset sends no audio setting",
			outputFormat:   "",
			wantAssetLabel: "mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&capturedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, speechSuccessBody)
			}))
			t.Cleanup(server.Close)

			generator, err := minimax.NewSpeech(speechConfig(server.URL))
			require.NoError(t, err)
			result, err := generator.GenerateSpeech(context.Background(), provider.SpeechRequest{
				Text:         "hello",
				OutputFormat: tt.outputFormat,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantAssetLabel, result.Audio.Format)

			audioSetting, ok := capturedBody["audio_setting"].(map[string]any)
			if tt.wantFormat == nil {
				assert.False(t, ok, "no audio_setting expected")
				return
			}
			require.True(t, ok)
			assert.Equal(t, tt.wantFormat, audioSetting["format"])
			if tt.wantSampleRate == nil {
				assert.NotContains(t, audioSetting, "sample_rate")
				return
			}
			assert.Equal(t, tt.wantSampleRate, audioSetting["sample_rate"])
		})
	}
}

func TestSpeechGeneratorReturnsSemanticAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"base_resp":{"status_code":1004,"status_msg":"authentication failed"}
		}`)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.Error(t, err)

	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "minimax", apiErr.Provider)
	assert.Equal(t, "generate speech", apiErr.Operation)
	assert.Equal(t, http.StatusOK, apiErr.StatusCode)
	assert.Equal(t, "1004", apiErr.Code)
	assert.Equal(t, "authentication failed", apiErr.Message)
	assert.NotContains(t, err.Error(), "test-key")
}

func TestSpeechGeneratorReturnsHTTPAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "request-speech-1")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{
			"base_resp":{"status_code":1002,"status_msg":"rate limited"}
		}`)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.Error(t, err)

	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	assert.Equal(t, "1002", apiErr.Code)
	assert.Equal(t, "rate limited", apiErr.Message)
	assert.Equal(t, "request-speech-1", apiErr.RequestID)
}

func TestSpeechGeneratorRejectsUndecodableAudio(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "invalid hex",
			body:    `{"data":{"audio":"zzzz","status":2},"base_resp":{"status_code":0}}`,
			wantErr: "decode audio",
		},
		{
			name:    "missing audio",
			body:    `{"data":{"status":2},"base_resp":{"status_code":0}}`,
			wantErr: "no audio",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(server.Close)

			generator, err := minimax.NewSpeech(speechConfig(server.URL))
			require.NoError(t, err)
			_, err = generator.GenerateSpeech(
				context.Background(),
				provider.SpeechRequest{Text: "hello"},
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSpeechGeneratorRejectsEmptyText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("an invalid request must not reach the upstream server")
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(context.Background(), provider.SpeechRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "speech text is required")
}

func TestSpeechGeneratorRequiresCredential(t *testing.T) {
	generator, err := minimax.NewSpeech(provider.ResolvedConfig{
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

func TestSpeechGeneratorRequestAuthOverridesConstructionAuth(t *testing.T) {
	var capturedAuth string
	var capturedTrace string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedTrace = r.Header.Get("X-Trace-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, speechSuccessBody)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(context.Background(), provider.SpeechRequest{
		Text: "hello",
		Auth: core.Auth{
			Bearer:  "request-token",
			Headers: map[string]string{"X-Trace-ID": "request-trace"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer request-token", capturedAuth)
	assert.Equal(t, "request-trace", capturedTrace)
}

func TestSpeechGeneratorHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("canceled request must not reach the upstream server")
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = generator.GenerateSpeech(ctx, provider.SpeechRequest{Text: "hello"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestNewSpeechUsesSpeechSpecificBaseURLEnv(t *testing.T) {
	// MINIMAX_SPEECH_BASE_URL replaces the base wholesale; the chat surface's
	// MINIMAX_BASE_URL must not be consulted for t2a_v2 at all.
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, speechSuccessBody)
	}))
	t.Cleanup(server.Close)

	env := map[string]string{
		"MINIMAX_API_KEY":         "test-key",
		"MINIMAX_BASE_URL":        "https://chat.example.invalid/anthropic",
		"MINIMAX_SPEECH_BASE_URL": server.URL,
	}
	generator, err := provider.NewSpeech("minimax", provider.Options{
		LookupEnv: func(key string) string { return env[key] },
	})
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.NoError(t, err)
	assert.Equal(t, "/v1/t2a_v2", capturedPath)
}

func TestNewSpeechIgnoresTheChatBaseURLEnv(t *testing.T) {
	// Without MINIMAX_SPEECH_BASE_URL the base falls back to the public
	// default, exactly as music does: a chat-surface env must never silently
	// become the t2a_v2 host.
	entry, ok := provider.Lookup("minimax")
	require.True(t, ok)
	require.Equal(t, "MINIMAX_SPEECH_BASE_URL", entry.Metadata.SpeechBaseURLEnv)

	env := map[string]string{
		"MINIMAX_API_KEY":  "test-key",
		"MINIMAX_BASE_URL": "https://chat.example.invalid/anthropic",
	}
	resolved, err := provider.Options{
		LookupEnv: func(key string) string { return env[key] },
	}.Resolve(speechMetadata(entry.Metadata))
	require.NoError(t, err)
	assert.Empty(t, resolved.BaseURL, "the chat base URL must not reach speech")
}

func TestNewSpeechTrimsTheAnthropicCompatSuffix(t *testing.T) {
	// The fallback for a caller that hands over the chat base URL explicitly
	// instead of setting MINIMAX_SPEECH_BASE_URL: t2a_v2 lives one path
	// segment above the Anthropic-compat surface.
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, speechSuccessBody)
	}))
	t.Cleanup(server.Close)

	generator, err := provider.NewSpeech("minimax", provider.Options{
		BaseURL:   server.URL + anthropicCompatPath,
		APIKey:    "test-key",
		LookupEnv: func(string) string { return "" },
	})
	require.NoError(t, err)
	_, err = generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "hello"},
	)
	require.NoError(t, err)
	assert.Equal(t, "/v1/t2a_v2", capturedPath)
}

// anthropicCompatPath mirrors the suffix DefaultBaseURL carries; the adapter
// keeps its own unexported copy.
const anthropicCompatPath = "/anthropic"

// speechMetadata applies the same base URL override provider.NewSpeech does,
// so the test asserts on the registry's resolution rather than on a
// constructed client.
func speechMetadata(metadata provider.Metadata) provider.Metadata {
	if metadata.SpeechBaseURLEnv != "" {
		metadata.BaseURLEnv = metadata.SpeechBaseURLEnv
	}
	return metadata
}

func TestSpeechCatalogListsTheVoiceModels(t *testing.T) {
	ids := map[string]bool{}
	for _, spec := range minimax.DefaultCatalog() {
		ids[spec.ID] = true
	}
	for _, want := range []string{
		"speech-2.5-hd-preview",
		"speech-2.5-turbo-preview",
		"speech-02-hd",
		"speech-02-turbo",
		"speech-01-hd",
		"speech-01-turbo",
	} {
		assert.Truef(t, ids[want], "catalog is missing speech model %q", want)
	}
}

const speechSuccessBody = `{
	"data":{"audio":"6d7033","status":2},
	"base_resp":{"status_code":0}
}`

func speechConfig(baseURL string) provider.ResolvedConfig {
	return provider.ResolvedConfig{
		BaseURL: baseURL,
		Auth:    core.Auth{APIKey: "test-key"},
	}
}
