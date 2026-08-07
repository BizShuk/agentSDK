package provider_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/google"
	"github.com/bizshuk/agentsdk/provider/grok"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type openAIImageAdapterCase struct {
	name         string
	defaultModel string
	new          func(t *testing.T, baseURL string, model string) provider.ImageGenerator
}

func openAIImageAdapterCases() []openAIImageAdapterCase {
	return []openAIImageAdapterCase{
		{
			name:         "google",
			defaultModel: "gemini-2.5-flash-image",
			new: func(t *testing.T, baseURL string, model string) provider.ImageGenerator {
				t.Helper()
				generator, err := google.NewImage(provider.ResolvedConfig{
					Model:   model,
					BaseURL: baseURL,
					Auth: core.Auth{
						APIKey: "test-key",
						Headers: map[string]string{
							"X-Trace": "trace-1",
						},
					},
				})
				require.NoError(t, err)
				return generator
			},
		},
		{
			name:         "grok",
			defaultModel: "grok-imagine-image-quality",
			new: func(t *testing.T, baseURL string, model string) provider.ImageGenerator {
				t.Helper()
				generator, err := grok.NewImage(provider.ResolvedConfig{
					Model:   model,
					BaseURL: baseURL,
					Auth: core.Auth{
						APIKey: "test-key",
						Headers: map[string]string{
							"X-Trace": "trace-1",
						},
					},
				})
				require.NoError(t, err)
				return generator
			},
		},
	}
}

func openAIImageRequest() provider.ImageRequest {
	return provider.ImageRequest{
		Prompt:         "a geometric fox",
		Count:          2,
		Size:           "1024x1024",
		Quality:        "high",
		ResponseFormat: "b64_json",
		OutputFormat:   "png",
		Background:     "opaque",
		Moderation:     "auto",
		User:           "user-1",
	}
}

func readOpenAIImageGolden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/openaiimage/" + name)
	require.NoError(t, err)
	return bytes.TrimSuffix(raw, []byte("\n"))
}

func TestOpenAIImageGenerateGolden(t *testing.T) {
	response := readOpenAIImageGolden(t, "response.json")
	wantRequest := readOpenAIImageGolden(t, "generate_request.json")
	wantResult := provider.ImageResult{
		Created: 1713833628,
		Images: []provider.Image{
			{
				Base64:        "aW1hZ2UtMQ==",
				MIMEType:      "image/png",
				RevisedPrompt: "a precise geometric fox",
			},
			{URL: "https://example.test/image-2.png", MIMEType: "image/png"},
		},
		Usage: provider.ImageUsage{
			TotalTokens:     100,
			InputTokens:     50,
			OutputTokens:    50,
			GeneratedImages: 2,
			InputTokenDetails: provider.ImageInputTokenDetails{
				TextTokens:  10,
				ImageTokens: 40,
			},
		},
		Cost: core.ExactCostFromUSDTicks(200000000),
	}

	for _, tc := range openAIImageAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte
			var gotPath string
			var gotAuth string
			var gotTrace string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				gotTrace = r.Header.Get("X-Trace")
				var err error
				gotBody, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(response)
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			got, err := tc.new(t, srv.URL, "shared-image-model").
				GenerateImage(context.Background(), openAIImageRequest())
			require.NoError(t, err)
			assert.Equal(t, wantRequest, gotBody)
			assert.Equal(t, "/images/generations", gotPath)
			assert.Equal(t, "Bearer test-key", gotAuth)
			assert.Equal(t, "trace-1", gotTrace)
			assert.Equal(t, wantResult, got)
		})
	}
}

func TestOpenAIImageProviderDefaults(t *testing.T) {
	for _, tc := range openAIImageAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				gotBody = string(raw)
				_, err = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			_, err := tc.new(t, srv.URL, "").
				GenerateImage(context.Background(), provider.ImageRequest{Prompt: "draw"})
			require.NoError(t, err)
			assert.JSONEq(t,
				`{"model":"`+tc.defaultModel+`","prompt":"draw"}`,
				gotBody,
			)
		})
	}
}

