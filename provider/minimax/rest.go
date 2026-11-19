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

// rest.go is the shared transport for MiniMax's account-root REST surfaces
// (speech_to_text, voice_design, voice_clone, files/upload, delete_voice,
// images/*). They share one host, one Bearer scheme, and two error shapes:
// the legacy `base_resp` envelope (status_code != 0 on HTTP 200) and the
// OpenAI-style `{"error":{"type","message","http_code"},"request_id"}` body
// the newer endpoints return with a real HTTP status.

const (
	maxRESTErrorResponseBytes int64 = 1 << 20
	maxRESTErrorMessageRunes        = 512
)

// restBaseResponse mirrors the vendor's shared base_resp envelope.
type restBaseResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

// restClient posts to one MiniMax account root.
type restClient struct {
	baseURL string
	client  *http.Client
}

// accountBaseURL resolves the REST root shared by every non-chat surface: an
// empty base falls back to DefaultSpeechBaseURL, and a trailing "/anthropic"
// (the chat surface one segment below the account root) is trimmed.
func accountBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return DefaultSpeechBaseURL
	}
	return strings.TrimSuffix(trimmed, anthropicCompatSuffix)
}

// post sends one request and returns the bounded success body. A non-2xx
// status, or a 2xx body whose base_resp.status_code is non-zero, becomes a
// typed *provider.APIError.
func (c restClient) post(
	ctx context.Context,
	auth core.Auth,
	operation string,
	path string,
	contentType string,
	body []byte,
	maxBytes int64,
) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("minimax %s: build request: %w", operation, err)
	}
	applySpeechAuthHeaders(request, auth)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", contentType)
	return c.do(request, operation, maxBytes)
}

// do executes a prepared request; callers that need extra headers build the
// request themselves and hand it over.
func (c restClient) do(request *http.Request, operation string, maxBytes int64) ([]byte, error) {
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("minimax %s: request: %w", operation, err)
	}
	defer response.Body.Close()

	id := speechRequestID(response.Header)
	if response.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, maxRESTErrorResponseBytes))
		return nil, decodeRESTError(operation, response.StatusCode, id, raw)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("minimax %s: read response: %w", operation, err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("minimax %s: read response: exceeds %d bytes", operation, maxBytes)
	}
	var envelope struct {
		BaseResp *restBaseResponse `json:"base_resp"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.BaseResp != nil && envelope.BaseResp.StatusCode != 0 {
		return nil, newRESTError(operation, response.StatusCode, id, strconv.Itoa(envelope.BaseResp.StatusCode), "", envelope.BaseResp.StatusMsg)
	}
	return raw, nil
}

func (c restClient) postJSON(
	ctx context.Context,
	auth core.Auth,
	operation string,
	path string,
	payload any,
	maxBytes int64,
) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("minimax %s: encode request: %w", operation, err)
	}
	return c.post(ctx, auth, operation, path, "application/json", body, maxBytes)
}

func decodeRESTError(operation string, statusCode int, id string, raw []byte) error {
	var envelope struct {
		BaseResp  restBaseResponse `json:"base_resp"`
		RequestID string           `json:"request_id"`
		Message   string           `json:"message"`
		Error     struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return newRESTError(operation, statusCode, id, "", "", "upstream request failed")
	}
	if envelope.RequestID != "" {
		id = envelope.RequestID
	}
	code := ""
	if envelope.BaseResp.StatusCode != 0 {
		code = strconv.Itoa(envelope.BaseResp.StatusCode)
	}
	message := envelope.Error.Message
	if message == "" {
		message = envelope.BaseResp.StatusMsg
	}
	if message == "" {
		message = envelope.Message
	}
	return newRESTError(operation, statusCode, id, code, envelope.Error.Type, message)
}

func newRESTError(operation string, statusCode int, id, code, errorType, message string) error {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > maxRESTErrorMessageRunes {
		runes = runes[:maxRESTErrorMessageRunes]
	}
	message = string(runes)
	if message == "" {
		message = "upstream request failed"
	}
	return &provider.APIError{
		Provider:   "minimax",
		Operation:  operation,
		StatusCode: statusCode,
		Code:       code,
		Type:       errorType,
		Message:    message,
		RequestID:  id,
	}
}
