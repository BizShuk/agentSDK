package minimax

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const (
	videoPromptLimit            = 3000
	defaultVideoPollInterval    = 10 * time.Second
	defaultVideoRequestTimeout  = 60 * time.Second
	defaultVideoDownloadTimeout = 5 * time.Minute
)

const (
	videoStatusQueueing   = "Queueing"
	videoStatusPreparing  = "Preparing"
	videoStatusProcessing = "Processing"
	videoStatusSuccess    = "Success"
	videoStatusFail       = "Fail"
)

var (
	// ErrVideoTaskFailed identifies a terminal failure reported by MiniMax.
	ErrVideoTaskFailed = errors.New("minimax video task failed")
	// ErrVideoPollingCancelled identifies cancellation while awaiting a task.
	ErrVideoPollingCancelled = errors.New("minimax video polling cancelled")
)

// VideoProvider implements provider.VideoGenerator against the asynchronous
// MiniMax video API.
type VideoProvider struct {
	baseURL         string
	auth            core.Auth
	client          *http.Client
	downloadTimeout time.Duration
}

// NewVideo returns a MiniMax video generator from registry-resolved config.
func NewVideo(cfg provider.ResolvedConfig) (*VideoProvider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultVideoBaseURL
	}
	requestTimeout := cfg.Timeout
	if requestTimeout <= 0 {
		requestTimeout = defaultVideoRequestTimeout
	}
	downloadTimeout := cfg.Timeout
	if downloadTimeout <= 0 {
		downloadTimeout = defaultVideoDownloadTimeout
	}
	return &VideoProvider{
		baseURL:         strings.TrimRight(cfg.BaseURL, "/"),
		auth:            cfg.Auth,
		client:          &http.Client{Timeout: requestTimeout},
		downloadTimeout: downloadTimeout,
	}, nil
}

// MaxPromptLength returns MiniMax's documented prompt ceiling in runes.
func (*VideoProvider) MaxPromptLength() int {
	return videoPromptLimit
}

// GenerateVideo creates, polls, downloads, and verifies one MiniMax video.
func (p *VideoProvider) GenerateVideo(
	ctx context.Context,
	request provider.VideoRequest,
) (provider.VideoResult, error) {
	if err := request.Validate(); err != nil {
		return provider.VideoResult{}, err
	}
	if count := utf8.RuneCountInString(request.Prompt); count > p.MaxPromptLength() {
		return provider.VideoResult{}, fmt.Errorf(
			"minimax video prompt has %d runes, exceeds limit of %d",
			count,
			p.MaxPromptLength(),
		)
	}
	if err := validateVideoFormat(request.OutputFormat); err != nil {
		return provider.VideoResult{}, err
	}

	auth := p.auth.Merge(request.Auth)
	if auth.Token() == "" {
		return provider.VideoResult{}, fmt.Errorf("minimax video credential is required")
	}
	model := request.Model
	if model == "" {
		model = defaultVideoModel(request.Mode)
	}
	payload, err := buildVideoPayload(request, model)
	if err != nil {
		return provider.VideoResult{}, err
	}

	taskID, err := p.createVideoTask(ctx, auth, payload)
	if err != nil {
		return provider.VideoResult{}, err
	}
	fileID, err := p.pollVideoTask(ctx, auth, taskID, request.PollInterval)
	if err != nil {
		return provider.VideoResult{}, err
	}
	downloadURL, err := p.retrieveVideoFile(ctx, auth, fileID)
	if err != nil {
		return provider.VideoResult{}, err
	}
	if err := p.downloadVideo(ctx, auth, downloadURL, request.OutputPath); err != nil {
		return provider.VideoResult{}, err
	}
	if err := verifyVideoFile(request.OutputPath, request.OutputFormat); err != nil {
		return provider.VideoResult{}, err
	}
	return provider.VideoResult{Path: request.OutputPath}, nil
}

func defaultVideoModel(mode provider.VideoMode) string {
	switch mode {
	case provider.VIDEO_MODE_TEXT, provider.VIDEO_MODE_IMAGE:
		return "MiniMax-Hailuo-2.3"
	case provider.VIDEO_MODE_START_END:
		return "MiniMax-Hailuo-02"
	case provider.VIDEO_MODE_SUBJECT:
		return "S2V-01"
	default:
		return ""
	}
}
