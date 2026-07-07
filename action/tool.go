// Package action is the output side of the agent loop: tools, sandboxing,
// registry, approval policy.
package action

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// TypedTool wraps a typed Go function so callers do not write the JSON
// marshal / unmarshal / error wrapping boilerplate per tool.
//
// Args schema is auto-generated from TArgs via reflection (see schema.go)
// — callers no longer supply a hand-written schema. Optional Arguments
// must use `omitempty` in their json tag so jsonschema can mark them
// non-required; fields without `omitempty` are inferred as required.
type TypedTool[TArgs any, TOut any] struct {
	NameV  string
	DescV  string
	RiskV  core.RiskLevel
	Fn     func(ctx context.Context, args TArgs) (TOut, error)

	schemaOnce sync.Once
	schema     core.ToolSpec
	schemaErr  error
}

// NewTypedTool is the sugar constructor.
func NewTypedTool[TArgs any, TOut any](name, desc string, fn func(ctx context.Context, args TArgs) (TOut, error)) *TypedTool[TArgs, TOut] {
	return &TypedTool[TArgs, TOut]{
		NameV: name,
		DescV: desc,
		RiskV: core.RISK_LEVEL_LOW,
		Fn:    fn,
	}
}

// SetRisk changes the tool's risk level — call before Register if the
// tool has side effects the user must approve.
func (t *TypedTool[TArgs, TOut]) SetRisk(r core.RiskLevel) { t.RiskV = r }

// Name returns the tool name.
func (t *TypedTool[TArgs, TOut]) Name() string { return t.NameV }

// Description returns the tool description.
func (t *TypedTool[TArgs, TOut]) Description() string { return t.DescV }

// Schema returns the JSON schema for the tool's args (computed once
// and cached). Errors during reflection are surfaced as an empty
// schema — the rest of the tool still works, just without a schema.
func (t *TypedTool[TArgs, TOut]) Schema() core.ToolSpec {
	t.schemaOnce.Do(func() {
		s, err := SchemaForTool[TArgs](t.NameV, t.DescV, t.RiskV)
		t.schemaErr = err
		t.schema = s
	})
	return t.schema
}

// Risk returns the configured risk level.
func (t *TypedTool[TArgs, TOut]) Risk() core.RiskLevel { return t.RiskV }

// Call dispatches: validate args, unmarshal, call fn, wrap result.
func (t *TypedTool[TArgs, TOut]) Call(ctx context.Context, raw json.RawMessage) (core.ToolResult, error) {
	// M3: validate args against the schema before invoking the fn.
	if ok, err := ValidateArgs[TArgs](t.NameV, raw); !ok {
		return core.ToolResult{Name: t.NameV, OK: false, Error: err.Error()}, nil
	}
	var args TArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return core.ToolResult{Name: t.NameV, OK: false, Error: "invalid args: " + err.Error()}, nil
		}
	}
	out, err := t.Fn(ctx, args)
	if err != nil {
		return core.ToolResult{Name: t.NameV, OK: false, Error: err.Error()}, nil
	}
	// best-effort: encode output as JSON so the LLM can read it.
	encoded, mErr := json.Marshal(out)
	if mErr != nil {
		return core.ToolResult{Name: t.NameV, OK: true, Output: fmt.Sprintf("%v", out)}, nil
	}
	return core.ToolResult{Name: t.NameV, OK: true, Output: json.RawMessage(encoded)}, nil
}