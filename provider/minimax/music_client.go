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
	musicGenerationPath              = "/v1/music_generation"
	maxMusicResponseBytes      int64 = 128 << 20
	maxMusicErrorResponseBytes       = 1 << 20
	maxMusicErrorMessageRunes        = 512
)

func (p *MusicProvider) createMusic(
	ctx context.Context,
	auth core.Auth,
	payload []byte,
) (musicGenerationResponse, string, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+musicGenerationPath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return musicGenerationResponse{}, "", fmt.Errorf(
			"minimax music: build request: %w",
			err,
		)
	}
	applyMusicAuthHeaders(request, auth)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return musicGenerationResponse{}, "", fmt.Errorf(
			"minimax music: request: %w",
			err,
		)
	}
	defer response.Body.Close()

	requestID := musicRequestID(response.Header)
	if response.StatusCode/100 != 2 {
		raw, readErr := readMusicErrorResponse(
			response.Body,
			maxMusicErrorResponseBytes,
		)
		if readErr != nil {
			return musicGenerationResponse{}, requestID, readErr
		}
		return musicGenerationResponse{}, requestID, decodeMusicAPIError(
			response.StatusCode,
			requestID,
			raw,
		)
	}

	raw, err := readMusicResponse(response.Body, maxMusicResponseBytes)
	if err != nil {
		return musicGenerationResponse{}, requestID, err
	}
	var decoded musicGenerationResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return musicGenerationResponse{}, requestID, fmt.Errorf(
			"minimax music: decode response: %w",
			err,
		)
	}
	return decoded, requestID, nil
}

func readMusicResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("minimax music: read response: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf(
			"minimax music: read response: exceeds %d bytes",
			maxBytes,
		)
	}
	return raw, nil
}

func readMusicErrorResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("minimax music: read error response: %w", err)
	}
	return raw, nil
}

func applyMusicAuthHeaders(request *http.Request, auth core.Auth) {
	if token := auth.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range auth.Headers {
		if value != "" {
			request.Header.Set(key, value)
		}
	}
}

func musicRequestID(header http.Header) string {
	for _, key := range []string{"X-Request-ID", "Request-ID", "Trace-ID"} {
		if value := header.Get(key); value != "" {
			return value
		}
	}
	return ""
}

func decodeMusicAPIError(statusCode int, requestID string, raw []byte) error {
	var envelope struct {
		BaseResp musicBaseResponse `json:"base_resp"`
		Message  string            `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return &provider.APIError{
			Provider:   "minimax",
			Operation:  "generate music",
			StatusCode: statusCode,
			Message:    "upstream request failed",
			RequestID:  requestID,
		}
	}
	if envelope.BaseResp.StatusMsg == "" {
		envelope.BaseResp.StatusMsg = envelope.Message
	}
	return newMusicAPIError(statusCode, requestID, envelope.BaseResp)
}

func newMusicAPIError(
	statusCode int,
	requestID string,
	baseResp musicBaseResponse,
) error {
	message := boundMusicErrorMessage(baseResp.StatusMsg)
	if message == "" {
		message = "upstream request failed"
	}
	code := ""
	if baseResp.StatusCode != 0 {
		code = strconv.Itoa(baseResp.StatusCode)
	}
	return &provider.APIError{
		Provider:   "minimax",
		Operation:  "generate music",
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		RequestID:  requestID,
	}
}

func boundMusicErrorMessage(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) <= maxMusicErrorMessageRunes {
		return string(runes)
	}
	return string(runes[:maxMusicErrorMessageRunes])
}
