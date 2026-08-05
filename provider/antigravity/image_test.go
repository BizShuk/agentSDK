package antigravity_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/antigravity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// imageFrame is one SSE frame carrying a generated picture, the shape the
// gateway actually returns.
func imageFrame(payload []byte) string {
	return `data: {"response":{"candidates":[{"content":{"parts":[` +
		`{"inlineData":{"mimeType":"image/jpeg","data":"` +
		base64.StdEncoding.EncodeToString(payload) + `"}}]}}],` +
		`"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":1200,"totalTokenCount":1209}}}` +
		"\n\n" +
		`data: {"response":{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}]}}` + "\n\n"
}

// TestGenerateImageThroughRegistry — the capability is reachable by the
// same provider.NewImage path every other vendor uses, and it defaults to
// the image-capable model rather than the chat default.
func TestGenerateImageThroughRegistry(t *testing.T) {
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x01, 0x02}
	g := newGateway(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(imageFrame(payload)))
	})

	t.Setenv("ANTIGRAVITY_OAUTH_TOKEN", "ya29.token")
	t.Setenv("ANTIGRAVITY_BASE_URL", g.URL)

	gen, err := provider.NewImage("antigravity", provider.Options{})
	require.NoError(t, err)

	res, err := gen.GenerateImage(context.Background(), provider.ImageRequest{
		Prompt: "a single blue circle",
	})
	require.NoError(t, err)

	require.Len(t, res.Images, 1)
	assert.Equal(t, "image/jpeg", res.Images[0].MIMEType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(payload), res.Images[0].Base64)
	assert.Equal(t, 9, res.Usage.InputTokens)
	assert.Equal(t, 1200, res.Usage.OutputTokens)

	// Image generation rides the ordinary chat call; there is no image
	// endpoint on this gateway.
	assert.Contains(t, g.visited(), "/v1internal:streamGenerateContent")
	assert.Equal(t, "gemini-3.1-flash-image", g.body()["model"])
}

// TestGenerateImageRepeatsForCount — the gateway returns one image per
// turn, so Count becomes that many billed requests.
func TestGenerateImageRepeatsForCount(t *testing.T) {
	var calls int
	g := newGateway(t, func(w http.ResponseWriter, path string) {
		if strings.HasSuffix(path, "streamGenerateContent") {
			calls++
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(imageFrame([]byte{0xff, 0xd8})))
	})

	gen, err := antigravity.NewImageGenerator(provider.ResolvedConfig{
		BaseURL: g.URL,
		Auth:    core.Auth{Bearer: "t"},
	})
	require.NoError(t, err)
	gen.WithProjectID("proj")

	res, err := gen.GenerateImage(context.Background(), provider.ImageRequest{
		Prompt: "a circle",
		Count:  3,
	})
	require.NoError(t, err)
	assert.Len(t, res.Images, 3)
	assert.Equal(t, 3, calls)
	assert.Equal(t, 27, res.Usage.InputTokens, "usage accumulates across turns")
}

// TestGenerateImageRejectsExcessiveCount — this endpoint exhausts an
// account's image quota for days, so an unbounded Count is refused rather
// than served.
func TestGenerateImageRejectsExcessiveCount(t *testing.T) {
	gen, err := antigravity.NewImageGenerator(provider.ResolvedConfig{
		BaseURL: "https://example.invalid",
		Auth:    core.Auth{Bearer: "t"},
	})
	require.NoError(t, err)
	gen.WithProjectID("proj")

	_, err = gen.GenerateImage(context.Background(), provider.ImageRequest{
		Prompt: "a circle",
		Count:  99,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

// TestGenerateImageRejectsUnsupportedControls — the chat surface carries
// none of the OpenAI-shaped image knobs, and silently dropping one would
// hand back an image that ignored what the caller asked for.
func TestGenerateImageRejectsUnsupportedControls(t *testing.T) {
	gen, err := antigravity.NewImageGenerator(provider.ResolvedConfig{
		BaseURL: "https://example.invalid",
		Auth:    core.Auth{Bearer: "t"},
	})
	require.NoError(t, err)
	gen.WithProjectID("proj")

	cases := []struct {
		name string
		req  provider.ImageRequest
		want string
	}{
		{"size", provider.ImageRequest{Prompt: "x", Size: "1024x1024"}, "size"},
		{"quality", provider.ImageRequest{Prompt: "x", Quality: "high"}, "quality"},
		{"background", provider.ImageRequest{Prompt: "x", Background: "transparent"}, "background"},
		{"response_format", provider.ImageRequest{Prompt: "x", ResponseFormat: "url"}, "response_format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gen.GenerateImage(context.Background(), tc.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestGenerateImageSendsSubjectReference — image-to-image is native here:
// a reference image is simply another part of the same message, so unlike
// the shared openaiimage codec this adapter accepts one.
func TestGenerateImageSendsSubjectReference(t *testing.T) {
	reference := []byte{0x89, 0x50, 0x4e, 0x47}
	g := newGateway(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(imageFrame([]byte{0xff, 0xd8})))
	})

	gen, err := antigravity.NewImageGenerator(provider.ResolvedConfig{
		BaseURL: g.URL,
		Auth:    core.Auth{Bearer: "t"},
	})
	require.NoError(t, err)
	gen.WithProjectID("proj")

	_, err = gen.GenerateImage(context.Background(), provider.ImageRequest{
		Prompt: "make it blue",
		SubjectReferences: []provider.ImageReference{{
			Base64:   base64.StdEncoding.EncodeToString(reference),
			MIMEType: "image/png",
		}},
	})
	require.NoError(t, err)

	parts := g.body()["request"].(map[string]any)["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	require.Len(t, parts, 2, "reference image precedes the prompt")
	inline := parts[0].(map[string]any)["inlineData"].(map[string]any)
	assert.Equal(t, "image/png", inline["mimeType"])
	assert.Equal(t, base64.StdEncoding.EncodeToString(reference), inline["data"])
	assert.Equal(t, "make it blue", parts[1].(map[string]any)["text"])
}

// TestGenerateImageRejectsURLReference — the gateway takes inline bytes
// only; accepting a URL and dropping it would generate an unconditioned
// image that looks like a bad model rather than a missing capability.
func TestGenerateImageRejectsURLReference(t *testing.T) {
	gen, err := antigravity.NewImageGenerator(provider.ResolvedConfig{
		BaseURL: "https://example.invalid",
		Auth:    core.Auth{Bearer: "t"},
	})
	require.NoError(t, err)
	gen.WithProjectID("proj")

	_, err = gen.GenerateImage(context.Background(), provider.ImageRequest{
		Prompt:            "make it blue",
		SubjectReferences: []provider.ImageReference{{URL: "https://example.com/a.png"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline bytes only")
}

// TestGenerateImageFailsOnTextOnlyReply — a non-image model answers by
// describing the picture. That is a failure, not an empty success.
func TestGenerateImageFailsOnTextOnlyReply(t *testing.T) {
	g := newGateway(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"response":{"candidates":[{"content":{"parts":[{"text":"Here is an image of a circle."}]},"finishReason":"STOP"}]}}` + "\n\n"))
	})

	gen, err := antigravity.NewImageGenerator(provider.ResolvedConfig{
		BaseURL: g.URL,
		Model:   "gemini-3.6-flash-high",
		Auth:    core.Auth{Bearer: "t"},
	})
	require.NoError(t, err)
	gen.WithProjectID("proj")

	_, err = gen.GenerateImage(context.Background(), provider.ImageRequest{Prompt: "a circle"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "returned no image")
}
