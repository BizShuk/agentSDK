package provider

import (
	"encoding/json"
	"fmt"
)

// APIError is a structured, bounded error returned by an upstream provider.
// It preserves status and stable machine-readable fields without exposing the
// provider's full response body.
type APIError struct {
	Provider   string          `json:"provider"`
	Operation  string          `json:"operation"`
	StatusCode int             `json:"status_code"`
	Code       string          `json:"code,omitempty"`
	Type       string          `json:"type,omitempty"`
	Message    string          `json:"message,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	prefix := fmt.Sprintf("%s: %s: status %d", e.Provider, e.Operation, e.StatusCode)
	if e.Code != "" {
		prefix += " (" + e.Code + ")"
	}
	if e.Message != "" {
		prefix += ": " + e.Message
	}
	return prefix
}
