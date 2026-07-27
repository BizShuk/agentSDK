// Package anthropic adapts the official anthropic-sdk-go to agentsdk's
// core.Provider interface. The adapter is intentionally thin — it owns
// the auth token + model selection and translates core.Message ⇄
// anthropic.MessageParam in both directions.
//
// File layout:
//
//   - provider.go    — entry point, Provider struct, interface methods
//   - dto.go         — wire-format types (RequestBody, ContentBlock, ...)
//   - validate.go    — RequestBody.Validate()
//   - translate.go   — core.Message ⇄ Anthropic wire conversion
//   - stream.go      — SSE parser → core.ModelChunk
//   - models.go      — DefaultCatalog
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const defaultModel = "claude-3-5-sonnet-latest"

// Provider implements core.Provider against the Anthropic API.
type Provider struct {
	client   *anthropic.Client
	model    anthropic.Model
	auth     core.Auth // resolved at construction; honors req.Auth overrides per call
	httpDoer *http.Client
	endpoint string
	apiVer   string
}

// New returns a Provider from registry-resolved construction config.
func New(cfg provider.ResolvedConfig) (*Provider, error) {
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	clientOpts := authRequestOptions(cfg.Auth)
	if cfg.BaseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(cfg.BaseURL))
	}
	client := anthropic.NewClient(clientOpts...)
	return &Provider{
		client:   &client,
		model:    anthropic.Model(cfg.Model),
		auth:     cfg.Auth,
		httpDoer: http.DefaultClient,
		endpoint: resolveEndpoint(cfg.BaseURL),
		apiVer:   "2023-06-01",
	}, nil
}

// Generate implements core.Provider.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	body := p.buildRequestBody(req)
	if err := body.Validate(); err != nil {
		return core.ModelResult{}, err
	}
	params, err := toSDKParams(body)
	if err != nil {
		return core.ModelResult{}, err
	}
	opts := p.authOptions(req)
	resp, err := p.client.Messages.New(ctx, params, opts...)
	if err != nil {
		return core.ModelResult{}, err
	}
	return fromSDKResponse(resp), nil
}

// Stream implements core.StreamProvider. We go through a direct HTTP request
// (rather than the SDK's NewStreaming) so the SSE parser in stream.go
// owns the wire format and stays independently testable.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	body := p.buildRequestBody(req)
	if err := body.Validate(); err != nil {
		return nil, err
	}
	httpReq, err := p.buildHTTPRequest(ctx, body, true, req.Auth)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpDoer.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("anthropic: stream http %d: %s", resp.StatusCode, buf.String())
	}
	ch, _ := ParseStream(ctx, resp.Body)
	return ch, nil
}

// buildRequestBody assembles a wire-format body from a core request and
// the provider's configured model. The model field is filled here so
// callers don't have to thread it through every call.
func (p *Provider) buildRequestBody(req core.ModelRequest) RequestBody {
	out := RequestBody{
		Model:     string(p.model),
		MaxTokens: maxTokensOrDefault(req),
		Messages:  toMessageParams(req.Messages),
	}
	if len(req.Tools) > 0 {
		out.Tools = toToolParams(req.Tools)
	}
	return out
}

// buildHTTPRequest marshals the wire body and stamps auth headers on the
// outbound request. Used by Stream; Generate goes through the SDK.
//
// override is the per-call auth (req.Auth). When empty we use the auth
// bound at construction time.
func (p *Provider) buildHTTPRequest(ctx context.Context, body RequestBody, stream bool, override core.Auth) (*http.Request, error) {
	body.Stream = stream
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", p.apiVer)
	p.applyAuthHeaders(req, override)
	return req, nil
}

// authOptions translates the construction credential plus per-call override
// into SDK request options.
func (p *Provider) authOptions(req core.ModelRequest) []option.RequestOption {
	return authRequestOptions(p.auth.Merge(req.Auth))
}

func authRequestOptions(a core.Auth) []option.RequestOption {
	var opts []option.RequestOption
	if a.Bearer != "" {
		opts = append(opts, option.WithAuthToken(a.Bearer))
		opts = append(opts, option.WithHeader(OAuthBetaHeader, OAuthBetaValue))
	} else {
		opts = append(opts, option.WithAPIKey(a.APIKey))
	}
	for key, value := range a.Headers {
		if value != "" {
			opts = append(opts, option.WithHeader(key, value))
		}
	}
	return opts
}

// applyAuthHeaders stamps x-api-key OR Bearer on the outbound request
// plus any provider-specific overrides (e.g. anthropic-beta for OAuth).
// override takes precedence over p.auth when set.
func (p *Provider) applyAuthHeaders(req *http.Request, override core.Auth) {
	useOverride := override.APIKey != "" || override.Bearer != "" || len(override.Headers) > 0
	src := p.auth
	if useOverride {
		src = p.auth.Merge(override)
	}
	if src.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+src.Bearer)
		req.Header.Set(OAuthBetaHeader, OAuthBetaValue)
	} else if src.APIKey != "" {
		req.Header.Set("x-api-key", src.APIKey)
	}
	for k, v := range src.Headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
}

// resolveEndpoint computes the /v1/messages URL for the configured
// base. Empty base falls back to the public Anthropic endpoint.
func resolveEndpoint(base string) string {
	if base == "" {
		return "https://api.anthropic.com/v1/messages"
	}
	return base + "/v1/messages"
}
