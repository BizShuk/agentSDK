package minimax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

func newTestImageProvider(t *testing.T, handler http.HandlerFunc) *ImageProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	generator, err := NewImage(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "test-key"},
	})
	require.NoError(t, err)
	return generator
}

func TestGenerateImageTextToImage(t *testing.T) {
	var captured map[string]json.RawMessage
	generator := newTestImageProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, imageGenerationPath, r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "trace-1",
			"data": map[string]any{
				"image_urls": []string{"https://cdn.example/a.png", "https://cdn.example/b.png"},
			},
			"metadata":  map[string]any{"success_count": 2},
			"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
		})
	})

	result, err := generator.GenerateImage(context.Background(), provider.ImageRequest{
		Prompt: "a lighthouse at dawn",
		Count:  2,
		Size:   "16:9",
		Extra: map[string]json.RawMessage{
			"prompt_optimizer": json.RawMessage("true"),
		},
	})
	require.NoError(t, err)

	assert.JSONEq(t, `"image-01"`, string(captured["model"]))
	assert.JSONEq(t, `"16:9"`, string(captured["aspect_ratio"]))
	assert.JSONEq(t, `2`, string(captured["n"]))
	assert.JSONEq(t, `true`, string(captured["prompt_optimizer"]))
	assert.NotContains(t, captured, "width")
	assert.NotContains(t, captured, "subject_reference")

	require.Len(t, result.Images, 2)
	assert.Equal(t, "https://cdn.example/a.png", result.Images[0].URL)
}

func TestGenerateImageImageToImage(t *testing.T) {
	var captured struct {
		Model            string                  `json:"model"`
		Width            int                     `json:"width"`
		Height           int                     `json:"height"`
		ResponseFormat   string                  `json:"response_format"`
		SubjectReference []imageSubjectReference `json:"subject_reference"`
	}
	generator := newTestImageProvider(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":      map[string]any{"image_base64": []string{"aGVsbG8="}},
			"base_resp": map[string]any{"status_code": 0},
		})
	})

	result, err := generator.GenerateImage(context.Background(), provider.ImageRequest{
		Model:          "image-01-live",
		Prompt:         "same person in a library",
		Size:           "1024x768",
		ResponseFormat: "b64_json",
		SubjectReferences: []provider.ImageReference{
			{URL: "https://example.com/portrait.jpg"},
			{Base64: "cG9ydHJhaXQ=", MIMEType: "image/png"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "image-01-live", captured.Model)
	assert.Equal(t, 1024, captured.Width)
	assert.Equal(t, 768, captured.Height)
	assert.Equal(t, "base64", captured.ResponseFormat)
	require.Len(t, captured.SubjectReference, 2)
	assert.Equal(t, "character", captured.SubjectReference[0].Type)
	assert.Equal(t, "https://example.com/portrait.jpg", captured.SubjectReference[0].ImageFile)
	assert.Equal(t, "data:image/png;base64,cG9ydHJhaXQ=", captured.SubjectReference[1].ImageFile)

	require.Len(t, result.Images, 1)
	assert.Equal(t, "aGVsbG8=", result.Images[0].Base64)
}

func TestGenerateImageAPIError(t *testing.T) {
	generator := newTestImageProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"base_resp": map[string]any{"status_code": 1008, "status_msg": "insufficient balance"},
		})
	})

	_, err := generator.GenerateImage(context.Background(), provider.ImageRequest{
		Prompt: "a lighthouse",
	})
	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "1008", apiErr.Code)
	assert.Equal(t, "insufficient balance", apiErr.Message)
	assert.Equal(t, "generate image", apiErr.Operation)
}

func TestGenerateImageValidation(t *testing.T) {
	generator, err := NewImage(provider.ResolvedConfig{Auth: core.Auth{APIKey: "k"}})
	require.NoError(t, err)

	cases := []struct {
		name    string
		request provider.ImageRequest
		wantErr string
	}{
		{
			name:    "count over limit",
			request: provider.ImageRequest{Prompt: "p", Count: 10},
			wantErr: "exceeds limit",
		},
		{
			name:    "bad size",
			request: provider.ImageRequest{Prompt: "p", Size: "huge"},
			wantErr: "aspect ratio",
		},
		{
			name:    "dimensions not divisible",
			request: provider.ImageRequest{Prompt: "p", Size: "1000x513"},
			wantErr: "divisible by 8",
		},
		{
			name:    "unsupported quality",
			request: provider.ImageRequest{Prompt: "p", Quality: "hd"},
			wantErr: "does not support quality",
		},
		{
			name: "reference with both url and base64",
			request: provider.ImageRequest{
				Prompt: "p",
				SubjectReferences: []provider.ImageReference{
					{URL: "https://example.com/a.jpg", Base64: "aGk="},
				},
			},
			wantErr: "exactly one of url or base64",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := generator.GenerateImage(context.Background(), tc.request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestImageBaseURLTrimsAnthropicSuffix(t *testing.T) {
	assert.Equal(t, "https://api.minimax.io", imageBaseURL("https://api.minimax.io/anthropic"))
	assert.Equal(t, DefaultImageBaseURL, imageBaseURL(""))
	assert.Equal(t, "https://proxy.local", imageBaseURL("https://proxy.local/"))
}
