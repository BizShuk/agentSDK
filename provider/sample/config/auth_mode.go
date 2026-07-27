package config

import (
	"fmt"
	"strings"
)

type AuthMode string

const (
	// AUTH_MODE_AUTO lets provider metadata select OAuth or API-key env.
	AUTH_MODE_AUTO AuthMode = "auto"
	// AUTH_MODE_APIKEY restricts resolution to API-key env.
	AUTH_MODE_APIKEY AuthMode = "api_key"
	// AUTH_MODE_OAUTH restricts resolution to OAuth env.
	AUTH_MODE_OAUTH AuthMode = "oauth"
)

func (m *AuthMode) String() string {
	return string(*m)
}

func (m *AuthMode) Set(value string) error {
	normalized := AuthMode(strings.ToLower(strings.TrimSpace(value)))
	switch normalized {
	case AUTH_MODE_AUTO, AUTH_MODE_APIKEY, AUTH_MODE_OAUTH:
		*m = normalized
		return nil
	default:
		return fmt.Errorf("auth %q must be auto, api_key, or oauth", value)
	}
}

func (m *AuthMode) Type() string {
	return "string"
}
