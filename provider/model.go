package provider

import (
	"context"
	"slices"
)

// ModelSpec describes one model exposed by a provider catalog.
type ModelSpec struct {
	ID               string       `json:"id"`
	Family           string       `json:"family,omitempty"`
	Reasoning        bool         `json:"reasoning,omitempty"`
	Capabilities     []Capability `json:"capabilities,omitempty"`
	InputModalities  []Modality   `json:"input_modalities,omitempty"`
	OutputModalities []Modality   `json:"output_modalities,omitempty"`
	ContextWindow    int          `json:"context_window,omitempty"`
	MaxTokens        int          `json:"max_tokens,omitempty"`
}

// Supports reports whether the model declares capability.
func (s ModelSpec) Supports(capability Capability) bool {
	return slices.Contains(s.Capabilities, capability)
}

// Modality names a data form a model can accept or produce.
type Modality string

const (
	MODALITY_TEXT  Modality = "text"
	MODALITY_IMAGE Modality = "image"
	MODALITY_AUDIO Modality = "audio"
	MODALITY_VIDEO Modality = "video"
)

// ModelLister discovers the models currently exposed by an upstream.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelSpec, error)
}
