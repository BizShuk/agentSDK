package minimax

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

// imageRequest is the shared body of /v1/images/generations and
// /v1/images/edits (JSON variant). Images is set only for edits.
type imageRequest struct {
	Model             string           `json:"model"`
	Prompt            string           `json:"prompt"`
	Images            []imageReference `json:"images,omitempty"`
	N                 int              `json:"n,omitempty"`
	Size              string           `json:"size,omitempty"`
	AspectRatio       string           `json:"aspect_ratio,omitempty"`
	OutputFormat      string           `json:"output_format,omitempty"`
	ResponseFormat    string           `json:"response_format,omitempty"`
	OutputCompression *int             `json:"output_compression,omitempty"`
}

type imageReference struct {
	ImageURL string `json:"image_url"`
}

// imageReservedFields are the wire keys Extra must not override: each has a
// neutral ImageRequest field. Extra carries the MiniMax-only knobs (area,
// disable_prompt_enhanced, stream is rejected: this adapter is blocking).
var imageReservedFields = map[string]struct{}{
	"model": {}, "prompt": {}, "images": {}, "n": {}, "size": {}, "aspect_ratio": {},
	"output_format": {}, "response_format": {}, "output_compression": {}, "stream": {},
}

type imageResponse struct {
	Created      int64  `json:"created"`
	OutputFormat string `json:"output_format"`
	Data         []struct {
		URL           string `json:"url"`
		B64JSON       string `json:"b64_json"`
		RevisedPrompt string `json:"revised_prompt"`
		Error         *struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"data"`
	Usage struct {
		GeneratedImages   int `json:"generated_images"`
		TotalTokens       int `json:"total_tokens"`
		InputTokens       int `json:"input_tokens"`
		OutputTokens      int `json:"output_tokens"`
		InputTokenDetails struct {
			TextTokens  int `json:"text_tokens"`
			ImageTokens int `json:"image_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
	RequestID string `json:"request_id"`
}

// imageResponseFormat maps the neutral vocabulary ("url", "base64",
// "b64_json") onto the wire's url / b64_json.
func imageResponseFormat(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "url":
		return "url", nil
	case "base64", "b64_json":
		return "b64_json", nil
	}
	return "", fmt.Errorf("minimax image response format %q is not supported", value)
}

func encodeImageRequest(request provider.ImageRequest, model string) ([]byte, error) {
	responseFormat, err := imageResponseFormat(request.ResponseFormat)
	if err != nil {
		return nil, err
	}
	wire := imageRequest{
		Model:             model,
		Prompt:            request.Prompt,
		N:                 request.Count,
		OutputFormat:      strings.ToLower(strings.TrimSpace(request.OutputFormat)),
		ResponseFormat:    responseFormat,
		OutputCompression: request.OutputCompression,
	}
	// Size takes WIDTHxHEIGHT or auto; an X:Y value is an aspect ratio.
	size := strings.TrimSpace(request.Size)
	if strings.Contains(size, ":") {
		wire.AspectRatio = size
	} else {
		wire.Size = size
	}
	for _, ref := range request.SubjectReferences {
		wire.Images = append(wire.Images, imageReference{ImageURL: imageReferenceURL(ref)})
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
		if _, reserved := imageReservedFields[key]; reserved {
			return nil, fmt.Errorf("minimax image extra field %q duplicates a standard field", key)
		}
		object[key] = value
	}
	return json.Marshal(object)
}

// imageReferenceURL renders one neutral reference as image_url: a URL (or
// mm_file:// id) passes through, base64 payloads become a data URI.
func imageReferenceURL(ref provider.ImageReference) string {
	if url := strings.TrimSpace(ref.URL); url != "" {
		return url
	}
	mime := strings.TrimSpace(ref.MIMEType)
	if mime == "" {
		mime = "image/png"
	}
	return "data:" + mime + ";base64," + strings.TrimSpace(ref.Base64)
}

func decodeImageResponse(raw []byte) (provider.ImageResult, error) {
	var response imageResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return provider.ImageResult{}, fmt.Errorf("minimax image: decode response: %w", err)
	}
	mime := "image/" + response.OutputFormat
	if response.OutputFormat == "" {
		mime = "image/jpeg"
	}
	result := provider.ImageResult{
		Created: response.Created,
		Usage: provider.ImageUsage{
			TotalTokens:  response.Usage.TotalTokens,
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			InputTokenDetails: provider.ImageInputTokenDetails{
				TextTokens:  response.Usage.InputTokenDetails.TextTokens,
				ImageTokens: response.Usage.InputTokenDetails.ImageTokens,
			},
			GeneratedImages: response.Usage.GeneratedImages,
		},
	}
	var firstError string
	for _, item := range response.Data {
		if item.Error != nil {
			if firstError == "" {
				firstError = item.Error.Message
			}
			continue
		}
		result.Images = append(result.Images, provider.Image{
			URL: item.URL, Base64: item.B64JSON, MIMEType: mime, RevisedPrompt: item.RevisedPrompt,
		})
	}
	if len(result.Images) == 0 {
		return provider.ImageResult{}, newRESTError("generate image", 200, response.RequestID, "", "", "no image succeeded: "+firstError)
	}
	return result, nil
}
