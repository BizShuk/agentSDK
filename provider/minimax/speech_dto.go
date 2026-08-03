package minimax

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

type speechGenerationRequest struct {
	Model  string `json:"model"`
	Text   string `json:"text"`
	Stream bool   `json:"stream"`

	VoiceSetting *speechVoiceSetting `json:"voice_setting,omitempty"`
	AudioSetting *speechAudioSetting `json:"audio_setting,omitempty"`
}

// speechVoiceSetting carries only the knobs MiniMax names. The neutral
// contract's Stability / Similarity / Style have no t2a_v2 counterpart and are
// dropped rather than mapped onto vol or pitch, which mean something else.
type speechVoiceSetting struct {
	VoiceID string  `json:"voice_id,omitempty"`
	Speed   float64 `json:"speed,omitempty"`
	Vol     float64 `json:"vol,omitempty"`
	Pitch   int     `json:"pitch,omitempty"`
}

type speechAudioSetting struct {
	SampleRate int    `json:"sample_rate,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Format     string `json:"format,omitempty"`
	Channel    int    `json:"channel,omitempty"`
}

type speechBaseResponse struct {
	StatusCode int    `json:"status_code,omitempty"`
	StatusMsg  string `json:"status_msg,omitempty"`
}

type speechGenerationResponse struct {
	Data struct {
		Audio  string `json:"audio"`
		Status int    `json:"status"`
	} `json:"data"`
	ExtraInfo struct {
		AudioLength     int64 `json:"audio_length"`
		AudioSampleRate int   `json:"audio_sample_rate"`
		AudioSize       int64 `json:"audio_size"`
		AudioBitrate    int   `json:"audio_bitrate"`
		AudioChannel    int   `json:"audio_channel"`
	} `json:"extra_info"`
	TraceID  string             `json:"trace_id"`
	BaseResp speechBaseResponse `json:"base_resp"`
}

func encodeSpeechRequest(
	request provider.SpeechRequest,
	model string,
	format string,
	sampleRate int,
) ([]byte, error) {
	voiceSetting := &speechVoiceSetting{
		VoiceID: resolveSpeechVoiceID(request.Voice),
		Speed:   request.VoiceSetting.Speed,
	}
	var audioSetting *speechAudioSetting
	if format != "" || sampleRate > 0 {
		audioSetting = &speechAudioSetting{
			SampleRate: sampleRate,
			Format:     format,
		}
	}
	raw, err := json.Marshal(speechGenerationRequest{
		Model:        model,
		Text:         request.Text,
		Stream:       false,
		VoiceSetting: voiceSetting,
		AudioSetting: audioSetting,
	})
	if err != nil {
		return nil, fmt.Errorf("minimax speech: encode request: %w", err)
	}
	return raw, nil
}

func resolveSpeechVoiceID(voice string) string {
	if trimmed := strings.TrimSpace(voice); trimmed != "" {
		return trimmed
	}
	return defaultSpeechVoiceID
}

// foldSpeechResponse decodes the vendor's hex payload into the canonical
// bytes SpeechAsset carries, so no caller has to know t2a_v2 ships audio as a
// hex string.
func foldSpeechResponse(
	response speechGenerationResponse,
	format string,
) (provider.SpeechResult, error) {
	encoded := strings.TrimSpace(response.Data.Audio)
	if encoded == "" {
		return provider.SpeechResult{}, fmt.Errorf("minimax speech: response has no audio")
	}
	audio, err := hex.DecodeString(encoded)
	if err != nil {
		return provider.SpeechResult{}, fmt.Errorf("minimax speech: decode audio: %w", err)
	}
	if format == "" {
		format = defaultSpeechFormat
	}
	return provider.SpeechResult{
		Audio: provider.SpeechAsset{
			Bytes:  audio,
			Format: format,
		},
		Info: provider.SpeechInfo{
			DurationMs: response.ExtraInfo.AudioLength,
			SampleRate: response.ExtraInfo.AudioSampleRate,
			Channels:   response.ExtraInfo.AudioChannel,
			Bitrate:    response.ExtraInfo.AudioBitrate,
			SizeBytes:  response.ExtraInfo.AudioSize,
		},
	}, nil
}
