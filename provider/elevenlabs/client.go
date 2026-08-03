package elevenlabs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// Both capabilities share one host, one credential header, and one error
// envelope, so the transport helpers live here rather than being duplicated
// per endpoint file.
const (
	maxAudioResponseBytes      int64 = 128 << 20
	maxTranscriptResponseBytes int64 = 32 << 20
	maxErrorResponseBytes      int64 = 1 << 20
	maxErrorMessageRunes             = 512
)

// applyAuthHeaders writes the vendor credential header. The token is never
// copied anywhere else — errors are built from the response body alone.
func applyAuthHeaders(request *http.Request, auth core.Auth) {
	if token := auth.Token(); token != "" {
		request.Header.Set(APIKeyHeader, token)
	}
	for key, value := range auth.Headers {
		if value != "" {
			request.Header.Set(key, value)
		}
	}
}

func requestID(header http.Header) string {
	for _, key := range []string{"X-Request-ID", "Request-ID", "X-Trace-ID"} {
		if value := header.Get(key); value != "" {
			return value
		}
	}
	return ""
}

func readBoundedResponse(operation string, reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs %s: read response: %w", operation, err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf(
			"elevenlabs %s: read response: exceeds %d bytes",
			operation,
			maxBytes,
		)
	}
	return raw, nil
}

func readErrorResponse(operation string, reader io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs %s: read error response: %w", operation, err)
	}
	return raw, nil
}

// errorEnvelope is the FastAPI-shaped body ElevenLabs returns on failure.
// `detail` is a string on some routes and an object on others, so it stays
// raw until decodeAPIError picks the shape apart.
type errorEnvelope struct {
	Detail json.RawMessage `json:"detail,omitempty"`
}

type errorDetail struct {
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

// decodeAPIError turns a non-2xx response into a typed, bounded error. Only
// the vendor's own status/message fields survive; anything unrecognized is
// truncated to a fixed rune budget so an oversized or unexpected upstream body
// cannot reach a log line intact.
func decodeAPIError(operation string, statusCode int, id string, raw []byte) error {
	apiErr := &provider.APIError{
		Provider:   "elevenlabs",
		Operation:  operation,
		StatusCode: statusCode,
		Message:    "upstream request failed",
		RequestID:  id,
	}

	var envelope errorEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Detail) == 0 {
		return apiErr
	}

	var text string
	if err := json.Unmarshal(envelope.Detail, &text); err == nil {
		if message := boundErrorMessage(text); message != "" {
			apiErr.Message = message
		}
		return apiErr
	}

	var detail errorDetail
	if err := json.Unmarshal(envelope.Detail, &detail); err == nil &&
		(detail.Status != "" || detail.Message != "") {
		apiErr.Code = boundErrorMessage(detail.Status)
		if message := boundErrorMessage(detail.Message); message != "" {
			apiErr.Message = message
		}
		return apiErr
	}

	if message := boundErrorMessage(string(envelope.Detail)); message != "" {
		apiErr.Message = message
	}
	return apiErr
}

func boundErrorMessage(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) <= maxErrorMessageRunes {
		return string(runes)
	}
	return string(runes[:maxErrorMessageRunes])
}
