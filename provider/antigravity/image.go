package antigravity

// Image generation on this gateway is not a separate API. Unlike Google's
// public /images/generations or MiniMax's /v1/image_generation, Cloud Code
// has no image endpoint at all: an image model is asked in ordinary chat
// and answers with an inlineData part on the same GenerateContent call.
//
// This file adapts that chat surface to provider.ImageGenerator so callers
// reach it through the same Entry.NewImage path as every other vendor,
// rather than having to know that antigravity is special.
//
// The seam is narrow on purpose. ImageRequest carries fields an
// OpenAI-shaped image API accepts — size, quality, background, moderation
// — and the chat surface can express none of them. They are rejected
// rather than dropped: a caller who asked for 1024x1024 and silently got
// whatever the model chose has no way to tell that the field did nothing.

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// DefaultImageModel is the image-capable model on this gateway. The chat
// default (gemini-3.6-flash-high) cannot generate images — asked for one
// it replies with prose describing the picture it did not draw.
const DefaultImageModel = "gemini-3.1-flash-image"

// MAX_IMAGE_COUNT bounds a single GenerateImage call. The gateway returns
// one image per turn, so Count is served by repeating the request; the
// bound keeps a stray Count from spending an account's image quota, which
// this endpoint exhausts for days rather than minutes.
const MAX_IMAGE_COUNT = 4

// ImageProvider implements provider.ImageGenerator over the chat surface.
type ImageProvider struct {
	chat  *Provider
	model string
}

// NewImageGenerator builds the image capability from registry-resolved
// construction config.
func NewImageGenerator(cfg provider.ResolvedConfig) (*ImageProvider, error) {
	model := cfg.Model
	if model == "" {
		model = DefaultImageModel
	}
	// The chat client keeps its own default: this Provider is the
	// transport (hosts, session, project cache), and the image model is
	// passed per request.
	cfg.Model = ""
	chat, err := New(cfg)
	if err != nil {
		return nil, err
	}
	return &ImageProvider{chat: chat, model: model}, nil
}

// WithProjectID pins the Cloud Code project, skipping discovery.
func (p *ImageProvider) WithProjectID(id string) *ImageProvider {
	p.chat.WithProjectID(id)
	return p
}

// GenerateImage implements provider.ImageGenerator.
func (p *ImageProvider) GenerateImage(
	ctx context.Context,
	request provider.ImageRequest,
) (provider.ImageResult, error) {
	if err := request.Validate(); err != nil {
		return provider.ImageResult{}, err
	}
	if err := rejectUnsupportedImageFields(request); err != nil {
		return provider.ImageResult{}, err
	}

	model := request.Model
	if model == "" {
		model = p.model
	}
	count := request.Count
	if count <= 0 {
		count = 1
	}
	if count > MAX_IMAGE_COUNT {
		return provider.ImageResult{}, fmt.Errorf(
			"antigravity image: count %d exceeds %d; the gateway returns one image per turn, so each additional image is a separate billed request",
			count, MAX_IMAGE_COUNT)
	}

	parts, err := imagePrompt(request)
	if err != nil {
		return provider.ImageResult{}, err
	}

	var out provider.ImageResult
	for i := range count {
		res, err := p.chat.generateWith(ctx, core.ModelRequest{
			Messages: []core.Message{{Role: core.ROLE_USER, Parts: parts}},
			Auth:     request.Auth,
		}, model)
		if err != nil {
			return provider.ImageResult{}, fmt.Errorf("antigravity image %d/%d: %w", i+1, count, err)
		}
		images := collectImages(res)
		if len(images) == 0 {
			// A text-only reply means the model described the picture
			// instead of drawing it — the usual cause is a model that is
			// not image-capable. Say so rather than returning an empty
			// success.
			return provider.ImageResult{}, fmt.Errorf(
				"antigravity image: model %q returned no image (is it image-capable? default is %s)",
				model, DefaultImageModel)
		}
		out.Images = append(out.Images, images...)
		out.Usage.InputTokens += res.Usage.InputTokens
		out.Usage.OutputTokens += res.Usage.OutputTokens
		out.Usage.TotalTokens += res.Usage.TotalTokens
	}
	return out, nil
}

// imagePrompt builds the chat turn: the prompt text, plus any reference
// images as inlineData parts.
//
// Subject references ARE supported here, unlike in the shared openaiimage
// codec — image-to-image on this gateway is just an image in the same
// message, which is exactly what the chat surface already carries.
func imagePrompt(request provider.ImageRequest) ([]core.Part, error) {
	parts := make([]core.Part, 0, 1+len(request.SubjectReferences))
	for i, ref := range request.SubjectReferences {
		if strings.TrimSpace(ref.URL) != "" {
			return nil, fmt.Errorf(
				"antigravity image: subject reference %d is a URL; this gateway takes inline bytes only, so fetch it and pass Base64", i)
		}
		raw, err := base64.StdEncoding.DecodeString(ref.Base64)
		if err != nil {
			return nil, fmt.Errorf("antigravity image: subject reference %d is not valid base64: %w", i, err)
		}
		parts = append(parts, core.Part{
			Kind:      core.PART_KIND_IMAGE,
			Image:     raw,
			ImageMIME: mimeOrDefault(ref.MIMEType, "image/png"),
		})
	}
	return append(parts, core.Part{Kind: core.PART_KIND_PLAIN_TEXT, Text: request.Prompt}), nil
}

// collectImages projects the image parts of one chat reply.
func collectImages(res core.ModelResult) []provider.Image {
	var out []provider.Image
	for _, part := range res.Parts {
		if part.Kind != core.PART_KIND_IMAGE || len(part.Image) == 0 {
			continue
		}
		out = append(out, provider.Image{
			Base64:   base64.StdEncoding.EncodeToString(part.Image),
			MIMEType: part.ImageMIME,
		})
	}
	return out
}

// rejectUnsupportedImageFields fails a request carrying a knob the chat
// surface cannot honor.
func rejectUnsupportedImageFields(r provider.ImageRequest) error {
	unsupported := []struct {
		name string
		set  bool
	}{
		{"size", r.Size != ""},
		{"quality", r.Quality != ""},
		{"output_format", r.OutputFormat != ""},
		{"output_compression", r.OutputCompression != nil},
		{"background", r.Background != ""},
		{"moderation", r.Moderation != ""},
		{"user", r.User != ""},
		{"extra", len(r.Extra) > 0},
	}
	var named []string
	for _, f := range unsupported {
		if f.set {
			named = append(named, f.name)
		}
	}
	if len(named) > 0 {
		return fmt.Errorf(
			"antigravity image: %s not supported; this gateway generates images through its chat surface, which carries no such control",
			strings.Join(named, ", "))
	}
	// The gateway always returns inline bytes; it has no asset URLs to hand out.
	if f := strings.TrimSpace(r.ResponseFormat); f != "" && f != "b64_json" {
		return fmt.Errorf("antigravity image: response_format %q not supported (only b64_json — the gateway returns inline bytes)", f)
	}
	return nil
}
