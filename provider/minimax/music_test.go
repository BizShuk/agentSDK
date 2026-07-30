package minimax_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/minimax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMusicGeneratorMatchesCoverRequestContract(t *testing.T) {
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
			"data":{
				"audio":"https://cdn.example.test/generated.mp3",
				"status":2
			},
			"trace_id":"trace-music-1",
			"extra_info":{
				"music_duration":25364,
				"music_sample_rate":44100,
				"music_channel":2,
				"bitrate":256000,
				"music_size":813651
			},
			"base_resp":{"status_code":0,"status_msg":"success"}
		}`)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewMusic(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "test-key"},
	})
	require.NoError(t, err)

	result, err := generator.GenerateMusic(context.Background(), provider.MusicRequest{
		Model:        "music-cover",
		AudioURL:     "https://example.com/original-song.mp3",
		Prompt:       "Jazz, smooth, late night lounge, saxophone",
		OutputFormat: "url",
		AudioSetting: provider.MusicAudioSetting{
			SampleRate: 44100,
			Bitrate:    256000,
			Format:     "mp3",
		},
	})
	require.NoError(t, err)
	require.NoError(t, decodeErr)

	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "/v1/music_generation", capturedPath)
	assert.Equal(t, "Bearer test-key", capturedAuth)
	assert.Equal(t, "music-cover", capturedBody["model"])
	assert.Equal(t, "https://example.com/original-song.mp3", capturedBody["audio_url"])
	assert.Equal(t, "Jazz, smooth, late night lounge, saxophone", capturedBody["prompt"])
	assert.Equal(t, "url", capturedBody["output_format"])
	audioSetting, ok := capturedBody["audio_setting"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(44100), audioSetting["sample_rate"])
	assert.Equal(t, float64(256000), audioSetting["bitrate"])
	assert.Equal(t, "mp3", audioSetting["format"])

	assert.Equal(t, "https://cdn.example.test/generated.mp3", result.Audio.URL)
	assert.Empty(t, result.Audio.Hex)
	assert.Equal(t, "mp3", result.Audio.Format)
	assert.Equal(t, 2, result.Status)
	assert.Equal(t, "trace-music-1", result.TraceID)
	assert.Equal(t, int64(25364), result.Info.DurationMilliseconds)
	assert.Equal(t, 44100, result.Info.SampleRate)
	assert.Equal(t, 2, result.Info.Channels)
	assert.Equal(t, 256000, result.Info.Bitrate)
	assert.Equal(t, int64(813651), result.Info.SizeBytes)
}

func TestMusicGeneratorSelectsDefaultModel(t *testing.T) {
	tests := []struct {
		name      string
		request   provider.MusicRequest
		wantModel string
	}{
		{
			name: "text",
			request: provider.MusicRequest{
				Prompt:          "Ambient piano for quiet reflection",
				LyricsOptimizer: true,
			},
			wantModel: "music-3.0",
		},
		{
			name: "cover",
			request: provider.MusicRequest{
				Prompt:   "Jazz, smooth, late night lounge",
				AudioURL: "https://example.test/original.mp3",
			},
			wantModel: "music-cover",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var model string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				model = body.Model
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{
					"data":{"audio":"deadbeef","status":2},
					"base_resp":{"status_code":0}
				}`)
			}))
			t.Cleanup(server.Close)

			generator, err := minimax.NewMusic(musicConfig(server.URL))
			require.NoError(t, err)
			result, err := generator.GenerateMusic(context.Background(), tt.request)
			require.NoError(t, err)
			assert.Equal(t, tt.wantModel, model)
			assert.Equal(t, "deadbeef", result.Audio.Hex)
			assert.Empty(t, result.Audio.URL)
		})
	}
}

