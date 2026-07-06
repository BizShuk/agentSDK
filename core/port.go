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

// ModelRequest is the bridge between Step-produced effects and the provider.
type ModelRequest struct {
	Messages  []Message     `json:"messages"`
	Tools     []ToolSchema  `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
	StopReasons []string    `json:"stop_reasons,omitempty"`
}

// StateStore persists State. Implementations must be safe for concurrent use
// across runs — RunID is the namespace.
//
// File-backed default lives in memory/filestore.FileStateStore (M2).
type StateStore interface {
	Save(ctx context.Context, s State) error
	Load(ctx context.Context, runID string) (State, error)
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, runID string) error
}

// WAL is the append-only replay log. Replay returns every Input whose Seq > sinceSeq.
// Effective recovery: load State, replay Inputs since LastInputSeq.
//
// File-backed default lives in memory/filestore.FileWAL (M2).
type WAL interface {
	Append(ctx context.Context, runID string, seq int, in Input) error
	Replay(ctx context.Context, runID string, sinceSeq int) ([]Input, error)
	Truncate(ctx context.Context, runID string, uptoSeq int) error
}

// ToolRegistry resolves a tool call to a tool by name. Concrete impl lives in
// action.Registry — it composes static registrations and dynamic ToolSource
// discoveries (e.g. MCP).
type ToolRegistry interface {
	Register(t Tool)
	Get(name string) (Tool, bool)
	List() []ToolSchema
	Call(ctx context.Context, call ToolCall) ToolResult
}

// Tool is the runtime-side contract — what Registry dispatches into.
//
// Note: action.Tool extends this with a typed-call helper.
type Tool interface {
	Name() string
	Description() string
	Schema() ToolSchema
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
