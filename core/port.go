package core

import (
	"context"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// Provider — outer LLM-facing port.
//
// The interface intentionally exposes only the surface runtime and proxy need:
// model identity, available model catalog, supported auth flavors, and the
// three call shapes (blocking Generate, streaming Stream, cheap CountTokens).
//
// Every provider package under provider/<name>/ implements this interface
// directly. Per-provider DTO conversion (Messages ⇄ native wire format),
// auth assembly (API key / OAuth header set), and stream parsing all live
// INSIDE that provider package — runtime and proxy never see them.
// ---------------------------------------------------------------------------

// Provider is the outer interface every agentsdk provider package implements.
//
// Implementations must:
//   - Be safe to call concurrently.
//   - Honor ctx cancellation for every blocking call.
//   - Return provider-specific errors (HTTP / SSE / SDK errors) verbatim;
//     the runtime / proxy layers wrap them at their boundary.
type Provider interface {
	// ID reports the provider family (e.g. "anthropic", "ollama"). It must
	// NOT carry a model id — Models() enumerates those.
	ID() string

	// Models enumerates the provider's bundled catalog without any I/O.
	// It is the offline answer — a compiled-in snapshot that is correct
	// on the day it ships and drifts afterwards. Prefer ModelLister when
	// the provider implements it; see ListModels below.
	Models() []ModelSpec

	// AuthSchemes advertises which auth flavors the provider accepts.
	// Use this so the proxy / CLI can pick the right credential kind.
	//
	//   "api_key"   — long-lived key from environment or store
	//   "oauth"     — OAuth access token (carried as Bearer)
	//   "keyless"   — no auth needed (local Ollama, public endpoints)
	AuthSchemes() []string

	// Generate sends a blocking request. Returns the full ModelResult.
	Generate(ctx context.Context, req ModelRequest) (ModelResult, error)

	// Stream returns a channel of model chunks. The final chunk carries
	// Done=true; the runtime closes over the channel and folds into a
	// ModelResult.
	Stream(ctx context.Context, req ModelRequest) (<-chan ModelChunk, error)

	// CountTokens returns an approximate token count for a transcript.
	// Implementations may use a heuristic — accuracy is not guaranteed.
	CountTokens(ctx context.Context, msgs []Message) (int, error)
}

// ModelLister is the optional live-catalog half of Provider. Adapters whose
// upstream publishes a catalog endpoint (GET /models and friends) implement
// it so callers see the models that exist right now rather than the set that
// existed when the binary was built.
//
// It is deliberately NOT part of Provider: some upstreams (OAuth-gated
// backends, private gateways) expose no catalog endpoint at all, and forcing
// them to implement a method that can only ever fail would push that failure
// into every caller. Type-assert instead:
//
//	specs := prov.Models()
//	if l, ok := prov.(core.ModelLister); ok {
//	    if live, err := l.ListModels(ctx); err == nil {
//	        specs = live
//	    }
//	}
//
// Implementations must fall back to Models() semantics on their own only
// when that is meaningful; otherwise return the error and let the caller
// decide whether a stale catalog beats no catalog.
type ModelLister interface {
	// ListModels queries the provider's upstream catalog endpoint. The
	// returned specs carry live model ids; metadata the endpoint does not
	// report (reasoning flag, context window) is filled in from the
	// bundled catalog where the id is recognized.
	ListModels(ctx context.Context) ([]ModelSpec, error)
}

// ModelSpec is one entry in a provider's catalog. It mirrors pi/ai's Model
// type (id / reasoning / input modalities / contextWindow / maxTokens) so
// picker UIs and budget middleware can plan across providers.
type ModelSpec struct {
	ID           string    `json:"id"`
	Family       string    `json:"family,omitempty"` // provider family hint (e.g. "claude-opus")
	Reasoning    bool      `json:"reasoning,omitempty"`
	Input        []Modality `json:"input,omitempty"`
	ContextWindow int      `json:"context_window,omitempty"`
	MaxTokens    int       `json:"max_tokens,omitempty"`
}

// Modality enumerates input types a model accepts. Most providers take text;
// some take images inline; audio is rare and Anthropic-specific.
type Modality string

const (
	MODALITY_TEXT  Modality = "text"
	MODALITY_IMAGE Modality = "image"
	MODALITY_AUDIO Modality = "audio"
)

// Auth carries the resolved credentials for a single request. Callers
// (runtime, proxy dispatcher) populate this before calling Generate/Stream
// using a credential store or env-var lookup; the provider itself does not
// reach out to fetch credentials.
//
// At most one of APIKey / Bearer should be set. Headers / BaseURL are
// optional overrides.
//
// Mirrors pi/ai's ModelAuth type.
type Auth struct {
	// APIKey is sent as `x-api-key: <value>` (Anthropic-style) or
	// `Authorization: Bearer <value>` (OpenAI-style) depending on the
	// provider. Empty when using OAuth.
	APIKey string `json:"api_key,omitempty"`

	// Bearer is the OAuth access token. Empty when using an API key.
	Bearer string `json:"bearer,omitempty"`

	// Headers carries provider-specific overrides (e.g. anthropic-beta,
	// ChatGPT-Account-ID). Merged on top of provider defaults; nil values
	// suppress a default header of the same name.
	Headers map[string]string `json:"headers,omitempty"`

	// BaseURL overrides the provider's default base URL. Empty means
	// use the provider default (Ollama → localhost:11434, codex →
	// chatgpt.com/backend-api, etc.).
	BaseURL string `json:"base_url,omitempty"`
}

// IsZero reports whether the Auth carries nothing — no credential, no
// header override, no endpoint override. Adapters use it to skip the
// merge entirely when a request brings no per-call override.
func (a Auth) IsZero() bool {
	return a.APIKey == "" && a.Bearer == "" && a.BaseURL == "" && len(a.Headers) == 0
}

// Merge returns a copy of a with every non-zero field of override applied
// on top. Neither receiver nor argument is mutated, so the construction-
// time Auth an adapter holds stays intact across concurrent requests —
// which is the whole reason this returns a value rather than mutating.
//
// Headers merge key by key rather than replacing the map wholesale: a
// per-request override that sets one header must not drop the adapter's
// own defaults.
func (a Auth) Merge(override Auth) Auth {
	out := a
	if override.APIKey != "" {
		out.APIKey = override.APIKey
	}
	if override.Bearer != "" {
		out.Bearer = override.Bearer
	}
	if override.BaseURL != "" {
		out.BaseURL = override.BaseURL
	}
	if len(override.Headers) > 0 {
		merged := make(map[string]string, len(out.Headers)+len(override.Headers))
		for k, v := range out.Headers {
			merged[k] = v
		}
		for k, v := range override.Headers {
			merged[k] = v
		}
		out.Headers = merged
	}
	return out
}

// Token returns the credential that should be carried, preferring the
// OAuth access token. Adapters that send both classes in the same header
// (`Authorization: Bearer`) use this instead of branching.
func (a Auth) Token() string {
	if a.Bearer != "" {
		return a.Bearer
	}
	return a.APIKey
}

// ---------------------------------------------------------------------------
// ModelRequest — the bridge between runtime / proxy and a Provider.
//
// Auth is optional: providers whose constructor already accepted a
// credential (anthropic.WithAPIKey, etc.) can ignore the field and use the
// baked-in secret. Providers built for OAuth dispatch (where the proxy
// resolves the access token per request) read req.Auth.
// ---------------------------------------------------------------------------

// ModelRequest is what runtime / proxy sends to a Provider.
type ModelRequest struct {
	Messages    []Message    `json:"messages"`
	Tools       []ToolSpec   `json:"tools,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	StopReasons []string     `json:"stop_reasons,omitempty"`

	// Auth overrides the provider's built-in credential for this call.
	// Empty value = use the credential bound at construction time.
	Auth Auth `json:"auth,omitempty"`
}

// ---------------------------------------------------------------------------
// Other ports — unchanged.
// ---------------------------------------------------------------------------

// StateStore persists State. Implementations must be safe for concurrent use
// across runs — RunID is the namespace.
//
// File-backed default lives in memory/filestore.JSONFileStateStore.
type StateStore interface {
	Save(ctx context.Context, s State) error
	Load(ctx context.Context, runID string) (State, error)
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, runID string) error
}

// WriteAheadLog is the append-only event log used for crash recovery.
// (Database term: append-only log of events; recovery replays from
// "sinceSeq" forward without re-issuing model calls.)
type WriteAheadLog interface {
	Append(ctx context.Context, runID string, seq int, ev Event) error
	Read(ctx context.Context, runID string, sinceSeq int) ([]Event, error)
	TruncateFrom(ctx context.Context, runID string, uptoSeq int) error
}

// ToolRegistry resolves a tool call to a tool by name. Concrete impl lives in
// action.Registry — it composes static registrations and dynamic ToolSource
// discoveries (e.g. MCP).
//
// Register accepts a Tool (metadata only) or a CallableTool (metadata +
// dispatch). For metadata-only tools, the caller must have registered a
// dispatch function via action.RegisterFunc beforehand.
type ToolRegistry interface {
	Get(name string) (Tool, bool)
	List() []ToolSpec
	Call(ctx context.Context, call ToolCall) ToolResult
}

// Tool describes a tool's metadata — no execution logic. Tools are pure
// business logic; JSON marshal/unmarshal/schema reflection is the
// Registry's responsibility (via action.RegisterFunc).
type Tool interface {
	Name() string
	Description() string
	Risk() RiskLevel
	Schema() ToolSpec
}

// CallableTool extends Tool with a Call method for tools that manage their
// own JSON dispatch — typically dynamic or composite tools like Spawner
// where the registry cannot reflect args from a static Go type.
type CallableTool interface {
	Tool
	Call(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

// Notifier mirrors gosdk/notify.Notifier exactly so the gosdk Multi / Stdout /
// Slack notifiers are structurally usable without an adapter.
//
// Method set intentionally matches Notify(ctx, message) error.
type Notifier interface {
	Notify(ctx context.Context, message string) error
}

// Compile-time assertion: any *anthropic.Provider, *google.Provider, and
// *ollama.Provider must satisfy Provider. The provider modules own
// their own compile-time assertion (they import core directly).
var _ Provider = nil