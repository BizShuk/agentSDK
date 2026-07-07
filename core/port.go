package core

import (
	"context"
	"encoding/json"
)

// ModelProvider is the LLM-side port. Adapters live under provider/*.
//
// Three methods reflect three distinct capabilities:
//   - Generate:     blocking, returns full result.
//   - Stream:       returns a channel; runtime folds into ModelResult.
//   - CountTokens:  separate to allow cheap heuristics for non-Anthropic.
type ModelProvider interface {
	Name() string
	Generate(ctx context.Context, req ModelRequest) (ModelResult, error)
	Stream(ctx context.Context, req ModelRequest) (<-chan ModelChunk, error)
	CountTokens(ctx context.Context, msgs []Message) (int, error)
}

// ModelRequest is the bridge between Decide-produced instructions and the provider.
type ModelRequest struct {
	Messages    []Message    `json:"messages"`
	Tools       []ToolSpec   `json:"tools,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	StopReasons []string     `json:"stop_reasons,omitempty"`
}

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
type ToolRegistry interface {
	Register(t Tool)
	Get(name string) (Tool, bool)
	List() []ToolSpec
	Call(ctx context.Context, call ToolCall) ToolResult
}

// Tool is the runtime-side contract — what Registry dispatches into.
//
// Note: action.Tool extends this with a typed-call helper.
type Tool interface {
	Name() string
	Description() string
	Schema() ToolSpec
	Risk() RiskLevel
	Call(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

// Notifier mirrors gosdk/notify.Notifier exactly so the gosdk Multi / Stdout /
// Slack notifiers are structurally usable without an adapter.
//
// Method set intentionally matches Notify(ctx, message) error.
type Notifier interface {
	Notify(ctx context.Context, message string) error
}
