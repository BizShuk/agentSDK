package core

import "context"

// Provider is the runtime's blocking model-generation port.
//
// Implementations must:
//   - be safe to call concurrently;
//   - honor context cancellation for every blocking call;
//   - return upstream errors for the runtime boundary to wrap.
type Provider interface {
	// Generate sends a blocking request and returns the full ModelResult.
	Generate(ctx context.Context, request ModelRequest) (ModelResult, error)
}

// StreamProvider is the optional streaming capability implemented by model
// providers that can deliver incremental chunks.
type StreamProvider interface {
	// Stream returns model chunks. A clean stream ends with Done=true. If
	// transport reading fails after Stream returns, implementations close the
	// channel without Done so consumers can reject a partial response.
	Stream(ctx context.Context, request ModelRequest) (<-chan ModelChunk, error)
}

// ModelLister is the optional live-catalog capability. Adapters whose
// upstream publishes a catalog endpoint implement it so callers can prefer
// current model IDs over the bundled snapshot. It stays separate because some
// upstreams expose no catalog endpoint.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelSpec, error)
}
