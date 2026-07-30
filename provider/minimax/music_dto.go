package minimax

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

type musicGenerationRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt,omitempty"`
	Lyrics string `json:"lyrics,omitempty"`

	AudioURL       string `json:"audio_url,omitempty"`
	AudioBase64    string `json:"audio_base64,omitempty"`
	CoverFeatureID string `json:"cover_feature_id,omitempty"`

	LyricsOptimizer bool `json:"lyrics_optimizer,omitempty"`
	Instrumental    bool `json:"is_instrumental,omitempty"`

	OutputFormat string             `json:"output_format,omitempty"`
	AudioSetting *musicAudioSetting `json:"audio_setting,omitempty"`
}

type musicAudioSetting struct {
	SampleRate int    `json:"sample_rate,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Format     string `json:"format,omitempty"`
}

type musicBaseResponse struct {
	StatusCode int    `json:"status_code,omitempty"`
	StatusMsg  string `json:"status_msg,omitempty"`
}

type musicGenerationResponse struct {
	Data struct {
		Audio  string `json:"audio"`
		Status int    `json:"status"`
	} `json:"data"`
	TraceID   string `json:"trace_id"`
	ExtraInfo struct {
		Duration   int64 `json:"music_duration"`
		SampleRate int   `json:"music_sample_rate"`
		Channels   int   `json:"music_channel"`
		Bitrate    int   `json:"bitrate"`
		Size       int64 `json:"music_size"`
	} `json:"extra_info"`
	BaseResp musicBaseResponse `json:"base_resp"`
}

func encodeMusicRequest(request provider.MusicRequest, model string) ([]byte, error) {
	var audioSetting *musicAudioSetting
	if request.AudioSetting != (provider.MusicAudioSetting{}) {
		audioSetting = &musicAudioSetting{
			SampleRate: request.AudioSetting.SampleRate,
			Bitrate:    request.AudioSetting.Bitrate,
			Format:     strings.ToLower(strings.TrimSpace(request.AudioSetting.Format)),
		}
	}
	raw, err := json.Marshal(musicGenerationRequest{
		Model:           model,
		Prompt:          request.Prompt,
		Lyrics:          request.Lyrics,
		AudioURL:        request.AudioURL,
		AudioBase64:     request.AudioBase64,
		CoverFeatureID:  request.CoverFeatureID,
		LyricsOptimizer: request.LyricsOptimizer,
		Instrumental:    request.Instrumental,
		OutputFormat:    strings.ToLower(strings.TrimSpace(request.OutputFormat)),
		AudioSetting:    audioSetting,
	})
	if err != nil {
		return nil, fmt.Errorf("minimax music: encode request: %w", err)
	}
	return raw, nil
}

func foldMusicResponse(
	response musicGenerationResponse,
	request provider.MusicRequest,
) (provider.MusicResult, error) {
	if response.Data.Status != musicGenerationCompleted {
		return provider.MusicResult{}, fmt.Errorf(
			"minimax music: generation returned status %d, want %d",
			response.Data.Status,
			musicGenerationCompleted,
		)
	}
	if strings.TrimSpace(response.Data.Audio) == "" {
		return provider.MusicResult{}, fmt.Errorf(
			"minimax music: completed response has no audio",
		)
	}

	format := strings.ToLower(strings.TrimSpace(request.AudioSetting.Format))
	if format == "" {
		format = "mp3"
	}
	result := provider.MusicResult{
		Audio:   provider.MusicAsset{Format: format},
		Status:  response.Data.Status,
		TraceID: response.TraceID,
		Info: provider.MusicInfo{
			DurationMilliseconds: response.ExtraInfo.Duration,
			SampleRate:           response.ExtraInfo.SampleRate,
			Channels:             response.ExtraInfo.Channels,
			Bitrate:              response.ExtraInfo.Bitrate,
			SizeBytes:            response.ExtraInfo.Size,
		},
	}
	if strings.EqualFold(strings.TrimSpace(request.OutputFormat), "url") {
		result.Audio.URL = response.Data.Audio
	} else {
		result.Audio.Hex = response.Data.Audio
	}
	return result, nil
}
