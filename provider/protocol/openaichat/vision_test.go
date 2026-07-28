package openaichat

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeMessages pulls the messages array back out of an encoded request so
// assertions run against the actual wire bytes, not the encoder's internals.
func decodeMessages(t *testing.T, raw []byte) []json.RawMessage {
	t.Helper()
	var body struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(raw, &body))
	out := make([]json.RawMessage, 0, len(body.Messages))
	for _, m := range body.Messages {
		out = append(out, m.Content)
	}
	return out
}

// A PART_KIND_IMAGE part must reach the wire as the multimodal content
// array with a base64 data URI — this is what vision models actually read.
// Before multimodal support the image was dropped and the model answered
// from the prompt text alone, with no signal that it never saw the picture.
func TestEncodeRequestSendsImagePartAsDataURI(t *testing.T) {
	imgBytes := []byte{0xff, 0xd8, 0xff, 0xe0} // JPEG SOI + APP0
	req := core.ModelRequest{Messages: []core.Message{{
		Role: core.ROLE_USER,
		Parts: []core.Part{
			{Kind: core.PART_KIND_PLAIN_TEXT, Text: "classify this"},
			{Kind: core.PART_KIND_IMAGE, ImageMIME: "image/jpeg", Image: imgBytes},
		},
	}}}

	raw, err := EncodeRequest(req, "qwen2.5vl:3b", false)
	require.NoError(t, err)

	contents := decodeMessages(t, raw)
	require.Len(t, contents, 1)

	var parts []contentPart
	require.NoError(t, json.Unmarshal(contents[0], &parts))
	require.Len(t, parts, 2)

	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "classify this", parts[0].Text)

	assert.Equal(t, "image_url", parts[1].Type)
	require.NotNil(t, parts[1].ImageURL)
	want := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imgBytes)
	assert.Equal(t, want, parts[1].ImageURL.URL)
}

// An image with no declared MIME still has to produce a valid data URI;
// image/jpeg is the fallback every local vision model accepts.
func TestEncodeRequestDefaultsImageMIMEToJPEG(t *testing.T) {
	req := core.ModelRequest{Messages: []core.Message{{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_IMAGE, Image: []byte{0x01}}},
	}}}

	raw, err := EncodeRequest(req, "m", false)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `data:image/jpeg;base64,`)
}

// An image part with no bytes must not emit an empty image_url — a data URI
// with no payload is a 400 from every server.
func TestEncodeRequestSkipsEmptyImage(t *testing.T) {
	req := core.ModelRequest{Messages: []core.Message{{
		Role: core.ROLE_USER,
		Parts: []core.Part{
			{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"},
			{Kind: core.PART_KIND_IMAGE, ImageMIME: "image/png"},
		},
	}}}

	raw, err := EncodeRequest(req, "m", false)
	require.NoError(t, err)

	contents := decodeMessages(t, raw)
	require.Len(t, contents, 1)
	assert.JSONEq(t, `"hi"`, string(contents[0]))
}

// Text-only turns must keep the plain-string content form. Servers that
// predate multimodal support parse only that shape, so promoting every
// message to the array would be a silent regression for them.
func TestEncodeRequestKeepsPlainStringContentWithoutImages(t *testing.T) {
	req := core.ModelRequest{Messages: []core.Message{{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}},
	}}}

	raw, err := EncodeRequest(req, "m", false)
	require.NoError(t, err)

	contents := decodeMessages(t, raw)
	require.Len(t, contents, 1)
	assert.JSONEq(t, `"hi"`, string(contents[0]))
}

// A tool result collapses to the plain-string form even when the same turn
// carried an image, because role=tool has no multimodal content shape.
func TestEncodeRequestToolResultStaysString(t *testing.T) {
	req := core.ModelRequest{Messages: []core.Message{{
		Role: core.ROLE_TOOL,
		Parts: []core.Part{
			{Kind: core.PART_KIND_IMAGE, Image: []byte{0x01}},
			{Kind: core.PART_KIND_TOOL_RESULT, ToolResult: &core.ToolResult{
				CallID: "call_1",
				Name:   "lookup",
				Output: "done",
			}},
		},
	}}}

	raw, err := EncodeRequest(req, "m", false)
	require.NoError(t, err)

	contents := decodeMessages(t, raw)
	require.Len(t, contents, 1)
	assert.JSONEq(t, `"done"`, string(contents[0]))
}

// validate rejects a hand-built content of an unsupported type rather than
// letting the upstream answer with an opaque HTTP 400.
func TestValidateRejectsUnsupportedContentType(t *testing.T) {
	body := requestBody{
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: 42}},
	}
	require.ErrorContains(t, body.validate(), "content must be string or []contentPart")
}

// Decoding is unaffected by the request-side content change: responses only
// ever carry the plain-string form.
func TestDecodeResponseStillReadsStringContent(t *testing.T) {
	raw := []byte(`{"choices":[{"message":{"role":"assistant","content":"{\"category\":\"non_receipt\"}"},"finish_reason":"stop"}]}`)
	got, err := DecodeResponse(raw)
	require.NoError(t, err)
	assert.Equal(t, `{"category":"non_receipt"}`, got.Text)
	assert.Equal(t, "stop", got.StopReason)
}
