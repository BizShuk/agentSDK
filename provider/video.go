package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// VideoMode identifies the input strategy used to create a video.
type VideoMode string

const (
	VIDEO_MODE_TEXT      VideoMode = "text"
	VIDEO_MODE_IMAGE     VideoMode = "image"
	VIDEO_MODE_START_END VideoMode = "startend"
	VIDEO_MODE_SUBJECT   VideoMode = "subject"
)

// VideoGenerator is the optional provider capability for creating a video and
// persisting it to a caller-selected local path.
type VideoGenerator interface {
	// MaxPromptLength returns the provider's prompt limit in runes. Zero means
	// the provider does not publish a limit.
	MaxPromptLength() int
	GenerateVideo(ctx context.Context, request VideoRequest) (VideoResult, error)
}

// VideoFactory builds a video generator from registry-resolved options.
type VideoFactory func(ResolvedConfig) (VideoGenerator, error)

// VideoRequest is the provider-neutral input to an asynchronous video
// generation API. Empty optional fields let the adapter apply its own model,
// format, polling, duration, and resolution defaults.
type VideoRequest struct {
	Mode VideoMode `json:"mode"`

	Model      string `json:"model,omitempty"`
	Prompt     string `json:"prompt"`
	Duration   int    `json:"duration,omitempty"`
	Resolution string `json:"resolution,omitempty"`

	FirstFrameURL    string   `json:"first_frame_url,omitempty"`
	LastFrameURL     string   `json:"last_frame_url,omitempty"`
	SubjectImageURLs []string `json:"subject_image_urls,omitempty"`

	OutputPath   string        `json:"output_path"`
	OutputFormat string        `json:"output_format,omitempty"`
	PollInterval time.Duration `json:"poll_interval,omitempty"`

	// Auth overrides construction-time or decorated credentials for this call.
	Auth core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent video request invariants.
func (r VideoRequest) Validate() error {
	switch r.Mode {
	case "":
		return fmt.Errorf("video mode is required")
	case VIDEO_MODE_TEXT, VIDEO_MODE_IMAGE, VIDEO_MODE_START_END, VIDEO_MODE_SUBJECT:
	default:
		return fmt.Errorf("unsupported video mode %q", r.Mode)
	}
	if strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("video prompt is required")
	}
	if strings.TrimSpace(r.OutputPath) == "" {
		return fmt.Errorf("video output path is required")
	}
	if r.Duration < 0 {
		return fmt.Errorf("video duration must not be negative")
	}

	switch r.Mode {
	case VIDEO_MODE_IMAGE:
		if strings.TrimSpace(r.FirstFrameURL) == "" {
			return fmt.Errorf("video first frame URL is required for %s mode", r.Mode)
		}
	case VIDEO_MODE_START_END:
		if strings.TrimSpace(r.FirstFrameURL) == "" {
			return fmt.Errorf("video first frame URL is required for %s mode", r.Mode)
		}
		if strings.TrimSpace(r.LastFrameURL) == "" {
			return fmt.Errorf("video last frame URL is required for %s mode", r.Mode)
		}
	case VIDEO_MODE_SUBJECT:
		if !hasNonEmptyString(r.SubjectImageURLs) {
			return fmt.Errorf("video subject image URL is required for %s mode", r.Mode)
		}
	}
	return nil
}

func hasNonEmptyString(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// VideoUsage is the billable work completed by one video request.
type VideoUsage struct {
	GeneratedVideos      int   `json:"generated_videos,omitempty"`
	DurationMilliseconds int64 `json:"duration_milliseconds,omitempty"`
}

// VideoResult is the completed result of one video-generation request.
type VideoResult struct {
	Path  string     `json:"path"`
	Usage VideoUsage `json:"usage,omitempty"`
	Cost  core.Cost  `json:"cost"`
}

type decoratedVideo struct {
	generator VideoGenerator
	name      string
	decorate  Decorator
}

func (d *decoratedVideo) MaxPromptLength() int {
	return d.generator.MaxPromptLength()
}

func (d *decoratedVideo) GenerateVideo(
	ctx context.Context,
	req VideoRequest,
) (VideoResult, error) {
	auth, err := resolveRequestAuth(ctx, d.name, d.decorate, req.Auth)
	if err != nil {
		return VideoResult{}, err
	}
	req.Auth = auth
	return d.generator.GenerateVideo(ctx, req)
}

// WithVideoDecorator returns a video generator that resolves credentials
// before every outbound request. A nil decorator returns generator unchanged.
func WithVideoDecorator(
	name string,
	generator VideoGenerator,
	decorator Decorator,
) VideoGenerator {
	if decorator == nil || generator == nil {
		return generator
	}
	return &decoratedVideo{
		generator: generator,
		name:      name,
		decorate:  decorator,
	}
}
