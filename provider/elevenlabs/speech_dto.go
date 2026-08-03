package elevenlabs

import (
	"encoding/json"
	"fmt"

	"github.com/bizshuk/agentsdk/provider"
)

type speechRequest struct {
	Text          string               `json:"text"`
	ModelID       string               `json:"model_id,omitempty"`
	VoiceSettings *speechVoiceSettings `json:"voice_settings,omitempty"`
}

// speechVoiceSettings mirrors the vendor's field names. It is a pointer on
// speechRequest so an all-zero provider.VoiceSetting omits the object outright
// rather than sending four zeros, which ElevenLabs reads as "silence the
// voice" instead of "use the voice's own defaults".
type speechVoiceSettings struct {
	Stability       float64 `json:"stability,omitempty"`
	SimilarityBoost float64 `json:"similarity_boost,omitempty"`
	Style           float64 `json:"style,omitempty"`
	Speed           float64 `json:"speed,omitempty"`
}

func encodeSpeechRequest(request provider.SpeechRequest, model string) ([]byte, error) {
	var settings *speechVoiceSettings
	if request.VoiceSetting != (provider.VoiceSetting{}) {
		settings = &speechVoiceSettings{
			Stability:       request.VoiceSetting.Stability,
			SimilarityBoost: request.VoiceSetting.Similarity,
			Style:           request.VoiceSetting.Style,
			Speed:           request.VoiceSetting.Speed,
		}
	}
	raw, err := json.Marshal(speechRequest{
		Text:          request.Text,
		ModelID:       model,
		VoiceSettings: settings,
	})
	if err != nil {
		return nil, fmt.Errorf("elevenlabs speech: encode request: %w", err)
	}
	return raw, nil
}
