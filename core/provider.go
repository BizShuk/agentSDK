package core

import "context"

// Provider is the outer interface every agentsdk provider package implements.
//
// Implementations must:
//   - be safe to call concurrently;
//   - honor context cancellation for every blocking call;
//   - return upstream errors for the runtime boundary to wrap.
type Provider interface {
	// ID reports the provider family. It must not carry a model ID.
	ID() string

	// Models returns the provider's bundled catalog without I/O. Prefer
	// ModelLister when the provider supports a live catalog.
	Models() []ModelSpec

	// AuthSchemes advertises which credential kinds the provider accepts:
	// api_key, oauth, or keyless.
	AuthSchemes() []string

	// Generate sends a blocking request and returns the full ModelResult.
	Generate(ctx context.Context, request ModelRequest) (ModelResult, error)

	// Stream returns model chunks. A clean stream ends with Done=true. If
	// transport reading fails after Stream returns, implementations close the
	// channel without Done so consumers can reject a partial response.
	Stream(ctx context.Context, request ModelRequest) (<-chan ModelChunk, error)

	// CountTokens returns an approximate token count for a transcript.
	CountTokens(ctx context.Context, messages []Message) (int, error)
}

// ModelLister is the optional live-catalog half of Provider. Adapters whose
// upstream publishes a catalog endpoint implement it so callers can prefer
// current model IDs over the bundled snapshot. It stays separate because some
// upstreams expose no catalog endpoint.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelSpec, error)
}
