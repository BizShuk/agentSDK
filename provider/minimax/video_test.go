package minimax_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/minimax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fakeMP4 = []byte{
	0x00, 0x00, 0x00, 0x18,
	'f', 't', 'y', 'p',
	'i', 's', 'o', 'm',
}

func TestVideoGeneratorEndToEndModes(t *testing.T) {
	tests := []struct {
		name      string
		mode      provider.VideoMode
		wantModel string
		configure func(*provider.VideoRequest)
		assert    func(*testing.T, map[string]any)
	}{
		{
			name:      "text",
			mode:      provider.VIDEO_MODE_TEXT,
			wantModel: "MiniMax-Hailuo-2.3",
		},
		{
			name:      "image",
			mode:      provider.VIDEO_MODE_IMAGE,
			wantModel: "MiniMax-Hailuo-2.3",
			configure: func(req *provider.VideoRequest) {
				req.FirstFrameURL = "https://example.test/first.png"
			},
			assert: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, "https://example.test/first.png", payload["first_frame_image"])
			},
		},
		{
			name:      "start and end",
			mode:      provider.VIDEO_MODE_START_END,
			wantModel: "MiniMax-Hailuo-02",
			configure: func(req *provider.VideoRequest) {
				req.FirstFrameURL = "https://example.test/young.png"
				req.LastFrameURL = "https://example.test/old.png"
			},
			assert: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, "https://example.test/young.png", payload["first_frame_image"])
				assert.Equal(t, "https://example.test/old.png", payload["last_frame_image"])
			},
		},
		{
			name:      "subject",
			mode:      provider.VIDEO_MODE_SUBJECT,
			wantModel: "S2V-01",
			configure: func(req *provider.VideoRequest) {
				req.SubjectImageURLs = []string{
					"https://example.test/front.png",
					"https://example.test/profile.png",
				}
			},
			assert: func(t *testing.T, payload map[string]any) {
				references, ok := payload["subject_reference"].([]any)
				require.True(t, ok)
				require.Len(t, references, 1)
				reference, ok := references[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "character", reference["type"])
				assert.Equal(t, []any{
					"https://example.test/front.png",
					"https://example.test/profile.png",
				}, reference["image"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured map[string]any
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")

				switch r.URL.Path {
				case "/v1/video_generation":
					require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
					_, _ = io.WriteString(w, `{"task_id":"task-1","base_resp":{"status_code":0}}`)
				case "/v1/query/video_generation":
					assert.Equal(t, "task-1", r.URL.Query().Get("task_id"))
					_, _ = io.WriteString(w, `{"status":"Success","file_id":"file-1","base_resp":{"status_code":0}}`)
				case "/v1/files/retrieve":
					assert.Equal(t, "file-1", r.URL.Query().Get("file_id"))
					_ = json.NewEncoder(w).Encode(map[string]any{
						"base_resp": map[string]any{"status_code": 0},
						"file":      map[string]any{"download_url": server.URL + "/download"},
					})
				case "/download":
					w.Header().Set("Content-Type", "video/mp4")
					_, _ = w.Write(fakeMP4)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			generator, err := minimax.NewVideo(provider.ResolvedConfig{
				BaseURL: server.URL,
				Auth:    core.Auth{APIKey: "test-key"},
			})
			require.NoError(t, err)

			output := filepath.Join(t.TempDir(), tt.name+".mp4")
			request := provider.VideoRequest{
				Mode:         tt.mode,
				Prompt:       "cinematic motion",
				Duration:     6,
				Resolution:   "1080P",
				OutputPath:   output,
				OutputFormat: "mp4",
				PollInterval: time.Millisecond,
			}
			if tt.configure != nil {
				tt.configure(&request)
			}

			result, err := generator.GenerateVideo(context.Background(), request)
			require.NoError(t, err)
			assert.Equal(t, output, result.Path)
			assert.Equal(t, "cinematic motion", captured["prompt"])
			assert.Equal(t, tt.wantModel, captured["model"])
			assert.Equal(t, float64(6), captured["duration"])
			assert.Equal(t, "1080P", captured["resolution"])
			if tt.assert != nil {
				tt.assert(t, captured)
			}

			got, err := os.ReadFile(output)
			require.NoError(t, err)
			assert.Equal(t, fakeMP4, got)
		})
	}
}

func TestVideoGeneratorPollsUntilSuccess(t *testing.T) {
	var polls atomic.Int32
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/query/video_generation":
			if polls.Add(1) == 1 {
				_, _ = io.WriteString(w, `{"status":"Processing","base_resp":{"status_code":0}}`)
				return
			}
			_, _ = io.WriteString(w, `{"status":"Success","file_id":"file-1","base_resp":{"status_code":0}}`)
		}
	})

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, polls.Load(), int32(2))
}

func TestVideoGeneratorReturnsTaskFailure(t *testing.T) {
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/query/video_generation" {
			_, _ = io.WriteString(w, `{
				"status":"Fail",
				"error_message":"content blocked",
				"base_resp":{"status_code":0}
			}`)
		}
	})

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.Error(t, err)
	assert.ErrorIs(t, err, minimax.ErrVideoTaskFailed)
	assert.Contains(t, err.Error(), "content blocked")
}

func TestVideoGeneratorReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"task_id":"",
			"base_resp":{"status_code":1004,"status_msg":"login failed"}
		}`)
	}))
	t.Cleanup(server.Close)

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "code 1004")
	assert.Contains(t, err.Error(), "login failed")
}

func TestVideoGeneratorRejectsEmptyTaskID(t *testing.T) {
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/video_generation" {
			_, _ = io.WriteString(w, `{"base_resp":{"status_code":0}}`)
		}
	})

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty task id")
}

func TestVideoGeneratorReturnsHTTPError(t *testing.T) {
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/video_generation" {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		}
	})

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status=502")
	assert.Contains(t, err.Error(), "upstream unavailable")
}

func TestVideoGeneratorRetriesTransientPollingError(t *testing.T) {
	var polls atomic.Int32
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/query/video_generation" && polls.Add(1) == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
		}
	})

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, polls.Load(), int32(2))
}

func TestVideoGeneratorRejectsMissingDownloadURL(t *testing.T) {
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/files/retrieve" {
			_, _ = io.WriteString(w, `{"file":{},"base_resp":{"status_code":0}}`)
		}
	})

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no download URL")
}

func TestVideoGeneratorRejectsEmptyDownloadedFile(t *testing.T) {
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output is empty")
}

func TestVideoGeneratorRejectsPromptOverLimit(t *testing.T) {
	generator, err := minimax.NewVideo(videoConfig("https://example.invalid"))
	require.NoError(t, err)
	assert.Equal(t, 3000, generator.MaxPromptLength())

	request := textVideoRequest(t, time.Millisecond)
	request.Prompt = strings.Repeat("場", generator.MaxPromptLength()+1)
	_, err = generator.GenerateVideo(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3000")
}

func TestVideoGeneratorRejectsUnsupportedFormat(t *testing.T) {
	generator, err := minimax.NewVideo(videoConfig("https://example.invalid"))
	require.NoError(t, err)

	request := textVideoRequest(t, time.Millisecond)
	request.OutputFormat = "webm"
	_, err = generator.GenerateVideo(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webm")
}

func TestVideoGeneratorRejectsInvalidDownloadedFile(t *testing.T) {
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			_, _ = io.WriteString(w, "not an mp4")
		}
	})

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an MP4")
}

func TestVideoGeneratorRequestAuthOverridesConstructionAuth(t *testing.T) {
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer request-token", r.Header.Get("Authorization"))
		assert.Equal(t, "request-trace", r.Header.Get("X-Trace-ID"))
	})

	generator, err := minimax.NewVideo(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "construction-key"},
	})
	require.NoError(t, err)
	request := textVideoRequest(t, time.Millisecond)
	request.Auth = core.Auth{
		Bearer: "request-token",
		Headers: map[string]string{
			"X-Trace-ID": "request-trace",
		},
	}
	_, err = generator.GenerateVideo(context.Background(), request)
	require.NoError(t, err)
}

func TestNewVideoUsesVideoSpecificBaseURLEnv(t *testing.T) {
	server := newVideoServer(t, nil)
	env := map[string]string{
		"MINIMAX_API_KEY":        "test-key",
		"MINIMAX_BASE_URL":       "https://chat.example.invalid/anthropic",
		"MINIMAX_VIDEO_BASE_URL": server.URL,
	}
	generator, err := provider.NewVideo("minimax", provider.Options{
		LookupEnv: func(key string) string { return env[key] },
	})
	require.NoError(t, err)

	_, err = generator.GenerateVideo(context.Background(), textVideoRequest(t, time.Millisecond))
	require.NoError(t, err)
}

func TestVideoGeneratorPollingHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := newVideoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/query/video_generation" {
			cancel()
			_, _ = io.WriteString(w, `{"status":"Processing","base_resp":{"status_code":0}}`)
		}
	})

	generator, err := minimax.NewVideo(videoConfig(server.URL))
	require.NoError(t, err)
	defer cancel()
	_, err = generator.GenerateVideo(ctx, textVideoRequest(t, time.Millisecond))
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.ErrorIs(t, err, minimax.ErrVideoPollingCancelled)
}

func videoConfig(baseURL string) provider.ResolvedConfig {
	return provider.ResolvedConfig{
		BaseURL: baseURL,
		Auth:    core.Auth{APIKey: "test-key"},
	}
}

func textVideoRequest(t *testing.T, pollInterval time.Duration) provider.VideoRequest {
	t.Helper()
	return provider.VideoRequest{
		Mode:         provider.VIDEO_MODE_TEXT,
		Prompt:       "cinematic motion",
		OutputPath:   filepath.Join(t.TempDir(), "output.mp4"),
		OutputFormat: "mp4",
		PollInterval: pollInterval,
	}
}

func newVideoServer(
	t *testing.T,
	override func(http.ResponseWriter, *http.Request),
) *httptest.Server {
	t.Helper()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if override != nil {
			recorder := httptest.NewRecorder()
			override(recorder, r)
			if recorder.Body.Len() > 0 || recorder.Code != http.StatusOK {
				for key, values := range recorder.Header() {
					w.Header()[key] = values
				}
				w.WriteHeader(recorder.Code)
				_, _ = w.Write(recorder.Body.Bytes())
				return
			}
		}

		switch r.URL.Path {
		case "/v1/video_generation":
			_, _ = io.WriteString(w, `{"task_id":"task-1","base_resp":{"status_code":0}}`)
		case "/v1/query/video_generation":
			_, _ = io.WriteString(w, `{"status":"Success","file_id":"file-1","base_resp":{"status_code":0}}`)
		case "/v1/files/retrieve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"base_resp": map[string]any{"status_code": 0},
				"file":      map[string]any{"download_url": server.URL + "/download"},
			})
		case "/download":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(fakeMP4)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
