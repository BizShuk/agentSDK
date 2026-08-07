package funasr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const (
	transcribePath             = "/v1/audio/transcriptions"
	defaultRequestTimeout      = 5 * time.Minute
	maxTranscriptResponseBytes = int64(32 << 20)
	maxErrorResponseBytes      = int64(1 << 20)
	maxErrorMessageRunes       = 512
)

// TranscribeProvider implements provider.Transcriber against a FunASR
// OpenAI-compatible /v1/audio/transcriptions endpoint.
type TranscribeProvider struct {
	baseURL string
	model   string
	auth    core.Auth
	client  *http.Client
}

// NewTranscriber returns a FunASR transcriber from registry-resolved config.
func NewTranscriber(cfg provider.ResolvedConfig) (*TranscribeProvider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return &TranscribeProvider{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   strings.TrimSpace(cfg.Model),
		auth:    cfg.Auth,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

// Transcribe converts one recording into text.
//
// Two request fields have no wire representation and are rejected rather
// than silently dropped:
//   - Audio.URL: the server only accepts an uploaded file.
//   - Diarize: speaker attribution is decided by the server deployment (a
//     pinned spk model), not per request. When the deployment carries one,
//     segment speakers flow into the result without the flag.
func (p *TranscribeProvider) Transcribe(
	ctx context.Context,
	request provider.TranscribeRequest,
) (provider.TranscribeResult, error) {
	if err := request.Validate(); err != nil {
		return provider.TranscribeResult{}, err
	}
	if strings.TrimSpace(request.Audio.URL) != "" {
		return provider.TranscribeResult{}, fmt.Errorf(
			"funasr transcribe: audio URL input is not supported; send bytes",
		)
	}
	if request.Diarize {
		return provider.TranscribeResult{}, fmt.Errorf(
			"funasr transcribe: diarize is not a request-time option; " +
				"deploy the server with a speaker (spk) model instead",
		)
	}
	payload, contentType, err := encodeTranscribeRequest(
		request,
		p.resolveModel(request),
	)
	if err != nil {
		return provider.TranscribeResult{}, err
	}
	response, err := p.postTranscribe(ctx, request.Auth, payload, contentType)
	if err != nil {
		return provider.TranscribeResult{}, err
	}
	return foldTranscribeResponse(response), nil
}

func (p *TranscribeProvider) resolveModel(
	request provider.TranscribeRequest,
) string {
	if model := strings.TrimSpace(request.Model); model != "" {
		return model
	}
	if p.model != "" {
		return p.model
	}
	return DefaultTranscribeModel
}

func (p *TranscribeProvider) postTranscribe(
	ctx context.Context,
	requestAuth core.Auth,
	payload []byte,
	contentType string,
) (transcribeResponse, error) {
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+transcribePath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return transcribeResponse{}, fmt.Errorf("funasr transcribe: build request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", contentType)
	// Keyless by default; a gateway-fronted deployment reads a Bearer token.
	if token := p.auth.Merge(requestAuth).Token(); token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return transcribeResponse{}, fmt.Errorf("funasr transcribe: request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode/100 != 2 {
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxErrorResponseBytes))
		if readErr != nil {
			return transcribeResponse{}, fmt.Errorf(
				"funasr transcribe: read error response: %w", readErr,
			)
		}
		return transcribeResponse{}, decodeAPIError(response.StatusCode, raw)
	}

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxTranscriptResponseBytes+1))
	if err != nil {
		return transcribeResponse{}, fmt.Errorf("funasr transcribe: read response: %w", err)
	}
	if int64(len(raw)) > maxTranscriptResponseBytes {
		return transcribeResponse{}, fmt.Errorf(
			"funasr transcribe: response exceeds %d bytes", maxTranscriptResponseBytes,
		)
	}
	var decoded transcribeResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return transcribeResponse{}, fmt.Errorf("funasr transcribe: decode response: %w", err)
	}
	return decoded, nil
}

// decodeAPIError turns a non-2xx FastAPI response ({"detail": ...}) into a
// typed, bounded error.
func decodeAPIError(statusCode int, raw []byte) error {
	apiErr := &provider.APIError{
		Provider:   "funasr",
		Operation:  "transcribe",
		StatusCode: statusCode,
		Message:    "upstream request failed",
	}
	var envelope struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Detail != "" {
		runes := []rune(strings.TrimSpace(envelope.Detail))
		if len(runes) > maxErrorMessageRunes {
			runes = runes[:maxErrorMessageRunes]
		}
		apiErr.Message = string(runes)
	}
	return apiErr
}
