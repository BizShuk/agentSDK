package minimax

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const (
	defaultMusicModel          = "music-3.0"
	defaultMusicCoverModel     = "music-cover"
	defaultMusicRequestTimeout = 5 * time.Minute

	musicPromptMaxRunes      = 2000
	musicLyricsMaxRunes      = 3500
	coverPromptMinRunes      = 10
	coverPromptMaxRunes      = 300
	coverLyricsMinRunes      = 10
	coverLyricsMaxRunes      = 1000
	musicGenerationCompleted = 2
)

// MusicProvider implements provider.MusicGenerator against MiniMax's
// non-streaming music-generation endpoint.
type MusicProvider struct {
	baseURL string
	model   string
	auth    core.Auth
	client  *http.Client
}

// NewMusic returns a MiniMax music generator from registry-resolved config.
func NewMusic(cfg provider.ResolvedConfig) (*MusicProvider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = DefaultMusicBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultMusicRequestTimeout
	}
	return &MusicProvider{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   strings.TrimSpace(cfg.Model),
		auth:    cfg.Auth,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

// GenerateMusic creates one complete MiniMax track.
func (p *MusicProvider) GenerateMusic(
	ctx context.Context,
	request provider.MusicRequest,
) (provider.MusicResult, error) {
	if err := request.Validate(); err != nil {
		return provider.MusicResult{}, err
	}
	model := p.resolveMusicModel(request)
	if err := validateMiniMaxMusicRequest(request, model); err != nil {
		return provider.MusicResult{}, err
	}

	auth := p.auth.Merge(request.Auth)
	if auth.Token() == "" {
		return provider.MusicResult{}, fmt.Errorf("minimax music credential is required")
	}
	payload, err := encodeMusicRequest(request, model)
	if err != nil {
		return provider.MusicResult{}, err
	}
	response, requestID, err := p.createMusic(ctx, auth, payload)
	if err != nil {
		return provider.MusicResult{}, err
	}
	if response.BaseResp.StatusCode != 0 {
		return provider.MusicResult{}, newMusicAPIError(
			http.StatusOK,
			requestID,
			response.BaseResp,
		)
	}
	return foldMusicResponse(response, request)
}

func (p *MusicProvider) resolveMusicModel(request provider.MusicRequest) string {
	if model := strings.TrimSpace(request.Model); model != "" {
		return model
	}
	if p.model != "" {
		return p.model
	}
	if hasMusicCoverSource(request) {
		return defaultMusicCoverModel
	}
	return defaultMusicModel
}

func validateMiniMaxMusicRequest(request provider.MusicRequest, model string) error {
	cover := isMusicCoverModel(model)
	if !cover && hasMusicCoverSource(request) && isKnownTextMusicModel(model) {
		return fmt.Errorf("minimax music cover source requires a music-cover model")
	}
	if cover {
		return validateMiniMaxMusicCoverRequest(request)
	}
	return validateMiniMaxTextMusicRequest(request, model)
}

func validateMiniMaxTextMusicRequest(request provider.MusicRequest, model string) error {
	promptRunes := trimmedRuneCount(request.Prompt)
	if promptRunes > musicPromptMaxRunes {
		return fmt.Errorf(
			"minimax music prompt has %d runes, exceeds limit of %d",
			promptRunes,
			musicPromptMaxRunes,
		)
	}
	lyricsRunes := trimmedRuneCount(request.Lyrics)
	if lyricsRunes > musicLyricsMaxRunes {
		return fmt.Errorf(
			"minimax music lyrics have %d runes, exceed limit of %d",
			lyricsRunes,
			musicLyricsMaxRunes,
		)
	}
	if request.Instrumental && promptRunes == 0 {
		return fmt.Errorf("minimax music prompt is required for instrumental generation")
	}
	if request.LyricsOptimizer && lyricsRunes == 0 && promptRunes == 0 {
		return fmt.Errorf("minimax music prompt is required when lyrics optimizer is enabled")
	}
	if isKnownTextMusicModel(model) &&
		!request.Instrumental &&
		!request.LyricsOptimizer &&
		lyricsRunes == 0 {
		return fmt.Errorf("minimax music lyrics are required")
	}
	return validateMiniMaxMusicOutput(request)
}

func validateMiniMaxMusicCoverRequest(request provider.MusicRequest) error {
	if !hasMusicCoverSource(request) {
		return fmt.Errorf("minimax music cover source is required")
	}
	promptRunes := trimmedRuneCount(request.Prompt)
	if promptRunes < coverPromptMinRunes || promptRunes > coverPromptMaxRunes {
		return fmt.Errorf(
			"minimax music cover prompt must contain between %d and %d runes",
			coverPromptMinRunes,
			coverPromptMaxRunes,
		)
	}
	lyricsRunes := trimmedRuneCount(request.Lyrics)
	if lyricsRunes > 0 &&
		(lyricsRunes < coverLyricsMinRunes || lyricsRunes > coverLyricsMaxRunes) {
		return fmt.Errorf(
			"minimax music cover lyrics must contain between %d and %d runes",
			coverLyricsMinRunes,
			coverLyricsMaxRunes,
		)
	}
	if strings.TrimSpace(request.CoverFeatureID) != "" && lyricsRunes == 0 {
		return fmt.Errorf("minimax music cover feature id requires lyrics")
	}
	if request.LyricsOptimizer {
		return fmt.Errorf("minimax music lyrics optimizer is not supported for cover generation")
	}
	if request.Instrumental {
		return fmt.Errorf("minimax music instrumental mode is not supported for cover generation")
	}
	return validateMiniMaxMusicOutput(request)
}

func validateMiniMaxMusicOutput(request provider.MusicRequest) error {
	switch strings.ToLower(strings.TrimSpace(request.OutputFormat)) {
	case "", "hex", "url":
	default:
		return fmt.Errorf(
			"minimax music output format %q is not supported",
			request.OutputFormat,
		)
	}
	if !allowedMusicInteger(
		request.AudioSetting.SampleRate,
		16000,
		24000,
		32000,
		44100,
	) {
		return fmt.Errorf(
			"minimax music sample rate %d is not supported",
			request.AudioSetting.SampleRate,
		)
	}
	if !allowedMusicInteger(
		request.AudioSetting.Bitrate,
		32000,
		64000,
		128000,
		256000,
	) {
		return fmt.Errorf(
			"minimax music bitrate %d is not supported",
			request.AudioSetting.Bitrate,
		)
	}
	switch strings.ToLower(strings.TrimSpace(request.AudioSetting.Format)) {
	case "", "mp3", "wav", "pcm":
	default:
		return fmt.Errorf(
			"minimax music audio format %q is not supported",
			request.AudioSetting.Format,
		)
	}
	return nil
}

func allowedMusicInteger(value int, allowed ...int) bool {
	if value == 0 {
		return true
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func hasMusicCoverSource(request provider.MusicRequest) bool {
	return strings.TrimSpace(request.AudioURL) != "" ||
		strings.TrimSpace(request.AudioBase64) != "" ||
		strings.TrimSpace(request.CoverFeatureID) != ""
}

func isMusicCoverModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "music-cover", "music-cover-free":
		return true
	default:
		return false
	}
}

func isKnownTextMusicModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "music-3.0", "music-3.0-free", "music-2.6", "music-2.6-free":
		return true
	default:
		return false
	}
}

func trimmedRuneCount(value string) int {
	return utf8.RuneCountInString(strings.TrimSpace(value))
}
