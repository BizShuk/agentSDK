// Package google adapts Google Generative AI's OpenAI-compatible
// /chat/completions endpoint to agentsdk's provider contracts.
package google

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/internal/openaichat"
)

const defaultModel = "gemini-3-flash-preview"

// Provider implements the generate and stream capabilities against Google
// Generative AI's OpenAI-compatible endpoint.
type Provider struct {
	baseURL string
	auth    core.Auth
	model   string
	client  *http.Client
}

// New returns a Provider from registry-resolved construction config.
func New(cfg provider.ResolvedConfig) (*Provider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	return &Provider{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		auth:    cfg.Auth,
		model:   cfg.Model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// Generate sends a blocking Chat Completions request.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	raw, err := openaichat.EncodeRequest(req, p.model, false)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("google: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/chat/completions",
		bytes.NewReader(raw),
	)
	if err != nil {
		return core.ModelResult{}, err
	}
	p.applyHeaders(httpReq, req.Auth, false)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("google: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("google: read: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return core.ModelResult{}, fmt.Errorf(
			"google: status %d: %s",
			resp.StatusCode,
			string(respBody),
		)
	}
	result, err := openaichat.DecodeResponse(respBody)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("google: decode: %w", err)
	}
	return result, nil
}

// Stream sends a streaming Chat Completions request.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	raw, err := openaichat.EncodeRequest(req, p.model, true)
	if err != nil {
		return nil, fmt.Errorf("google: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/chat/completions",
		bytes.NewReader(raw),
	)
	if err != nil {
		return nil, err
	}
	p.applyHeaders(httpReq, req.Auth, true)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("google: http: %w", err)
	}
	return openaichat.ParseStream(ctx, resp.Body)
}

func (p *Provider) applyHeaders(req *http.Request, override core.Auth, stream bool) {
	auth := p.auth.Merge(override)
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if token := auth.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range auth.Headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
}
