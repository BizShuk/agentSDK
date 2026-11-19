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

// Wire contracts:
// https://platform.minimax.io/docs/api-reference/image-generation-h3-generations
// https://platform.minimax.io/docs/api-reference/image-generation-h3-edits
const (
	imageGenerationsPath = "/v1/images/generations"
	imageEditsPath       = "/v1/images/edits"

	DefaultImageModel = "MiniMax-H3-Image"

	// The server may hold a request for up to 600 seconds.
	defaultImageRequestTimeout = 11 * time.Minute

	imagePromptMaxRunes   = 32000
	imageCountMax         = 10
	imageReferenceMax     = 16
	maxImageResponseBytes = 256 << 20
)

// ImageProvider implements provider.ImageGenerator against MiniMax-H3-Image.
// A request without SubjectReferences is text-to-image; with references it
// becomes an image edit, where the references are the images to edit or
// compose from.
//
// Neither endpoint is idempotent — a retry generates and bills again — so the
// adapter never retries.
type ImageProvider struct {
	rest  restClient
	model string
	auth  core.Auth
}

// NewImage returns a MiniMax image generator from registry-resolved config.
func NewImage(cfg provider.ResolvedConfig) (*ImageProvider, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultImageRequestTimeout
	}
	return &ImageProvider{
		rest:  restClient{baseURL: accountBaseURL(cfg.BaseURL), client: &http.Client{Timeout: timeout}},
		model: strings.TrimSpace(cfg.Model),
		auth:  cfg.Auth,
	}, nil
}

// GenerateImage creates one batch of images. HTTP 200 means at least one
// image succeeded; per-image failures are dropped from Images, so a result
// may hold fewer images than Count requested.
func (p *ImageProvider) GenerateImage(
	ctx context.Context,
	request provider.ImageRequest,
) (provider.ImageResult, error) {
	if err := request.Validate(); err != nil {
		return provider.ImageResult{}, err
	}
	if err := validateMiniMaxImageRequest(request); err != nil {
		return provider.ImageResult{}, err
	}
	auth := p.auth.Merge(request.Auth)
	if auth.Token() == "" {
		return provider.ImageResult{}, fmt.Errorf("minimax image credential is required")
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = DefaultImageModel
	}

	path := imageGenerationsPath
	if len(request.SubjectReferences) > 0 {
		path = imageEditsPath
	}
	payload, err := encodeImageRequest(request, model)
	if err != nil {
		return provider.ImageResult{}, err
	}
	raw, err := p.rest.post(ctx, auth, "generate image", path, "application/json", payload, maxImageResponseBytes)
	if err != nil {
		return provider.ImageResult{}, err
	}
	return decodeImageResponse(raw)
}

// validateMiniMaxImageRequest rejects what the API bounds and the neutral
// fields H3 has no wire slot for — erroring beats silently dropping a knob
// the caller set.
func validateMiniMaxImageRequest(request provider.ImageRequest) error {
	if n := trimmedRuneCount(request.Prompt); n > imagePromptMaxRunes {
		return fmt.Errorf("minimax image prompt has %d runes, exceeds limit of %d", n, imagePromptMaxRunes)
	}
	if request.Count > imageCountMax {
		return fmt.Errorf("minimax image count %d exceeds limit of %d", request.Count, imageCountMax)
	}
	if len(request.SubjectReferences) > imageReferenceMax {
		return fmt.Errorf("minimax image accepts at most %d references, got %d", imageReferenceMax, len(request.SubjectReferences))
	}
	if _, err := imageResponseFormat(request.ResponseFormat); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"quality":    request.Quality,
		"background": request.Background,
		"moderation": request.Moderation,
		"user":       request.User,
	} {
		if strings.TrimSpace(value) != "" {
			return fmt.Errorf("minimax image does not support %s", name)
		}
	}
	return nil
}
