package action

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// entry is the internal storage unit — metadata plus a typed dispatch
// function. The dispatch function is set by RegisterFunc (typed path) or
// RegisterCallable (self-dispatch path for Spawner-like tools).
type entry struct {
	spec core.ToolSpec
	risk core.RiskLevel
	desc string
	call func(ctx context.Context, raw json.RawMessage) (core.ToolResult, error)
}

// Registry is the in-memory tool registry. Tools are registered via
// RegisterFunc (typed, schema-reflected) or RegisterCallable (self-dispatch).
type Registry struct {
	mu      sync.RWMutex
	entries map[string]entry
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]entry, 8)}
}

// RegisterFunc registers a typed Go function as a tool. Schema is
// auto-reflected from TArgs; JSON unmarshal/marshal is handled by
// the registry — the function never touches json.RawMessage.
//
// This is the primary registration path for tools whose args are a
// static Go struct (all 6 built-ins, sample tools, test tools).
func RegisterFunc[TArgs any, TOut any](
	r *Registry,
	name, desc string,
	risk core.RiskLevel,
	fn func(ctx context.Context, args TArgs) (TOut, error),
) {
	spec, err := SchemaForTool[TArgs](name, desc, risk)
	if err != nil {
		// Schema reflection failure at registration time is a programming
		// error — panic like a nil map assignment would.
		panic(fmt.Sprintf("action.RegisterFunc(%q): schema: %v", name, err))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[name] = entry{
		spec: spec,
		risk: risk,
		desc: desc,
		call: typedCall[TArgs, TOut](name, fn),
	}
}

// RegisterCallable registers a tool that manages its own JSON dispatch.
// Used by composite tools like skill.Spawner where the registry cannot
// reflect args from a static Go type.
func (r *Registry) RegisterCallable(t core.CallableTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[t.Name()] = entry{
		spec: t.Schema(),
		risk: t.Risk(),
		desc: t.Description(),
		call: t.Call,
	}
}

// Register registers a metadata-only Tool with a pre-registered call
// function. This is a convenience for tools that assemble their own
// ToolSpec but still want registry dispatch. If the tool also implements
// CallableTool, its Call method is used directly.
func (r *Registry) Register(t core.Tool) {
	if ct, ok := t.(core.CallableTool); ok {
		r.RegisterCallable(ct)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Metadata-only registration — Call will panic if invoked.
	r.entries[t.Name()] = entry{
		spec: t.Schema(),
		risk: t.Risk(),
		desc: t.Description(),
		call: func(_ context.Context, _ json.RawMessage) (core.ToolResult, error) {
			return core.ToolResult{
				Name:  t.Name(),
				OK:    false,
				Error: fmt.Sprintf("tool %q was registered without a call function", t.Name()),
			}, nil
		},
	}
}

// Get returns the tool spec and whether it was found.
func (r *Registry) Get(name string) (core.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	if !ok {
		return nil, false
	}
	return &staticTool{spec: e.spec, risk: e.risk, desc: e.desc}, ok
}

// List returns schemas of all registered tools (copy).
func (r *Registry) List() []core.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]core.ToolSpec, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.spec)
	}
	return out
}

// Call dispatches to a tool by name.
func (r *Registry) Call(ctx context.Context, call core.ToolCall) core.ToolResult {
	r.mu.RLock()
	e, ok := r.entries[call.Name]
	r.mu.RUnlock()
	if !ok {
		return core.ToolResult{
			CallID: call.ID,
			Name:   call.Name,
			OK:     false,
			Error:  fmt.Sprintf("tool not found: %s", call.Name),
		}
	}
	raw, err := marshalArgs(call.Args)
	if err != nil {
		return core.ToolResult{CallID: call.ID, Name: call.Name, OK: false, Error: err.Error()}
	}
	res, _ := e.call(ctx, raw)
	if res.CallID == "" {
		res.CallID = call.ID
	}
	if res.Name == "" {
		res.Name = call.Name
	}
	return res
}

func marshalArgs(args map[string]any) (json.RawMessage, error) {
	if args == nil {
		return nil, nil
	}
	return json.Marshal(args)
}

// typedCall builds the dispatch closure for RegisterFunc — unmarshal,
// validate, call fn, marshal result.
func typedCall[TArgs any, TOut any](
	name string,
	fn func(ctx context.Context, args TArgs) (TOut, error),
) func(ctx context.Context, raw json.RawMessage) (core.ToolResult, error) {
	return func(ctx context.Context, raw json.RawMessage) (core.ToolResult, error) {
		if ok, err := ValidateArgs[TArgs](name, raw); !ok {
			return core.ToolResult{Name: name, OK: false, Error: err.Error()}, nil
		}
		var args TArgs
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &args); err != nil {
				return core.ToolResult{Name: name, OK: false, Error: "invalid args: " + err.Error()}, nil
			}
		}
		out, err := fn(ctx, args)
		if err != nil {
			return core.ToolResult{Name: name, OK: false, Error: err.Error()}, nil
		}
		encoded, mErr := json.Marshal(out)
		if mErr != nil {
			return core.ToolResult{Name: name, OK: true, Output: fmt.Sprintf("%v", out)}, nil
		}
		return core.ToolResult{Name: name, OK: true, Output: json.RawMessage(encoded)}, nil
	}
}

// staticTool is a read-only core.Tool returned by Get.
type staticTool struct {
	spec core.ToolSpec
	risk core.RiskLevel
	desc string
}

func (s *staticTool) Name() string          { return s.spec.Name }
func (s *staticTool) Description() string   { return s.desc }
func (s *staticTool) Risk() core.RiskLevel  { return s.risk }
func (s *staticTool) Schema() core.ToolSpec { return s.spec }

// ToolSource is the dynamic discovery interface (MCP shape).
//
// Implementations live under mcp/ (added in M3).
type ToolSource interface {
	Discover(ctx context.Context) ([]core.ToolSpec, error)
	Call(ctx context.Context, name string, args json.RawMessage) (core.ToolResult, error)
}