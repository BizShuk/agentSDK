package grok

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/protocol/openaiimage"
)

// NewImage returns xAI's OpenAI-compatible image generator.
func NewImage(cfg provider.ResolvedConfig) (*Provider, error) {
	imageModel := strings.TrimSpace(cfg.Model)
	cfg.Model = ""
	client, err := New(cfg)
	if err != nil {
		return nil, err
	}
	if imageModel != "" {
		client.imageModel = imageModel
	}
	return client, nil
}

// GenerateImage sends an OpenAI-compatible image generation request.
func (p *Provider) GenerateImage(
	ctx context.Context,
	req provider.ImageRequest,
) (provider.ImageResult, error) {
	raw, err := openaiimage.EncodeRequest(req, p.imageModel)
	if err != nil {
		return provider.ImageResult{}, fmt.Errorf("grok: image: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+openaiimage.GENERATIONS_PATH,
		bytes.NewReader(raw),
	)
	if err != nil {
		return provider.ImageResult{}, fmt.Errorf("grok: image request: %w", err)
	}
	p.applyHeaders(httpReq, req.Auth, false)

	resp, err := p.imageClient.Do(httpReq)
	if err != nil {
		return provider.ImageResult{}, fmt.Errorf("grok: image http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		respBody, err := openaiimage.ReadErrorResponse(resp.Body)
		if err != nil {
			return provider.ImageResult{}, fmt.Errorf("grok: image: %w", err)
		}
		return provider.ImageResult{}, openaiimage.DecodeAPIError(
			"grok",
			"generate image",
			resp.StatusCode,
			openaiimage.RequestID(resp.Header),
			respBody,
		)
	}
	respBody, err := openaiimage.ReadResponse(resp.Body)
	if err != nil {
		return provider.ImageResult{}, fmt.Errorf("grok: image: %w", err)
	}
	result, err := openaiimage.DecodeResponse(respBody)
	if err != nil {
		return provider.ImageResult{}, fmt.Errorf("grok: image: %w", err)
	}
	return result, nil
}
