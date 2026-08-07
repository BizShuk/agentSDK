// Package ollama adapts local Ollama and other OpenAI-compatible
// /v1/chat/completions endpoints to agentsdk's provider contracts.
package ollama

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
	"github.com/bizshuk/agentsdk/provider/protocol/openaichat"
)

const defaultModel = "qwen2.5vl:3b"

// defaultTimeout bounds a single blocking request when the caller does not
// set ResolvedConfig.Timeout. Local vision models reading a phone photo
// routinely need far more — those callers must raise it explicitly.
const defaultTimeout = 300 * time.Second

// Provider implements the generate and stream capabilities against an
// OpenAI-compatible endpoint.
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
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Provider{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		auth:    cfg.Auth,
		model:   cfg.Model,
		client:  &http.Client{Timeout: cfg.Timeout},
	}, nil
}

// Generate sends a blocking Chat Completions request.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	raw, err := openaichat.EncodeRequest(req, p.model, false)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("ollama: %w", err)
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
		return core.ModelResult{}, fmt.Errorf("ollama: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("ollama: read: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return core.ModelResult{}, fmt.Errorf(
			"ollama: status %d: %s",
			resp.StatusCode,
			string(respBody),
		)
	}
	result, err := openaichat.DecodeResponse(respBody)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("ollama: decode: %w", err)
	}
	result.Cost = core.FreeCost()
	return result, nil
}

// Stream sends a streaming Chat Completions request.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	raw, err := openaichat.EncodeRequest(req, p.model, true)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
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
		return nil, fmt.Errorf("ollama: http: %w", err)
	}
	chunks, err := openaichat.ParseStream(ctx, resp.Body)
	if err != nil {
		return nil, err
	}
	return freeTerminalCost(ctx, chunks), nil
}

func freeTerminalCost(
	ctx context.Context,
	input <-chan core.ModelChunk,
) <-chan core.ModelChunk {
	output := make(chan core.ModelChunk, 1)
	go func() {
		defer close(output)
		for chunk := range input {
			if chunk.Done {
				chunk.Cost = core.FreeCost()
			}
			select {
			case <-ctx.Done():
				return
			case output <- chunk:
			}
		}
	}()
	return output
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
