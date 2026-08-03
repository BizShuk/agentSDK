package elevenlabs

import "github.com/bizshuk/agentsdk/core"

// DefaultCatalog returns the bundled ElevenLabs model catalog.
//
// ElevenLabs has no chat surface, so this entry ships no core.ModelLister and
// the static catalog is the only model list callers see. ContextWindow and
// MaxTokens stay zero: the vendor bounds requests by characters of input text,
// not tokens, so a token figure here would be a fabricated one.
//
// The list is intentionally conservative — a model added here shows up in
// picker UIs, so new ids land in a follow-up once their API is stable.
func DefaultCatalog() []core.ModelSpec {
	return []core.ModelSpec{
		// Speech-to-text.
		{ID: "scribe_v1", Family: "scribe",
			Input: []core.Modality{core.MODALITY_AUDIO}},
		// Text-to-speech — flash is the low-latency default.
		{ID: "eleven_flash_v2_5", Family: "eleven_flash",
			Input: []core.Modality{core.MODALITY_TEXT}},
		{ID: "eleven_turbo_v2_5", Family: "eleven_turbo",
			Input: []core.Modality{core.MODALITY_TEXT}},
		{ID: "eleven_multilingual_v2", Family: "eleven_multilingual",
			Input: []core.Modality{core.MODALITY_TEXT}},
	}
}
