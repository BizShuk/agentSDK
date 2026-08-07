package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// MusicGenerator is the optional provider capability for creating music.
// It remains separate from core.Provider because the agent runtime does not
// consume music-generation requests.
type MusicGenerator interface {
	GenerateMusic(ctx context.Context, request MusicRequest) (MusicResult, error)
}

// MusicFactory builds a music generator from registry-resolved options.
type MusicFactory func(ResolvedConfig) (MusicGenerator, error)

// MusicAudioSetting describes the requested audio encoding. Zero values let
// the concrete provider apply its own defaults.
type MusicAudioSetting struct {
	SampleRate int    `json:"sample_rate,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Format     string `json:"format,omitempty"`
}

// MusicRequest is the provider-neutral input to a non-streaming music
// generation API. Cover inputs are mutually exclusive.
type MusicRequest struct {
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt,omitempty"`
	Lyrics string `json:"lyrics,omitempty"`

	AudioURL       string `json:"audio_url,omitempty"`
	AudioBase64    string `json:"audio_base64,omitempty"`
	CoverFeatureID string `json:"cover_feature_id,omitempty"`

	LyricsOptimizer bool `json:"lyrics_optimizer,omitempty"`
	Instrumental    bool `json:"is_instrumental,omitempty"`

	OutputFormat string            `json:"output_format,omitempty"`
	AudioSetting MusicAudioSetting `json:"audio_setting,omitempty"`

	// Auth overrides construction-time or decorated credentials for this call.
	Auth core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent music request invariants.
func (r MusicRequest) Validate() error {
	if strings.TrimSpace(r.Prompt) == "" &&
		strings.TrimSpace(r.Lyrics) == "" &&
		strings.TrimSpace(r.AudioURL) == "" &&
		strings.TrimSpace(r.AudioBase64) == "" &&
		strings.TrimSpace(r.CoverFeatureID) == "" &&
		!r.LyricsOptimizer &&
		!r.Instrumental {
		return fmt.Errorf("music input is required")
	}

	coverSources := 0
	for _, source := range []string{r.AudioURL, r.AudioBase64, r.CoverFeatureID} {
		if strings.TrimSpace(source) != "" {
			coverSources++
		}
	}
	if coverSources > 1 {
		return fmt.Errorf("music cover sources are mutually exclusive")
	}
	if r.AudioSetting.SampleRate < 0 {
		return fmt.Errorf("music sample rate must not be negative")
	}
	if r.AudioSetting.Bitrate < 0 {
		return fmt.Errorf("music bitrate must not be negative")
	}
	return nil
}

// MusicAsset is the generated audio payload. A provider returns either URL or
// Hex according to the requested output format. URL lifetimes remain
// provider-defined, so callers that need durable storage must copy the asset.
type MusicAsset struct {
	URL    string `json:"url,omitempty"`
	Hex    string `json:"hex,omitempty"`
	Format string `json:"format,omitempty"`
}

// MusicInfo is the common metadata returned for a generated track.
type MusicInfo struct {
	DurationMilliseconds int64 `json:"duration_milliseconds,omitempty"`
	SampleRate           int   `json:"sample_rate,omitempty"`
	Channels             int   `json:"channels,omitempty"`
	Bitrate              int   `json:"bitrate,omitempty"`
	SizeBytes            int64 `json:"size_bytes,omitempty"`
}

// MusicUsage is the billable work completed by one music request.
type MusicUsage struct {
	GeneratedTracks      int   `json:"generated_tracks,omitempty"`
	DurationMilliseconds int64 `json:"duration_milliseconds,omitempty"`
}

// MusicResult is the folded response from one music-generation request.
type MusicResult struct {
	Audio   MusicAsset `json:"audio"`
	Status  int        `json:"status,omitempty"`
	TraceID string     `json:"trace_id,omitempty"`
	Info    MusicInfo  `json:"info,omitempty"`
	Usage   MusicUsage `json:"usage,omitempty"`
	Cost    core.Cost  `json:"cost"`
}

type decoratedMusic struct {
	generator MusicGenerator
	name      string
	decorate  Decorator
}

func (d *decoratedMusic) GenerateMusic(
	ctx context.Context,
	req MusicRequest,
) (MusicResult, error) {
	auth, err := resolveRequestAuth(ctx, d.name, d.decorate, req.Auth)
	if err != nil {
		return MusicResult{}, err
	}
	req.Auth = auth
	return d.generator.GenerateMusic(ctx, req)
}

// WithMusicDecorator returns a music generator that resolves credentials
// before every outbound request. A nil decorator returns generator unchanged.
func WithMusicDecorator(
	name string,
	generator MusicGenerator,
	decorator Decorator,
) MusicGenerator {
	if decorator == nil || generator == nil {
		return generator
	}
	return &decoratedMusic{
		generator: generator,
		name:      name,
		decorate:  decorator,
	}
}
