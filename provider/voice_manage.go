package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// VoiceManager is the voice lifecycle capability: creating custom voices
// from a text description (design) or a recorded sample (clone), and
// deleting them. The resulting voice id is then synthesized through the same
// provider's SpeechGenerator and listed through its VoiceLister.
//
// It is a separate capability rather than an optional SpeechGenerator
// extension because the speech decorator and accounting wrappers enumerate
// every combination of optional interfaces; three more would multiply them.
type VoiceManager interface {
	DesignVoice(ctx context.Context, request VoiceDesignRequest) (VoiceDesignResult, error)
	CloneVoice(ctx context.Context, request VoiceCloneRequest) (VoiceCloneResult, error)
	DeleteVoice(ctx context.Context, request VoiceDeleteRequest) error
}

// VoiceFactory builds a voice manager from registry-resolved options.
type VoiceFactory func(ResolvedConfig) (VoiceManager, error)

// VoiceDesignRequest describes a voice in prose and asks for a preview
// spoken in it. VoiceID is optional; the vendor assigns one when empty.
type VoiceDesignRequest struct {
	Prompt      string    `json:"prompt"`
	PreviewText string    `json:"preview_text"`
	VoiceID     string    `json:"voice_id,omitempty"`
	Auth        core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent design invariants.
func (r VoiceDesignRequest) Validate() error {
	if strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("voice design prompt is required")
	}
	if strings.TrimSpace(r.PreviewText) == "" {
		return fmt.Errorf("voice design preview text is required")
	}
	return nil
}

// VoiceDesignResult carries the new voice id and its decoded preview audio.
type VoiceDesignResult struct {
	VoiceID string      `json:"voice_id"`
	Preview SpeechAsset `json:"preview"`
}

// VoiceCloneRequest clones a voice from Sample under the caller-chosen
// VoiceID. PromptAudio + PromptText (a short clip and its exact transcript)
// optionally improve similarity; PreviewText with Model asks for a preview.
type VoiceCloneRequest struct {
	VoiceID             string       `json:"voice_id"`
	Sample              AudioSource  `json:"sample"`
	PromptAudio         *AudioSource `json:"prompt_audio,omitempty"`
	PromptText          string       `json:"prompt_text,omitempty"`
	PreviewText         string       `json:"preview_text,omitempty"`
	Model               string       `json:"model,omitempty"`
	NoiseReduction      bool         `json:"noise_reduction,omitempty"`
	VolumeNormalization bool         `json:"volume_normalization,omitempty"`
	Auth                core.Auth    `json:"auth,omitempty"`
}

// Validate checks provider-independent clone invariants.
func (r VoiceCloneRequest) Validate() error {
	if strings.TrimSpace(r.VoiceID) == "" {
		return fmt.Errorf("voice clone voice id is required")
	}
	if len(r.Sample.Bytes) == 0 {
		return fmt.Errorf("voice clone sample audio bytes are required")
	}
	if r.PromptAudio != nil && len(r.PromptAudio.Bytes) == 0 {
		return fmt.Errorf("voice clone prompt audio must carry bytes")
	}
	if (r.PromptAudio != nil) != (strings.TrimSpace(r.PromptText) != "") {
		return fmt.Errorf("voice clone prompt audio and prompt text must be set together")
	}
	return nil
}

// VoiceCloneResult carries the cloned voice id and, when a preview was
// requested, a short-lived preview URL.
type VoiceCloneResult struct {
	VoiceID    string `json:"voice_id"`
	PreviewURL string `json:"preview_url,omitempty"`
}

// VoiceDeleteRequest deletes one custom voice. Category names the vendor's
// voice class when deletion is scoped by it ("voice_cloning" or
// "voice_generation" on MiniMax).
type VoiceDeleteRequest struct {
	VoiceID  string    `json:"voice_id"`
	Category string    `json:"category,omitempty"`
	Auth     core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent delete invariants.
func (r VoiceDeleteRequest) Validate() error {
	if strings.TrimSpace(r.VoiceID) == "" {
		return fmt.Errorf("voice delete voice id is required")
	}
	return nil
}

type decoratedVoiceManager struct {
	manager  VoiceManager
	name     string
	decorate Decorator
}

func (d *decoratedVoiceManager) auth(ctx context.Context, auth core.Auth) (core.Auth, error) {
	return resolveRequestAuth(ctx, d.name, d.decorate, auth)
}

func (d *decoratedVoiceManager) DesignVoice(ctx context.Context, req VoiceDesignRequest) (VoiceDesignResult, error) {
	auth, err := d.auth(ctx, req.Auth)
	if err != nil {
		return VoiceDesignResult{}, err
	}
	req.Auth = auth
	return d.manager.DesignVoice(ctx, req)
}

func (d *decoratedVoiceManager) CloneVoice(ctx context.Context, req VoiceCloneRequest) (VoiceCloneResult, error) {
	auth, err := d.auth(ctx, req.Auth)
	if err != nil {
		return VoiceCloneResult{}, err
	}
	req.Auth = auth
	return d.manager.CloneVoice(ctx, req)
}

func (d *decoratedVoiceManager) DeleteVoice(ctx context.Context, req VoiceDeleteRequest) error {
	auth, err := d.auth(ctx, req.Auth)
	if err != nil {
		return err
	}
	req.Auth = auth
	return d.manager.DeleteVoice(ctx, req)
}

// WithVoiceDecorator resolves credentials before every voice lifecycle call.
func WithVoiceDecorator(name string, manager VoiceManager, decorator Decorator) VoiceManager {
	if decorator == nil || manager == nil {
		return manager
	}
	return &decoratedVoiceManager{manager: manager, name: name, decorate: decorator}
}
