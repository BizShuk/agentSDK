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
	// API_TYPE_MUSIC selects non-streaming music generation.
	API_TYPE_MUSIC APIType = "music"
	// API_TYPE_SPEECH selects text-to-speech synthesis.
	API_TYPE_SPEECH APIType = "speech"
	// API_TYPE_TRANSCRIBE selects speech-to-text transcription.
	API_TYPE_TRANSCRIBE APIType = "transcribe"
)

// APITypeList is the human-facing enumeration used in flag help and errors.
const APITypeList = "chat, image, music, speech, or transcribe"

func (t *APIType) String() string {
	return string(*t)
}

func (t *APIType) Set(value string) error {
	normalized := APIType(strings.ToLower(strings.TrimSpace(value)))
	switch normalized {
	case API_TYPE_CHAT,
		API_TYPE_IMAGE,
		API_TYPE_MUSIC,
		API_TYPE_SPEECH,
		API_TYPE_TRANSCRIBE:
		*t = normalized
		return nil
	default:
		return fmt.Errorf("type %q must be %s", value, APITypeList)
	}
}

func (t *APIType) Type() string {
	return "string"
}
