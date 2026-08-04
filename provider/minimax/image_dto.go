package minimax

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

// Wire shapes for POST /v1/image_generation, kept in sync with
// https://platform.minimax.io/docs/api-reference/image-generation-t2i and
// the image-to-image variant (subject_reference) documented alongside it.

type imageGenerationRequest struct {
	Model            string                  `json:"model"`
	Prompt           string                  `json:"prompt"`
	AspectRatio      string                  `json:"aspect_ratio,omitempty"`
	Width            int                     `json:"width,omitempty"`
	Height           int                     `json:"height,omitempty"`
	ResponseFormat   string                  `json:"response_format,omitempty"`
	N                int                     `json:"n,omitempty"`
	SubjectReference []imageSubjectReference `json:"subject_reference,omitempty"`
}

// imageSubjectReference is one image-to-image conditioning entry. Type is
// "character" — the only value the API currently accepts; ImageFile is a
// public URL or an RFC 2397 data URI.
type imageSubjectReference struct {
	Type      string `json:"type"`
	ImageFile string `json:"image_file"`
}

type imageBaseResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

type imageGenerationResponse struct {
	ID   string `json:"id"`
	Data struct {
		ImageURLs   []string `json:"image_urls"`
		ImageBase64 []string `json:"image_base64"`
	} `json:"data"`
	Metadata struct {
		SuccessCount int `json:"success_count"`
		FailedCount  int `json:"failed_count"`
	} `json:"metadata"`
	BaseResp imageBaseResponse `json:"base_resp"`
}

// imageReservedRequestFields are the wire keys the neutral request already
// owns; an Extra entry may not override them.
var imageReservedRequestFields = map[string]struct{}{
	"model":             {},
	"prompt":            {},
	"aspect_ratio":      {},
	"width":             {},
	"height":            {},
	"response_format":   {},
	"n":                 {},
	"subject_reference": {},
}

// encodeImageRequest converts the validated neutral request into the wire
// payload, merging vendor extensions from Extra (seed, prompt_optimizer, ...).
func encodeImageRequest(
	request provider.ImageRequest,
	model string,
) ([]byte, error) {
	wire := imageGenerationRequest{
		Model:          model,
		Prompt:         strings.TrimSpace(request.Prompt),
		ResponseFormat: normalizeImageResponseFormat(request.ResponseFormat),
		N:              request.Count,
	}
	aspectRatio, width, height, err := parseImageSize(request.Size)
	if err != nil {
		return nil, err
	}
	wire.AspectRatio = aspectRatio
	wire.Width = width
	wire.Height = height
	for _, ref := range request.SubjectReferences {
		wire.SubjectReference = append(wire.SubjectReference, imageSubjectReference{
			Type:      "character",
			ImageFile: imageReferenceFile(ref),
		})
	}

	raw, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("minimax image: encode request: %w", err)
	}
	if len(request.Extra) == 0 {
		return raw, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("minimax image: merge extra: %w", err)
	}
	for key, value := range request.Extra {
		if _, reserved := imageReservedRequestFields[key]; reserved {
			return nil, fmt.Errorf(
				"minimax image extra field %q duplicates a standard field",
				key,
			)
		}
		object[key] = value
	}
	merged, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("minimax image: merge extra: %w", err)
	}
	return merged, nil
}

// imageReferenceFile renders one neutral reference as the image_file value:
// a URL passes through, base64 payloads become a data URI.
func imageReferenceFile(ref provider.ImageReference) string {
	if url := strings.TrimSpace(ref.URL); url != "" {
		return url
	}
	mime := strings.TrimSpace(ref.MIMEType)
	if mime == "" {
		mime = DEFAULT_IMAGE_MIME
	}
	return "data:" + mime + ";base64," + strings.TrimSpace(ref.Base64)
}

// normalizeImageResponseFormat maps the OpenAI-convention "b64_json" label
// onto MiniMax's "base64"; url and base64 pass through, empty keeps the
// vendor default (url).
func normalizeImageResponseFormat(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "b64_json" {
		return "base64"
	}
	return normalized
}

// parseImageSize splits the neutral Size label into MiniMax's two mutually
// exclusive shapes: "W:H" becomes aspect_ratio, "WxH" becomes width/height.
func parseImageSize(value string) (string, int, int, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "", 0, 0, nil
	}
	if strings.Contains(trimmed, ":") {
		return trimmed, 0, 0, nil
	}
	widthRaw, heightRaw, found := strings.Cut(trimmed, "x")
	if !found {
		return "", 0, 0, fmt.Errorf(
			"minimax image size %q must be an aspect ratio (16:9) or dimensions (1024x1024)",
			value,
		)
	}
	var width, height int
	if _, err := fmt.Sscanf(widthRaw+" "+heightRaw, "%d %d", &width, &height); err != nil {
		return "", 0, 0, fmt.Errorf(
			"minimax image size %q must be an aspect ratio (16:9) or dimensions (1024x1024)",
			value,
		)
	}
	return "", width, height, nil
}

// foldImageResponse projects the wire response onto the neutral result. The
// API returns either URLs or base64 payloads depending on response_format.
func foldImageResponse(response imageGenerationResponse) provider.ImageResult {
	result := provider.ImageResult{}
	for _, url := range response.Data.ImageURLs {
		result.Images = append(result.Images, provider.Image{URL: url})
	}
	for _, payload := range response.Data.ImageBase64 {
		result.Images = append(result.Images, provider.Image{Base64: payload})
	}
	return result
}
