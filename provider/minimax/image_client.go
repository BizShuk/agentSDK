package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const (
	imageGenerationPath              = "/v1/image_generation"
	maxImageResponseBytes      int64 = 128 << 20
	maxImageErrorResponseBytes int64 = 1 << 20
	maxImageErrorMessageRunes        = 512
)

func (p *ImageProvider) createImage(
	ctx context.Context,
	auth core.Auth,
	payload []byte,
) (imageGenerationResponse, string, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+imageGenerationPath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return imageGenerationResponse{}, "", fmt.Errorf(
			"minimax image: build request: %w",
			err,
		)
	}
	applyImageAuthHeaders(request, auth)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return imageGenerationResponse{}, "", fmt.Errorf(
			"minimax image: request: %w",
			err,
		)
	}
	defer response.Body.Close()

	id := imageRequestID(response.Header)
	if response.StatusCode/100 != 2 {
		raw, readErr := readImageErrorResponse(response.Body, maxImageErrorResponseBytes)
		if readErr != nil {
			return imageGenerationResponse{}, id, readErr
		}
		return imageGenerationResponse{}, id, decodeImageAPIError(
			response.StatusCode,
			id,
			raw,
		)
	}

	raw, err := readImageResponse(response.Body, maxImageResponseBytes)
	if err != nil {
		return imageGenerationResponse{}, id, err
	}
	var decoded imageGenerationResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return imageGenerationResponse{}, id, fmt.Errorf(
			"minimax image: decode response: %w",
			err,
		)
	}
	if decoded.ID != "" {
		id = decoded.ID
	}
	return decoded, id, nil
}

func readImageResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("minimax image: read response: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf(
			"minimax image: read response: exceeds %d bytes",
			maxBytes,
		)
	}
	return raw, nil
}

func readImageErrorResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("minimax image: read error response: %w", err)
	}
	return raw, nil
}

func applyImageAuthHeaders(request *http.Request, auth core.Auth) {
	if token := auth.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range auth.Headers {
		if value != "" {
			request.Header.Set(key, value)
		}
	}
}

func imageRequestID(header http.Header) string {
	for _, key := range []string{"X-Request-ID", "Request-ID", "Trace-ID"} {
		if value := header.Get(key); value != "" {
			return value
		}
	}
	return ""
}

func decodeImageAPIError(statusCode int, id string, raw []byte) error {
	var envelope struct {
		BaseResp imageBaseResponse `json:"base_resp"`
		Message  string            `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return &provider.APIError{
			Provider:   "minimax",
			Operation:  "generate image",
			StatusCode: statusCode,
			Message:    "upstream request failed",
			RequestID:  id,
		}
	}
	if envelope.BaseResp.StatusMsg == "" {
		envelope.BaseResp.StatusMsg = envelope.Message
	}
	return newImageAPIError(statusCode, id, envelope.BaseResp)
}

func newImageAPIError(statusCode int, id string, baseResp imageBaseResponse) error {
	message := boundImageErrorMessage(baseResp.StatusMsg)
	if message == "" {
		message = "upstream request failed"
	}
	code := ""
	if baseResp.StatusCode != 0 {
		code = strconv.Itoa(baseResp.StatusCode)
	}
	return &provider.APIError{
		Provider:   "minimax",
		Operation:  "generate image",
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		RequestID:  id,
	}
}

func boundImageErrorMessage(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) <= maxImageErrorMessageRunes {
		return string(runes)
	}
	return string(runes[:maxImageErrorMessageRunes])
}
