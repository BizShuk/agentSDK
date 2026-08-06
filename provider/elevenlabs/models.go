package elevenlabs

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/utils"
)

// Compile-time: the speech surface is what carries the catalog endpoint.
var _ provider.ModelLister = (*SpeechProvider)(nil)

// modelsPath is the vendor's catalog endpoint. It answers with a bare JSON
// array, not the {"data":[...]} envelope utils.DecodeIDList understands, so
// the decode stays local.
const modelsPath = "/v1/models"

// DefaultCatalog returns the bundled ElevenLabs model catalog.
//
// ContextWindow and MaxTokens stay zero: the vendor bounds requests by
// characters of input text, not tokens, so a token figure here would be a
// fabricated one.
func DefaultCatalog() []provider.ModelSpec {
	return []provider.ModelSpec{
		// Speech-to-text. scribe_v2_realtime is a websocket-only model and
		// is rejected by the batch /v1/speech-to-text route this adapter
		// posts to, so it is not listed.
		{ID: "scribe_v2", Family: "scribe",
			Capabilities:     []provider.Capability{provider.CAPABILITY_TRANSCRIBE},
			InputModalities:  []provider.Modality{provider.MODALITY_AUDIO},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT}},
		// Text-to-speech — flash is the low-latency default.
		{ID: "eleven_flash_v2_5", Family: "eleven_flash",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "eleven_flash_v2", Family: "eleven_flash",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "eleven_v3", Family: "eleven_v3",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "eleven_multilingual_v2", Family: "eleven_multilingual",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "eleven_turbo_v2_5", Family: "eleven_turbo",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "eleven_turbo_v2", Family: "eleven_turbo",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "eleven_english_sts_v2", Family: "eleven_sts",
			InputModalities:  []provider.Modality{provider.MODALITY_AUDIO},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "eleven_multilingual_sts_v2", Family: "eleven_sts",
			InputModalities:  []provider.Modality{provider.MODALITY_AUDIO},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
	}
}

// ListModels implements provider.ModelLister against GET /v1/models.
//
// The endpoint enumerates synthesis models only — text-to-speech and voice
// conversion. Speech-to-text (scribe) models are served by a different
// surface and never appear in the response, so the bundled STT entries are
// appended rather than being dropped by the live-drives-membership rule in
// utils.Merge. Everything else follows that rule: the live list decides
// which synthesis models exist, the bundle supplies their metadata.
func (p *SpeechProvider) ListModels(ctx context.Context) ([]provider.ModelSpec, error) {
	raw, err := utils.Fetch(ctx, p.client, p.baseURL+modelsPath, catalogHeaders(p.auth))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs list models: %w", err)
	}
	ids, err := decodeModelIDs(raw)
	if err != nil {
		return nil, err
	}
	static := DefaultCatalog()
	return appendTranscribeModels(utils.Merge(ids, static), static), nil
}

// catalogHeaders mirrors applyAuthHeaders for the header-map shape utils.Fetch
// takes. The token is only ever written onto the outgoing request.
func catalogHeaders(auth core.Auth) map[string]string {
	headers := make(map[string]string, len(auth.Headers)+1)
	if token := auth.Token(); token != "" {
		headers[APIKeyHeader] = token
	}
	for key, value := range auth.Headers {
		if value != "" {
			headers[key] = value
		}
	}
	return headers
}

// modelListEntry is the one field of the vendor's model object this adapter
// reads. The rest (languages, character limits, cost multipliers) describes
// billing and request bounds, not model identity, so it stays undecoded.
type modelListEntry struct {
	ModelID string `json:"model_id"`
}

// decodeModelIDs pulls model ids out of the bare-array catalog response. An
// empty array is an error rather than an empty slice, matching
// utils.DecodeIDList: an endpoint that answered with no models is the wrong
// endpoint, not a provider that genuinely serves nothing.
func decodeModelIDs(raw []byte) ([]string, error) {
	var entries []modelListEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("elevenlabs list models: decode: %w", err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.ModelID != "" {
			ids = append(ids, entry.ModelID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("elevenlabs list models: response carried no model ids")
	}
	return ids, nil
}

// appendTranscribeModels adds the bundled audio-input models the synthesis
// catalog cannot report, skipping any id the live list already covered.
func appendTranscribeModels(live, static []provider.ModelSpec) []provider.ModelSpec {
	seen := make(map[string]struct{}, len(live))
	for _, spec := range live {
		seen[spec.ID] = struct{}{}
	}
	for _, spec := range static {
		if _, done := seen[spec.ID]; done {
			continue
		}
		if takesAudio(spec) {
			live = append(live, spec)
		}
	}
	return live
}

func takesAudio(spec provider.ModelSpec) bool {
	return slices.Contains(spec.InputModalities, provider.MODALITY_AUDIO)
}
