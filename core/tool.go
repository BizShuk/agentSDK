package core

import (
	"context"
	"encoding/json"
)

// RiskLevel is set at tool definition time and consulted by ApprovalPolicy.
type RiskLevel string

const (
	RISK_LEVEL_LOW  RiskLevel = "low"
	RISK_LEVEL_HIGH RiskLevel = "high"
)

// ToolCall identifies a single tool invocation across model chunks,
// transcript parts, instructions, and replays.
type ToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
	Risk RiskLevel      `json:"risk,omitempty"`
}

// ToolResult is the canonical tool outcome carried by tools, transcript parts,
// and events.
type ToolResult struct {
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	Output    any    `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms,omitempty"`
}

// ToolSpec is what the LLM sees — the JSON shape of one tool's arguments.
// Parameters remains provider-neutral so each adapter can encode it in its own
// JSON Schema dialect.
type ToolSpec struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Parameters  any       `json:"parameters"` // JSON Schema object
	Risk        RiskLevel `json:"risk"`
}

// JSONSchema describes what a tool's Args struct accepts. It is generated
// by tool/schema.go from struct tags (jsonschema tag + json tag).
// This is a go-side annotation — the runtime-facing type is ToolSpec.
type JSONSchema struct {
	Type                 string                 `json:"type"`
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties any                    `json:"additionalProperties,omitempty"`
}

// Tool is the executable boundary shared by core, the registry, built-ins,
// samples, and dynamic adapters.
type Tool interface {
	Name() string
	Spec() ToolSpec
	Call(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

// ToolRegistry resolves and dispatches tools by name. The concrete
// implementation lives in tool.Registry.
type ToolRegistry interface {
	Get(name string) (Tool, bool)
	List() []ToolSpec
	Call(ctx context.Context, call ToolCall) ToolResult
}
