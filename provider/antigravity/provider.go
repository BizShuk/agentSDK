// Package antigravity adapts Google's Cloud Code "v1internal" gateway —
// the endpoint behind the Antigravity IDE — to agentsdk's core.Provider
// interface.
//
// The gateway speaks Gemini GenerateContent inside a Cloud Code envelope
// and serves both Gemini and Claude models through it. Authentication is
// a Google OAuth access token; see config.go for the credential story and
// the two open proxies this wire contract was reconstructed from.
//
// File layout:
//
//	provider.go    — Provider struct, transport, interface methods
//	config.go      — endpoints, env names, client identity constants
//	dto.go         — wire-format types
//	encode.go      — core.ModelRequest → CloudCodeRequest
//	decode.go      — GenerateResponse → core.ModelResult
//	schema.go      — JSON Schema → Google's schema dialect
//	stream.go      — SSE parser → core.ModelChunk, and the fold back
//	project.go     — loadCodeAssist project discovery
//	models.go      — catalog, live listing, model-family vocabulary
//	validate.go    — CloudCodeRequest.Validate()
package antigravity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/google/uuid"
)

// DefaultModel is used when the caller names none.
const DefaultModel = "gemini-3.6-flash-high"

// DEFAULT_TIMEOUT is the per-request HTTP ceiling. Thinking models on a
// large context routinely run past a minute before the first token.
const DEFAULT_TIMEOUT = 300 * time.Second

// Provider implements core.Provider against the Cloud Code gateway.
type Provider struct {
	// hosts is the endpoint fallback chain, tried in order. A caller who
	// pinned a base URL gets a chain of one — naming a host means that
	// host, not "that one and then Google's".
	hosts []string

	// auth holds whichever credential class the constructor was given.
	// The gateway wants an OAuth bearer; an API key is accepted in the
	// same position for deployments that front it with a local proxy.
	auth  core.Auth
	model string

	// sessionID is a prompt-cache key, stable for this Provider's
	// lifetime. The IDE generates one per launch and reuses it, which is
	// what makes the gateway's cache hit across turns.
	sessionID string

	client *http.Client

	// project is the Cloud Code project id. pinned is set from config or
	// the environment; discovered caches the loadCodeAssist answer.
	pinned     string
	mu         sync.Mutex
	discovered string
}

// New returns a Provider from registry-resolved construction config.
//
// With no base URL it gets the built-in daily → production chain; a
// configured base URL replaces the chain entirely.
func New(cfg provider.ResolvedConfig) (*Provider, error) {
	hosts := []string{DefaultBaseURL, FallbackBaseURL}
	if cfg.BaseURL != "" {
		hosts = []string{strings.TrimRight(cfg.BaseURL, "/")}
	}
	return NewForHosts(hosts, cfg), nil
}

// NewForHosts builds a Provider over an explicit endpoint chain, tried in
// order. Both reference proxies make this chain configurable because
// which Cloud Code channel serves an account varies — daily, production,
// and two sandbox hosts are all live, and entitlement differs between
// them.
func NewForHosts(hosts []string, cfg provider.ResolvedConfig) *Provider {
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DEFAULT_TIMEOUT
	}
	trimmed := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h = strings.TrimRight(strings.TrimSpace(h), "/"); h != "" {
			trimmed = append(trimmed, h)
		}
	}
	return &Provider{
		hosts:     trimmed,
		auth:      cfg.Auth,
		model:     cfg.Model,
		sessionID: uuid.NewString() + fmt.Sprint(time.Now().UnixMilli()),
		client:    &http.Client{Timeout: timeout},
		// The registry resolves credentials and base URLs through an
		// injected LookupEnv, but has no slot for a project id — it is
		// Cloud Code vocabulary, not a provider-registry concept. Reading
		// it here keeps the escape hatch available; WithProjectID
		// overrides it for callers that would rather not use the
		// environment at all.
		pinned: strings.TrimSpace(os.Getenv(ProjectIDEnvVar)),
	}
}

// WithProjectID pins the Cloud Code project, skipping discovery. The
// registry has no field for it, so the CLI and env path set it here.
func (p *Provider) WithProjectID(id string) *Provider {
	p.pinned = strings.TrimSpace(id)
	return p
}

// Generate implements core.Provider.
//
// Thinking models are served over the streaming endpoint and folded back
// into one result: the blocking endpoint silently omits thought parts, so
// asking it for a reasoning model would return an answer with its
// reasoning — and the signature the next turn needs — missing.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	return p.generateWith(ctx, req, p.model)
}

// generateWith runs one turn against an explicit model.
//
// The model is a parameter rather than only a field because the image
// capability drives this same chat surface with its own model id, and it
// must reuse this client's session id, project cache and host chain — a
// second Provider would re-run loadCodeAssist and break prompt caching.
func (p *Provider) generateWith(ctx context.Context, req core.ModelRequest, model string) (core.ModelResult, error) {
	body, err := p.buildBody(ctx, req, model)
	if err != nil {
		return core.ModelResult{}, err
	}

	if isThinkingModel(model) {
		resp, err := p.dispatch(ctx, PATH_STREAM, body, req.Auth, "text/event-stream", model)
		if err != nil {
			return core.ModelResult{}, err
		}
		defer resp.Body.Close()
		chunks, stop := ParseStream(ctx, resp.Body)
		return FoldStream(chunks, stop), nil
	}

	raw, err := p.send(ctx, PATH_GENERATE, body, req.Auth, "", model)
	if err != nil {
		return core.ModelResult{}, err
	}
	decoded, err := Unwrap(raw)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("antigravity: decode: %w", err)
	}
	return FromResponse(decoded), nil
}

