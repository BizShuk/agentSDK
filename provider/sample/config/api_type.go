package config

import (
	"fmt"
	"strings"
)

type APIType string

const (
	// API_TYPE_CHAT selects blocking model generation.
	API_TYPE_CHAT APIType = "chat"
	// API_TYPE_IMAGE selects image generation.
	API_TYPE_IMAGE APIType = "image"
	// API_TYPE_AUDIO reserves the explicit unsupported audio branch.
	API_TYPE_AUDIO APIType = "audio"
)

func (t *APIType) String() string {
	return string(*t)
}

func (t *APIType) Set(value string) error {
	normalized := APIType(strings.ToLower(strings.TrimSpace(value)))
	switch normalized {
	case API_TYPE_CHAT, API_TYPE_IMAGE, API_TYPE_AUDIO:
		*t = normalized
		return nil
	default:
		return fmt.Errorf("type %q must be chat, image, or audio", value)
	}
}

func (t *APIType) Type() string {
	return "string"
}
