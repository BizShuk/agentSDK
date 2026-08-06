package provider

import (
	"errors"
	"fmt"
)

// Capability names a provider API surface callers can discover before
// constructing a client.
type Capability string

const (
	CAPABILITY_CHAT       Capability = "chat"
	CAPABILITY_CATALOG    Capability = "catalog"
	CAPABILITY_IMAGE      Capability = "image"
	CAPABILITY_VIDEO      Capability = "video"
	CAPABILITY_MUSIC      Capability = "music"
	CAPABILITY_TRANSCRIBE Capability = "transcribe"
	CAPABILITY_SPEECH     Capability = "speech"
	CAPABILITY_LIVE       Capability = "live"
	CAPABILITY_TRANSLATE  Capability = "translate"
)

// ErrUnsupportedCapability identifies a provider that is registered but does
// not implement the requested API surface.
var ErrUnsupportedCapability = errors.New("unsupported provider capability")

// UnsupportedCapabilityError reports which provider capability is missing.
type UnsupportedCapabilityError struct {
	Provider   string
	Capability Capability
}

func (e *UnsupportedCapabilityError) Error() string {
	return fmt.Sprintf("provider %s: capability %s is not supported", e.Provider, e.Capability)
}

// Unwrap allows callers to use errors.Is(err, ErrUnsupportedCapability).
func (e *UnsupportedCapabilityError) Unwrap() error {
	return ErrUnsupportedCapability
}

// Supports reports whether this registry entry implements capability.
func (e Entry) Supports(capability Capability) bool {
	switch capability {
	case CAPABILITY_CHAT:
		return e.New != nil
	case CAPABILITY_CATALOG:
		return e.Catalog != nil
	case CAPABILITY_IMAGE:
		return e.NewImage != nil
	case CAPABILITY_VIDEO:
		return e.NewVideo != nil
	case CAPABILITY_MUSIC:
		return e.NewMusic != nil
	case CAPABILITY_TRANSCRIBE:
		return e.NewTranscriber != nil
	case CAPABILITY_SPEECH:
		return e.NewSpeech != nil
	case CAPABILITY_LIVE:
		return e.NewLive != nil
	case CAPABILITY_TRANSLATE:
		return e.NewTranslate != nil
	default:
		return false
	}
}

// Capabilities returns this entry's supported API surfaces in stable order.
func (e Entry) Capabilities() []Capability {
	ordered := []Capability{
		CAPABILITY_CHAT,
		CAPABILITY_CATALOG,
		CAPABILITY_IMAGE,
		CAPABILITY_VIDEO,
		CAPABILITY_MUSIC,
		CAPABILITY_TRANSCRIBE,
		CAPABILITY_SPEECH,
		CAPABILITY_LIVE,
		CAPABILITY_TRANSLATE,
	}
	out := make([]Capability, 0, len(ordered))
	for _, capability := range ordered {
		if e.Supports(capability) {
			out = append(out, capability)
		}
	}
	return out
}

// Capabilities returns the supported API surfaces for a registered provider.
func Capabilities(name string) ([]Capability, bool) {
	entry, ok := Lookup(name)
	if !ok {
		return nil, false
	}
	return entry.Capabilities(), true
}
