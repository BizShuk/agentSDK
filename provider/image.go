package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// ImageGenerator is the optional provider capability for creating images.
// It is deliberately separate from core.Provider because the agent runtime
// does not consume image-generation requests.
type ImageGenerator interface {
	GenerateImage(ctx context.Context, request ImageRequest) (ImageResult, error)
}

// ImageFactory builds an image generator from registry-resolved options.
type ImageFactory func(ResolvedConfig) (ImageGenerator, error)

// ImageRequest is the provider-neutral input to an image-generation API.
// Empty optional fields let the concrete provider apply its own defaults.
type ImageRequest struct {
	Model             string `json:"model,omitempty"`
	Prompt            string `json:"prompt"`
	Count             int    `json:"count,omitempty"`
	Size              string `json:"size,omitempty"`
	Quality           string `json:"quality,omitempty"`
	ResponseFormat    string `json:"response_format,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	Background        string `json:"background,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
	User              string `json:"user,omitempty"`
	// Extra carries non-streaming vendor extensions. Standard fields and
	// streaming controls are rejected by the shared codec.
	Extra map[string]json.RawMessage `json:"-"`

	// Auth overrides construction-time or decorated credentials for this call.
	Auth core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent image request invariants.
func (r ImageRequest) Validate() error {
	if strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("image prompt is required")
	}
	if r.Count < 0 {
		return fmt.Errorf("image count must not be negative")
	}
	if r.OutputCompression != nil && (*r.OutputCompression < 0 || *r.OutputCompression > 100) {
		return fmt.Errorf("image output compression must be between 0 and 100")
	}
	return nil
}

// Image is one generated image. Providers may return a URL, base64 payload, or
// both. URL lifetimes remain provider-defined, so callers that need durable
// storage must copy the asset.
type Image struct {
	URL           string `json:"url,omitempty"`
	Base64        string `json:"base64,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImageInputTokenDetails splits image-generation input usage by modality.
type ImageInputTokenDetails struct {
	TextTokens  int `json:"text_tokens,omitempty"`
	ImageTokens int `json:"image_tokens,omitempty"`
}

// ImageUsage is the common usage subset returned by OpenAI-compatible image
// APIs. CostInUSDTicks preserves xAI's exact integer accounting when present.
type ImageUsage struct {
	TotalTokens       int                    `json:"total_tokens,omitempty"`
	InputTokens       int                    `json:"input_tokens,omitempty"`
	OutputTokens      int                    `json:"output_tokens,omitempty"`
	InputTokenDetails ImageInputTokenDetails `json:"input_tokens_details,omitempty"`
	CostInUSDTicks    int64                  `json:"cost_in_usd_ticks,omitempty"`
}

// ImageResult is the folded response from one image-generation request.
type ImageResult struct {
	Created int64      `json:"created,omitempty"`
	Images  []Image    `json:"images"`
	Usage   ImageUsage `json:"usage,omitempty"`
}

type decoratedImage struct {
	generator ImageGenerator
	name      string
	decorate  Decorator
}

func (d *decoratedImage) GenerateImage(
	ctx context.Context,
	req ImageRequest,
) (ImageResult, error) {
	auth, err := resolveRequestAuth(ctx, d.name, d.decorate, req.Auth)
	if err != nil {
		return ImageResult{}, err
	}
	req.Auth = auth
	return d.generator.GenerateImage(ctx, req)
}

// WithImageDecorator returns an image generator that resolves credentials
// before every outbound request. A nil decorator returns generator unchanged.
func WithImageDecorator(
	name string,
	generator ImageGenerator,
	decorator Decorator,
) ImageGenerator {
	if decorator == nil || generator == nil {
		return generator
	}
	return &decoratedImage{
		generator: generator,
		name:      name,
		decorate:  decorator,
	}
}
