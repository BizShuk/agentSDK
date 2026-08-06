package benchmark

import (
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// CatalogSpecs returns the provider's bundled DefaultCatalog, or nil when the
// provider is unknown or ships none.
func CatalogSpecs(name string) []core.ModelSpec {
	specs, ok := provider.Catalog(name)
	if !ok {
		return nil
	}
	return specs
}

// KindsOf maps one DefaultCatalog model to the benchmark kinds it can serve.
// ModelSpec carries no output modality, so this mapping owns the per-provider
// naming knowledge; an empty result means the benchmark has no case that can
// drive the model (speech-to-speech, cover-from-audio, subject-to-video) or
// the adapter exposes no factory for it (Google TTS/Lyria).
func KindsOf(providerName string, spec core.ModelSpec) []Kind {
	id := strings.ToLower(spec.ID)
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "minimax":
		switch {
		case strings.HasPrefix(id, "image-"):
			return []Kind{KIND_IMAGE}
		case id == "music-cover": // conditions on reference audio the cases do not carry
			return nil
		case strings.HasPrefix(id, "music-"):
			return []Kind{KIND_MUSIC}
		case strings.HasPrefix(id, "speech-"):
			return []Kind{KIND_SPEECH}
		case strings.HasPrefix(id, "s2v"): // subject-to-video needs a subject reference
			return nil
		case strings.HasPrefix(id, "minimax-h"): // MiniMax-H3 and MiniMax-Hailuo-* video family
			return []Kind{KIND_VIDEO}
		default:
			return []Kind{KIND_CHAT}
		}
	case "elevenlabs":
		switch {
		case strings.HasPrefix(id, "scribe"):
			return []Kind{KIND_TRANSCRIBE}
		case strings.Contains(id, "sts"): // speech-to-speech has no SDK surface
			return nil
		default:
			return []Kind{KIND_SPEECH}
		}
	case "google":
		switch {
		case strings.Contains(id, "image"), strings.Contains(id, "banana"):
			return []Kind{KIND_IMAGE}
		case strings.Contains(id, "tts"), strings.Contains(id, "lyria"):
			return nil // the adapter has no speech or music factory
		default:
			return []Kind{KIND_CHAT}
		}
	case "antigravity":
		if strings.Contains(id, "image") {
			return []Kind{KIND_IMAGE}
		}
		return []Kind{KIND_CHAT}
	case "ollama":
		if strings.Contains(id, "bge") || strings.Contains(id, "embed") {
			return nil // embedding models have no chat surface
		}
		return []Kind{KIND_CHAT}
	default:
		return []Kind{KIND_CHAT}
	}
}
