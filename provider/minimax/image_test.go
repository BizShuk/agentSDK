package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

func imageServer(t *testing.T, status int, body string, captured *map[string]json.RawMessage, path *string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*path = r.URL.Path
		assert.Equal(t, "Bearer k", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(captured))
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func newTestImage(t *testing.T, baseURL string) *ImageProvider {
	t.Helper()
	p, err := NewImage(provider.ResolvedConfig{BaseURL: baseURL, Auth: core.Auth{APIKey: "k"}})
	require.NoError(t, err)
	return p
}

const imageSuccessBody = `{"created":1700000000,"output_format":"png",
	"data":[{"url":"https://cdn/a.png","size":"2048x2048","revised_prompt":"a cat, detailed"},
	        {"error":{"code":1026,"message":"withheld"}}],
	"usage":{"generated_images":1,"total_tokens":90,"input_tokens":10,"output_tokens":80,
	         "input_tokens_details":{"text_tokens":10,"image_tokens":0}},"request_id":"r1"}`

func TestGenerateImageTextToImage(t *testing.T) {
	var captured map[string]json.RawMessage
	var path string
	url := imageServer(t, http.StatusOK, imageSuccessBody, &captured, &path)

	result, err := newTestImage(t, url+"/anthropic").GenerateImage(context.Background(), provider.ImageRequest{
		Prompt:         "a cat",
		Count:          2,
		Size:           "16:9",
		ResponseFormat: "base64",
		OutputFormat:   "PNG",
		Extra:          map[string]json.RawMessage{"area": json.RawMessage(`"2K"`)},
	})
	require.NoError(t, err)

	assert.Equal(t, imageGenerationsPath, path)
	assert.JSONEq(t, `"MiniMax-H3-Image"`, string(captured["model"]))
	assert.JSONEq(t, `2`, string(captured["n"]))
	assert.JSONEq(t, `"16:9"`, string(captured["aspect_ratio"]))
	assert.NotContains(t, captured, "size")
	assert.JSONEq(t, `"b64_json"`, string(captured["response_format"]))
	assert.JSONEq(t, `"png"`, string(captured["output_format"]))
	assert.JSONEq(t, `"2K"`, string(captured["area"]))
	assert.NotContains(t, captured, "images")

	require.Len(t, result.Images, 1, "the withheld image is dropped")
	assert.Equal(t, provider.Image{URL: "https://cdn/a.png", MIMEType: "image/png", RevisedPrompt: "a cat, detailed"}, result.Images[0])
	assert.Equal(t, 1, result.Usage.GeneratedImages)
	assert.Equal(t, 80, result.Usage.OutputTokens)
}

func TestGenerateImageWithReferencesUsesEdits(t *testing.T) {
	var captured map[string]json.RawMessage
	var path string
	url := imageServer(t, http.StatusOK, imageSuccessBody, &captured, &path)

	_, err := newTestImage(t, url).GenerateImage(context.Background(), provider.ImageRequest{
		Prompt: "make it night",
		Size:   "2048x1152",
		SubjectReferences: []provider.ImageReference{
			{URL: "https://example.com/a.jpg"},
			{Base64: "AAAA", MIMEType: "image/webp"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, imageEditsPath, path)
	assert.JSONEq(t, `"2048x1152"`, string(captured["size"]))
	assert.JSONEq(t, `[{"image_url":"https://example.com/a.jpg"},{"image_url":"data:image/webp;base64,AAAA"}]`, string(captured["images"]))
}

func TestGenerateImageErrors(t *testing.T) {
	var captured map[string]json.RawMessage
	var path string
	url := imageServer(t, http.StatusTooManyRequests,
		`{"type":"error","error":{"type":"rate_limit_error","message":"code 1002","http_code":"429"},"request_id":"r2"}`, &captured, &path)
	_, err := newTestImage(t, url).GenerateImage(context.Background(), provider.ImageRequest{Prompt: "x"})
	var apiErr *provider.APIError
	require.True(t, errors.As(err, &apiErr), err)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, "r2", apiErr.RequestID)

	allFailed := imageServer(t, http.StatusOK, `{"data":[{"error":{"code":1026,"message":"withheld"}}],"request_id":"r3"}`, &captured, &path)
	_, err = newTestImage(t, allFailed).GenerateImage(context.Background(), provider.ImageRequest{Prompt: "x"})
	assert.ErrorContains(t, err, "no image succeeded: withheld")
}

func TestGenerateImageRejectsUnsupportedFields(t *testing.T) {
	p := newTestImage(t, "http://unused")
	for name, req := range map[string]provider.ImageRequest{
		"quality":         {Prompt: "x", Quality: "hd"},
		"count":           {Prompt: "x", Count: 11},
		"response format": {Prompt: "x", ResponseFormat: "svg"},
		"reserved extra":  {Prompt: "x", Extra: map[string]json.RawMessage{"stream": json.RawMessage(`true`)}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := p.GenerateImage(context.Background(), req)
			assert.Error(t, err)
		})
	}
}