func TestOpenAIImageErrorSemantics(t *testing.T) {
	for _, tc := range openAIImageAdapterCases() {
		t.Run(tc.name+"/validation", func(t *testing.T) {
			_, err := tc.new(t, "http://127.0.0.1:1", "").
				GenerateImage(context.Background(), provider.ImageRequest{})
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.name+": image: image prompt is required")
		})

		t.Run(tc.name+"/status", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Request-ID", "req-image-1")
				w.WriteHeader(http.StatusTooManyRequests)
				_, err := w.Write([]byte(`{
					"error":{
						"message":"rate limited",
						"type":"rate_limit_error",
						"code":"rate_limit_exceeded",
						"private":"must-not-leak"
					}
				}`))
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			_, err := tc.new(t, srv.URL, "").
				GenerateImage(context.Background(), provider.ImageRequest{Prompt: "draw"})
			var apiErr *provider.APIError
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, tc.name, apiErr.Provider)
			assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
			assert.Equal(t, "rate_limit_exceeded", apiErr.Code)
			assert.Equal(t, "req-image-1", apiErr.RequestID)
			assert.NotContains(t, err.Error(), "must-not-leak")
		})

		t.Run(tc.name+"/decode", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write([]byte("not-json"))
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			_, err := tc.new(t, srv.URL, "").
				GenerateImage(context.Background(), provider.ImageRequest{Prompt: "draw"})
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.name+": image: decode image response:")
		})
	}
}

func TestOpenAIImageContextCancellation(t *testing.T) {
	for _, tc := range openAIImageAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				<-r.Context().Done()
			}))
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := tc.new(t, srv.URL, "").
				GenerateImage(ctx, provider.ImageRequest{Prompt: "draw"})
			require.Error(t, err)
			assert.True(t,
				errors.Is(err, context.Canceled) ||
					strings.Contains(err.Error(), context.Canceled.Error()),
			)
		})
	}
}

func TestNewImageAuthPrecedence(t *testing.T) {
	cases := []struct {
		name        string
		options     func(baseURL string) provider.Options
		requestAuth core.Auth
		wantToken   string
	}{
		{
			name: "decorator resolves request token",
			options: func(baseURL string) provider.Options {
				return provider.Options{
					Model:     "image-model",
					BaseURL:   baseURL,
					LookupEnv: func(string) string { return "" },
					Decorator: func(context.Context) (core.Auth, error) {
						return core.Auth{Bearer: "ambient-token"}, nil
					},
				}
			},
			wantToken: "ambient-token",
		},
		{
			name: "explicit construction key outranks decorator",
			options: func(baseURL string) provider.Options {
				return provider.Options{
					Model:   "image-model",
					BaseURL: baseURL,
					APIKey:  "explicit-key",
					Decorator: func(context.Context) (core.Auth, error) {
						return core.Auth{Bearer: "ambient-token"}, nil
					},
				}
			},
			wantToken: "explicit-key",
		},
		{
			name: "per-request auth outranks explicit construction key",
			options: func(baseURL string) provider.Options {
				return provider.Options{
					Model:   "image-model",
					BaseURL: baseURL,
					APIKey:  "explicit-key",
					Decorator: func(context.Context) (core.Auth, error) {
						return core.Auth{Bearer: "ambient-token"}, nil
					},
				}
			},
			requestAuth: core.Auth{Bearer: "per-request-token"},
			wantToken:   "per-request-token",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				_, err := w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			generator, err := provider.NewImage("google", tc.options(srv.URL))
			require.NoError(t, err)
			_, err = generator.GenerateImage(
				context.Background(),
				provider.ImageRequest{
					Prompt: "draw",
					Auth:   tc.requestAuth,
				},
			)
			require.NoError(t, err)
			assert.Equal(t, "Bearer "+tc.wantToken, gotAuth)
		})
	}
}
