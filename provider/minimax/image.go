package minimax

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const (
	defaultImageModel          = "image-01"
	defaultImageRequestTimeout = 3 * time.Minute

	imagePromptMaxRunes = 1500
	imageCountMax       = 9
	imageDimensionMin   = 512
	imageDimensionMax   = 2048
)

// ImageProvider implements provider.ImageGenerator against MiniMax's
// /v1/image_generation endpoint. Text-to-image and image-to-image share the
// endpoint: a request carrying SubjectReferences becomes the subject_reference
// (i2i) variant.
type ImageProvider struct {
	baseURL string
	model   string
	auth    core.Auth
	client  *http.Client
}

// NewImage returns a MiniMax image generator from registry-resolved config.
func NewImage(cfg provider.ResolvedConfig) (*ImageProvider, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultImageRequestTimeout
	}
	return &ImageProvider{
		baseURL: imageBaseURL(cfg.BaseURL),
		model:   strings.TrimSpace(cfg.Model),
		auth:    cfg.Auth,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

// imageBaseURL resolves the image_generation root the same way speechBaseURL
// does for t2a_v2: capability env or explicit base first, default root when
// nothing resolved, and a trailing "/anthropic" trimmed because the media
// endpoints are served one segment above the Anthropic-compat chat surface.
func imageBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return DefaultImageBaseURL
	}
	return strings.TrimSuffix(trimmed, anthropicCompatSuffix)
}

// GenerateImage creates one batch of images.
func (p *ImageProvider) GenerateImage(
	ctx context.Context,
	request provider.ImageRequest,
) (provider.ImageResult, error) {
	if err := request.Validate(); err != nil {
		return provider.ImageResult{}, err
	}
	model := p.resolveImageModel(request)
	if err := validateMiniMaxImageRequest(request); err != nil {
		return provider.ImageResult{}, err
	}

	auth := p.auth.Merge(request.Auth)
	if auth.Token() == "" {
		return provider.ImageResult{}, fmt.Errorf("minimax image credential is required")
	}
	payload, err := encodeImageRequest(request, model)
	if err != nil {
		return provider.ImageResult{}, err
	}
	response, requestID, err := p.createImage(ctx, auth, payload)
	if err != nil {
		return provider.ImageResult{}, err
	}
	if response.BaseResp.StatusCode != 0 {
		return provider.ImageResult{}, newImageAPIError(
			http.StatusOK,
			requestID,
			response.BaseResp,
		)
	}
	return foldImageResponse(response), nil
}

func (p *ImageProvider) resolveImageModel(request provider.ImageRequest) string {
	if model := strings.TrimSpace(request.Model); model != "" {
		return model
	}
	if p.model != "" {
		return p.model
	}
	return defaultImageModel
}

// validateMiniMaxImageRequest rejects both what the API bounds (prompt
// length, batch size, dimensions) and the neutral fields this endpoint has
// no wire slot for — erroring beats silently dropping a knob the caller set.
func validateMiniMaxImageRequest(request provider.ImageRequest) error {
	promptRunes := trimmedRuneCount(request.Prompt)
	if promptRunes > imagePromptMaxRunes {
		return fmt.Errorf(
			"minimax image prompt has %d runes, exceeds limit of %d",
			promptRunes,
			imagePromptMaxRunes,
		)
	}
	if request.Count > imageCountMax {
		return fmt.Errorf(
			"minimax image count %d exceeds limit of %d",
			request.Count,
			imageCountMax,
		)
	}
	switch normalizeImageResponseFormat(request.ResponseFormat) {
	case "", "url", "base64":
	default:
		return fmt.Errorf(
			"minimax image response format %q is not supported",
			request.ResponseFormat,
		)
	}
	_, width, height, err := parseImageSize(request.Size)
	if err != nil {
		return err
	}
	if width != 0 || height != 0 {
		for _, dimension := range []int{width, height} {
			if dimension < imageDimensionMin ||
				dimension > imageDimensionMax ||
				dimension%8 != 0 {
				return fmt.Errorf(
					"minimax image dimensions %q must be %d-%d and divisible by 8",
					request.Size,
					imageDimensionMin,
					imageDimensionMax,
				)
			}
		}
	}
	if request.Quality != "" {
		return fmt.Errorf("minimax image does not support quality")
	}
	if request.OutputFormat != "" {
		return fmt.Errorf("minimax image does not support output format")
	}
	if request.OutputCompression != nil {
		return fmt.Errorf("minimax image does not support output compression")
	}
	if request.Background != "" {
		return fmt.Errorf("minimax image does not support background")
	}
	if request.Moderation != "" {
		return fmt.Errorf("minimax image does not support moderation")
	}
	if request.User != "" {
		return fmt.Errorf("minimax image does not support user attribution")
	}
	return nil
}

// Compile-time: ensure ImageProvider satisfies the image capability port.
var _ provider.ImageGenerator = (*ImageProvider)(nil)
