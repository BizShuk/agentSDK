package openaiimage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const (
	// GENERATIONS_PATH is appended to an OpenAI-compatible API base URL.
	GENERATIONS_PATH = "/images/generations"

	// MAX_RESPONSE_BYTES bounds a decoded base64 response while leaving room
	// for current high-resolution image outputs and JSON overhead.
	MAX_RESPONSE_BYTES int64 = 128 << 20

	// MAX_ERROR_RESPONSE_BYTES bounds the upstream body inspected on failure.
	MAX_ERROR_RESPONSE_BYTES int64 = 1 << 20

	// MAX_ERROR_MESSAGE_RUNES bounds text copied from an upstream error.
	MAX_ERROR_MESSAGE_RUNES = 512

	// MAX_ERROR_DETAILS_BYTES bounds structured moderation details retained for
	// callers. Oversized details are omitted instead of returning partial JSON.
	MAX_ERROR_DETAILS_BYTES = 16 << 10
)

var reservedRequestFields = map[string]struct{}{
	"model":              {},
	"prompt":             {},
	"n":                  {},
	"size":               {},
	"quality":            {},
	"response_format":    {},
	"output_format":      {},
	"output_compression": {},
	"background":         {},
	"moderation":         {},
	"user":               {},
}

var unsupportedRequestFields = map[string]struct{}{
	"partial_images": {},
	"stream":         {},
}

type request struct {
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	Count             int    `json:"n,omitempty"`
	Size              string `json:"size,omitempty"`
	Quality           string `json:"quality,omitempty"`
	ResponseFormat    string `json:"response_format,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	Background        string `json:"background,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
	User              string `json:"user,omitempty"`
}

type response struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL           string `json:"url"`
		Base64        string `json:"b64_json"`
		MIMEType      string `json:"mime_type"`
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"data"`
	Usage struct {
		TotalTokens    int   `json:"total_tokens"`
		InputTokens    int   `json:"input_tokens"`
		OutputTokens   int   `json:"output_tokens"`
		CostInUSDTicks int64 `json:"cost_in_usd_ticks"`
		InputDetails   struct {
			TextTokens  int `json:"text_tokens"`
			ImageTokens int `json:"image_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

// EncodeRequest converts a provider-neutral request to the shared JSON wire
// shape. defaultModel is used only when request.Model is empty.
func EncodeRequest(input provider.ImageRequest, defaultModel string) ([]byte, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if len(input.SubjectReferences) > 0 {
		return nil, fmt.Errorf("image subject references are not supported by this provider")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = strings.TrimSpace(defaultModel)
	}
	if model == "" {
		return nil, fmt.Errorf("image model is required")
	}

	raw, err := json.Marshal(request{
		Model:             model,
		Prompt:            input.Prompt,
		Count:             input.Count,
		Size:              input.Size,
		Quality:           input.Quality,
		ResponseFormat:    input.ResponseFormat,
		OutputFormat:      input.OutputFormat,
		OutputCompression: input.OutputCompression,
		Background:        input.Background,
		Moderation:        input.Moderation,
		User:              input.User,
	})
	if err != nil {
		return nil, fmt.Errorf("encode image request: %w", err)
	}
	if len(input.Extra) == 0 {
		return raw, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("encode image request object: %w", err)
	}
	for key, value := range input.Extra {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("image extra parameter name is required")
		}
		if _, reserved := reservedRequestFields[key]; reserved {
			return nil, fmt.Errorf("image extra parameter %q conflicts with a standard field", key)
		}
		if _, unsupported := unsupportedRequestFields[key]; unsupported {
			return nil, fmt.Errorf(
				"image extra parameter %q requires unsupported streaming behavior",
				key,
			)
		}
		if !json.Valid(value) {
			return nil, fmt.Errorf("image extra parameter %q is not valid JSON", key)
		}
		object[key] = value
	}
	raw, err = json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode image request extras: %w", err)
	}
	return raw, nil
}