// Stream implements core.StreamProvider.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	body, err := p.buildBody(ctx, req, p.model)
	if err != nil {
		return nil, err
	}
	resp, err := p.dispatch(ctx, PATH_STREAM, body, req.Auth, "text/event-stream", p.model)
	if err != nil {
		return nil, err
	}
	chunks, _ := ParseStream(ctx, resp.Body)

	// ParseStream owns the reader from here; close it once the parser
	// has drained, so a caller that ranges the channel leaks nothing.
	out := make(chan core.ModelChunk, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		for chunk := range chunks {
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// buildBody resolves the project, assembles the envelope, validates it,
// and marshals it once so every fallback attempt reuses the same bytes.
func (p *Provider) buildBody(ctx context.Context, req core.ModelRequest, model string) ([]byte, error) {
	// Discovery runs under the caller's context and is cached after the
	// first hit; a pinned id skips it entirely.
	project, err := p.ProjectID(ctx, req.Auth)
	if err != nil {
		return nil, err
	}
	requestID := req.RequestID
	if requestID == "" {
		requestID = uuid.NewString()
	}
	envelope, err := buildRequest(req, model, project, p.sessionID, "agent-"+requestID)
	if err != nil {
		return nil, err
	}
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("antigravity: marshal: %w", err)
	}
	return raw, nil
}

// ---------------------------------------------------------------------------
// transport
// ---------------------------------------------------------------------------

// post marshals body and returns the decoded response bytes. Discovery
// calls carry no model, so they pass the client's own.
func (p *Provider) post(ctx context.Context, path string, body any, override core.Auth, accept string) ([]byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("antigravity: marshal: %w", err)
	}
	return p.send(ctx, path, raw, override, accept, p.model)
}

// send performs the request and buffers the response body.
func (p *Provider) send(ctx context.Context, path string, raw []byte, override core.Auth, accept, model string) ([]byte, error) {
	resp, err := p.dispatch(ctx, path, raw, override, accept, model)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, MAX_BODY_BYTES))
}

// dispatch walks the host chain and returns the first 2xx response with
// its body still open.
//
// Only 403/404/5xx and transport failures move on to the next host: those
// are the shapes "this channel does not serve you" takes. A 400 or 429 is
// about the request or the account, and retrying it elsewhere would just
// spend the quota twice.
func (p *Provider) dispatch(ctx context.Context, path string, raw []byte, override core.Auth, accept, model string) (*http.Response, error) {
	var lastErr error
	for _, host := range p.hosts {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		if err := p.applyHeaders(req, override, accept, model); err != nil {
			return nil, err
		}
		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("antigravity: http %s: %w", host, err)
			continue
		}
		if resp.StatusCode/100 == 2 {
			return resp, nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, MAX_ERROR_BYTES))
		resp.Body.Close()
		err = statusError(host, resp.StatusCode, body)
		if !retryableStatus(resp.StatusCode) {
			return nil, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("antigravity: no endpoint configured")
	}
	return nil, lastErr
}

// retryableStatus reports whether another host is worth trying.
func retryableStatus(code int) bool {
	return code == http.StatusForbidden || code == http.StatusNotFound || code >= 500
}

// statusError renders a non-2xx response, preferring the structured
// google.rpc.Status message when the body carries one.
func statusError(host string, code int, body []byte) error {
	var env Envelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error != nil && env.Error.Message != "" {
		return fmt.Errorf("antigravity: %s status %d: %s", host, code, env.Error.Message)
	}
	return fmt.Errorf("antigravity: %s status %d: %s", host, code, strings.TrimSpace(string(body)))
}

// applyHeaders sets the client identity the gateway checks. A request
// missing them is answered 403 even with a valid token.
//
// override is the per-call core.ModelRequest.Auth, merged on top of the
// credential bound at construction; a zero override changes nothing.
func (p *Provider) applyHeaders(req *http.Request, override core.Auth, accept, model string) error {
	a := p.auth.Merge(override)
	token := a.Token()
	if token == "" {
		return fmt.Errorf("antigravity: requires a Google OAuth access token (set %s or supply a credential decorator)", OAuthEnvVar)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent())
	req.Header.Set("X-Client-Name", CLIENT_NAME)
	req.Header.Set("X-Client-Version", ClientVersion)
	req.Header.Set("x-goog-api-client", GOOG_API_CLIENT)
	req.Header.Set("X-Machine-Session-Id", p.sessionID)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if isClaudeModel(model) && isThinkingModel(model) {
		req.Header.Set("anthropic-beta", INTERLEAVED_THINKING_BETA)
	}
	for k, v := range a.Headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	return nil
}