func TestMusicGeneratorRejectsInvalidRequests(t *testing.T) {
	generator, err := minimax.NewMusic(musicConfig("https://example.invalid"))
	require.NoError(t, err)

	tests := []struct {
		name    string
		request provider.MusicRequest
		wantErr string
	}{
		{
			name: "general prompt too long",
			request: provider.MusicRequest{
				Prompt:          strings.Repeat("曲", 2001),
				LyricsOptimizer: true,
			},
			wantErr: "prompt",
		},
		{
			name: "general lyrics too long",
			request: provider.MusicRequest{
				Prompt: "Pop",
				Lyrics: strings.Repeat("詞", 3501),
			},
			wantErr: "lyrics",
		},
		{
			name: "default text model requires lyrics",
			request: provider.MusicRequest{
				Prompt: "Ambient piano",
			},
			wantErr: "lyrics are required",
		},
		{
			name: "cover source required",
			request: provider.MusicRequest{
				Model:  "music-cover",
				Prompt: "Jazz, smooth, late night lounge",
			},
			wantErr: "cover source",
		},
		{
			name: "cover prompt too short",
			request: provider.MusicRequest{
				Model:    "music-cover",
				Prompt:   strings.Repeat("a", 9),
				AudioURL: "https://example.test/original.mp3",
			},
			wantErr: "prompt",
		},
		{
			name: "cover prompt too long",
			request: provider.MusicRequest{
				Model:    "music-cover",
				Prompt:   strings.Repeat("a", 301),
				AudioURL: "https://example.test/original.mp3",
			},
			wantErr: "prompt",
		},
		{
			name: "cover lyrics too short",
			request: provider.MusicRequest{
				Model:    "music-cover",
				Prompt:   "Jazz, smooth, late night lounge",
				Lyrics:   strings.Repeat("a", 9),
				AudioURL: "https://example.test/original.mp3",
			},
			wantErr: "lyrics",
		},
		{
			name: "cover lyrics too long",
			request: provider.MusicRequest{
				Model:    "music-cover",
				Prompt:   "Jazz, smooth, late night lounge",
				Lyrics:   strings.Repeat("a", 1001),
				AudioURL: "https://example.test/original.mp3",
			},
			wantErr: "lyrics",
		},
		{
			name: "cover feature requires lyrics",
			request: provider.MusicRequest{
				Model:          "music-cover",
				Prompt:         "Jazz, smooth, late night lounge",
				CoverFeatureID: "feature-1",
			},
			wantErr: "lyrics",
		},
		{
			name: "lyrics optimizer unsupported for cover",
			request: provider.MusicRequest{
				Model:           "music-cover",
				Prompt:          "Jazz, smooth, late night lounge",
				AudioURL:        "https://example.test/original.mp3",
				LyricsOptimizer: true,
			},
			wantErr: "lyrics optimizer",
		},
		{
			name: "instrumental unsupported for cover",
			request: provider.MusicRequest{
				Model:        "music-cover",
				Prompt:       "Jazz, smooth, late night lounge",
				AudioURL:     "https://example.test/original.mp3",
				Instrumental: true,
			},
			wantErr: "instrumental",
		},
		{
			name: "output format",
			request: provider.MusicRequest{
				Prompt:       "Ambient piano",
				Lyrics:       "A line of lyrics",
				OutputFormat: "base64",
			},
			wantErr: "output format",
		},
		{
			name: "sample rate",
			request: provider.MusicRequest{
				Prompt: "Ambient piano",
				Lyrics: "A line of lyrics",
				AudioSetting: provider.MusicAudioSetting{
					SampleRate: 48000,
				},
			},
			wantErr: "sample rate",
		},
		{
			name: "bitrate",
			request: provider.MusicRequest{
				Prompt: "Ambient piano",
				Lyrics: "A line of lyrics",
				AudioSetting: provider.MusicAudioSetting{
					Bitrate: 192000,
				},
			},
			wantErr: "bitrate",
		},
		{
			name: "audio format",
			request: provider.MusicRequest{
				Prompt: "Ambient piano",
				Lyrics: "A line of lyrics",
				AudioSetting: provider.MusicAudioSetting{
					Format: "flac",
				},
			},
			wantErr: "audio format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := generator.GenerateMusic(context.Background(), tt.request)
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), tt.wantErr)
		})
	}
}

func TestMusicGeneratorReturnsSemanticAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"base_resp":{"status_code":1004,"status_msg":"authentication failed"}
		}`)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewMusic(musicConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateMusic(context.Background(), textMusicRequest())
	require.Error(t, err)

	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "minimax", apiErr.Provider)
	assert.Equal(t, "generate music", apiErr.Operation)
	assert.Equal(t, http.StatusOK, apiErr.StatusCode)
	assert.Equal(t, "1004", apiErr.Code)
	assert.Equal(t, "authentication failed", apiErr.Message)
}

func TestMusicGeneratorReturnsHTTPAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "request-music-1")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{
			"base_resp":{"status_code":1002,"status_msg":"rate limited"}
		}`)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewMusic(musicConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateMusic(context.Background(), textMusicRequest())
	require.Error(t, err)

	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	assert.Equal(t, "1002", apiErr.Code)
	assert.Equal(t, "rate limited", apiErr.Message)
	assert.Equal(t, "request-music-1", apiErr.RequestID)
}

func TestMusicGeneratorRejectsIncompleteResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing audio",
			body:    `{"data":{"status":2},"base_resp":{"status_code":0}}`,
			wantErr: "audio",
		},
		{
			name:    "still in progress",
			body:    `{"data":{"audio":"deadbeef","status":1},"base_resp":{"status_code":0}}`,
			wantErr: "status 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(server.Close)

			generator, err := minimax.NewMusic(musicConfig(server.URL))
			require.NoError(t, err)
			_, err = generator.GenerateMusic(context.Background(), textMusicRequest())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMusicGeneratorRequestAuthOverridesConstructionAuth(t *testing.T) {
	var authorization string
	var traceHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		traceHeader = r.Header.Get("X-Trace-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"data":{"audio":"deadbeef","status":2},
			"base_resp":{"status_code":0}
		}`)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewMusic(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "construction-key"},
	})
	require.NoError(t, err)
	request := textMusicRequest()
	request.Auth = core.Auth{
		Bearer: "request-token",
		Headers: map[string]string{
			"X-Trace-ID": "request-trace",
		},
	}
	_, err = generator.GenerateMusic(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, "Bearer request-token", authorization)
	assert.Equal(t, "request-trace", traceHeader)
}

func TestNewMusicUsesMusicSpecificBaseURLEnv(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"data":{"audio":"deadbeef","status":2},
			"base_resp":{"status_code":0}
		}`)
	}))
	t.Cleanup(server.Close)

	env := map[string]string{
		"MINIMAX_API_KEY":        "test-key",
		"MINIMAX_BASE_URL":       "https://chat.example.invalid/anthropic",
		"MINIMAX_MUSIC_BASE_URL": server.URL,
	}
	generator, err := provider.NewMusic("minimax", provider.Options{
		LookupEnv: func(key string) string { return env[key] },
	})
	require.NoError(t, err)
	_, err = generator.GenerateMusic(context.Background(), textMusicRequest())
	require.NoError(t, err)
}

func TestMusicGeneratorHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("canceled request must not reach the upstream server")
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewMusic(musicConfig(server.URL))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = generator.GenerateMusic(ctx, textMusicRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestMusicPromptLimitsCountRunes(t *testing.T) {
	generator, err := minimax.NewMusic(musicConfig("https://example.invalid"))
	require.NoError(t, err)
	request := textMusicRequest()
	request.Prompt = strings.Repeat("夜", 2001)
	assert.Equal(t, 2001, utf8.RuneCountInString(request.Prompt))

	_, err = generator.GenerateMusic(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2001")
}

func musicConfig(baseURL string) provider.ResolvedConfig {
	return provider.ResolvedConfig{
		BaseURL: baseURL,
		Auth:    core.Auth{APIKey: "test-key"},
	}
}

func textMusicRequest() provider.MusicRequest {
	return provider.MusicRequest{
		Prompt: "Ambient piano",
		Lyrics: "A line of lyrics",
	}
}
