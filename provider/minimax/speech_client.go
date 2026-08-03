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
	speechGenerationPath              = "/v1/t2a_v2"
	maxSpeechResponseBytes      int64 = 128 << 20
	maxSpeechErrorResponseBytes int64 = 1 << 20
	maxSpeechErrorMessageRunes        = 512
)

func (p *SpeechProvider) createSpeech(
	ctx context.Context,
	auth core.Auth,
	payload []byte,
) (speechGenerationResponse, string, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+speechGenerationPath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return speechGenerationResponse{}, "", fmt.Errorf(
			"minimax speech: build request: %w",
			err,
		)
	}
	applySpeechAuthHeaders(request, auth)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return speechGenerationResponse{}, "", fmt.Errorf(
			"minimax speech: request: %w",
			err,
		)
	}
	defer response.Body.Close()

	id := speechRequestID(response.Header)
	if response.StatusCode/100 != 2 {
		raw, readErr := readSpeechErrorResponse(response.Body, maxSpeechErrorResponseBytes)
		if readErr != nil {
			return speechGenerationResponse{}, id, readErr
		}
		return speechGenerationResponse{}, id, decodeSpeechAPIError(
			response.StatusCode,
			id,
			raw,
		)
	}

	raw, err := readSpeechResponse(response.Body, maxSpeechResponseBytes)
	if err != nil {
		return speechGenerationResponse{}, id, err
	}
	var decoded speechGenerationResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return speechGenerationResponse{}, id, fmt.Errorf(
			"minimax speech: decode response: %w",
			err,
		)
	}
	return decoded, id, nil
}

func readSpeechResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("minimax speech: read response: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf(
			"minimax speech: read response: exceeds %d bytes",
			maxBytes,
		)
	}
	return raw, nil
}

func readSpeechErrorResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("minimax speech: read error response: %w", err)
	}
	return raw, nil
}

func applySpeechAuthHeaders(request *http.Request, auth core.Auth) {
	if token := auth.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range auth.Headers {
		if value != "" {
			request.Header.Set(key, value)
		}
	}
}

func speechRequestID(header http.Header) string {
	for _, key := range []string{"X-Request-ID", "Request-ID", "Trace-ID"} {
		if value := header.Get(key); value != "" {
			return value
		}
	}
	return ""
}

func decodeSpeechAPIError(statusCode int, id string, raw []byte) error {
	var envelope struct {
		BaseResp speechBaseResponse `json:"base_resp"`
		Message  string             `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return &provider.APIError{
			Provider:   "minimax",
			Operation:  "generate speech",
			StatusCode: statusCode,
			Message:    "upstream request failed",
			RequestID:  id,
		}
	}
	if envelope.BaseResp.StatusMsg == "" {
		envelope.BaseResp.StatusMsg = envelope.Message
	}
	return newSpeechAPIError(statusCode, id, envelope.BaseResp)
}

func newSpeechAPIError(statusCode int, id string, baseResp speechBaseResponse) error {
	message := boundSpeechErrorMessage(baseResp.StatusMsg)
	if message == "" {
		message = "upstream request failed"
	}
	code := ""
	if baseResp.StatusCode != 0 {
		code = strconv.Itoa(baseResp.StatusCode)
	}
	return &provider.APIError{
		Provider:   "minimax",
		Operation:  "generate speech",
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		RequestID:  id,
	}
}

func boundSpeechErrorMessage(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) <= maxSpeechErrorMessageRunes {
		return string(runes)
	}
	return string(runes[:maxSpeechErrorMessageRunes])
}