// DecodeResponse converts an OpenAI-compatible image response to ImageResult.
func DecodeResponse(raw []byte) (provider.ImageResult, error) {
	var wire response
	if err := json.Unmarshal(raw, &wire); err != nil {
		return provider.ImageResult{}, fmt.Errorf("decode image response: %w", err)
	}
	if len(wire.Data) == 0 {
		return provider.ImageResult{}, fmt.Errorf("decode image response: data is empty")
	}

	images := make([]provider.Image, 0, len(wire.Data))
	for index, item := range wire.Data {
		if item.URL == "" && item.Base64 == "" {
			return provider.ImageResult{}, fmt.Errorf(
				"decode image response: data[%d] has neither url nor b64_json",
				index,
			)
		}
		images = append(images, provider.Image{
			URL:           item.URL,
			Base64:        item.Base64,
			MIMEType:      item.MIMEType,
			RevisedPrompt: item.RevisedPrompt,
		})
	}
	result := provider.ImageResult{
		Created: wire.Created,
		Images:  images,
		Usage: provider.ImageUsage{
			TotalTokens:     wire.Usage.TotalTokens,
			InputTokens:     wire.Usage.InputTokens,
			OutputTokens:    wire.Usage.OutputTokens,
			GeneratedImages: len(images),
			InputTokenDetails: provider.ImageInputTokenDetails{
				TextTokens:  wire.Usage.InputDetails.TextTokens,
				ImageTokens: wire.Usage.InputDetails.ImageTokens,
			},
		},
	}
	if wire.Usage.CostInUSDTicks > 0 {
		result.Cost = core.ExactCostFromUSDTicks(wire.Usage.CostInUSDTicks)
	}
	return result, nil
}

// ReadResponse reads a complete image response with a fixed upper bound.
func ReadResponse(reader io.Reader) ([]byte, error) {
	return readResponse(reader, MAX_RESPONSE_BYTES)
}

// ReadErrorResponse reads only a bounded prefix of an upstream error body.
// A truncated JSON document safely decodes to a generic structured APIError.
func ReadErrorResponse(reader io.Reader) ([]byte, error) {
	return readErrorResponse(reader, MAX_ERROR_RESPONSE_BYTES)
}

func readErrorResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("read image error response: %w", err)
	}
	return raw, nil
}

func readResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image response: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("read image response: exceeds %d bytes", maxBytes)
	}
	return raw, nil
}

// RequestID returns the first common upstream request-id header.
func RequestID(header http.Header) string {
	for _, key := range []string{"X-Request-ID", "Request-ID", "X-Goog-Request-ID"} {
		if value := header.Get(key); value != "" {
			return value
		}
	}
	return ""
}

// DecodeAPIError creates a structured error without exposing an unparsed
// upstream body.
func DecodeAPIError(
	providerName string,
	operation string,
	statusCode int,
	requestID string,
	raw []byte,
) error {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	var detail struct {
		Message           string          `json:"message"`
		Type              string          `json:"type"`
		Code              json.RawMessage `json:"code"`
		ModerationDetails json.RawMessage `json:"moderation_details"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Error) > 0 {
		if err := json.Unmarshal(envelope.Error, &detail); err != nil {
			_ = json.Unmarshal(envelope.Error, &detail.Message)
		}
	}
	message := boundMessage(detail.Message)
	if message == "" {
		message = "upstream request failed"
	}
	return &provider.APIError{
		Provider:   providerName,
		Operation:  operation,
		StatusCode: statusCode,
		Code:       scalarString(detail.Code),
		Type:       detail.Type,
		Message:    message,
		RequestID:  requestID,
		Details:    boundDetails(detail.ModerationDetails),
	}
}

func scalarString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	return ""
}

func boundMessage(message string) string {
	message = strings.TrimSpace(message)
	runes := []rune(message)
	if len(runes) <= MAX_ERROR_MESSAGE_RUNES {
		return message
	}
	return string(runes[:MAX_ERROR_MESSAGE_RUNES])
}

func boundDetails(details json.RawMessage) json.RawMessage {
	if len(details) == 0 || len(details) > MAX_ERROR_DETAILS_BYTES || !json.Valid(details) {
		return nil
	}
	return details
}
