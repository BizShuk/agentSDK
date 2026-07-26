package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// entry is the internal storage unit — metadata plus raw JSON dispatch.
type entry struct {
	spec core.ToolSpec
	call func(ctx context.Context, raw json.RawMessage) (core.ToolResult, error)
}

// Registry is the in-memory tool registry.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]entry
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]entry, 8)}
}

// funcTool adapts a typed Go function to core.Tool.
type funcTool[TArgs any, TOut any] struct {
	spec core.ToolSpec
	call func(ctx context.Context, args TArgs) (TOut, error)
}

func (f funcTool[TArgs, TOut]) Name() string        { return f.spec.Name }
func (f funcTool[TArgs, TOut]) Spec() core.ToolSpec { return f.spec }
func (f funcTool[TArgs, TOut]) Call(
	ctx context.Context,
	raw json.RawMessage,
) (core.ToolResult, error) {
	return CallWithRawMessage(ctx, f.Name(), raw, f.call)
}

// RegisterFunc registers a typed Go function as a tool. Schema is
// auto-reflected from TArgs; the function never touches json.RawMessage.
func RegisterFunc[TArgs any, TOut any](
	r *Registry,
	name, desc string,
	risk core.RiskLevel,
	call func(ctx context.Context, args TArgs) (TOut, error),
) {
	spec, err := SchemaForTool[TArgs](name, desc, risk)
	if err != nil {
		panic(fmt.Sprintf("tool.RegisterFunc(%q): schema: %v", name, err))
	}
	r.Register(funcTool[TArgs, TOut]{spec: spec, call: call})
}

// Register adds an executable tool to the registry.
func (r *Registry) Register(t core.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[t.Name()] = entry{
		spec: t.Spec(),
		call: t.Call,
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
	return &staticTool{spec: e.spec, call: e.call}, ok
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
	res, err := e.call(ctx, raw)
	if err != nil {
		return core.ToolResult{
			CallID: call.ID,
			Name:   call.Name,
			OK:     false,
			Error:  err.Error(),
		}
	}
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

// staticTool is a core.Tool view returned by Get.
type staticTool struct {
	spec core.ToolSpec
	call func(context.Context, json.RawMessage) (core.ToolResult, error)
}

func (s *staticTool) Name() string        { return s.spec.Name }
func (s *staticTool) Spec() core.ToolSpec { return s.spec }
func (s *staticTool) Call(
	ctx context.Context,
	raw json.RawMessage,
) (core.ToolResult, error) {
	return s.call(ctx, raw)
}

// ToolSource is the dynamic discovery interface (MCP shape).
//
// Implementations live under mcp/ (added in M3).
type ToolSource interface {
	Discover(ctx context.Context) ([]core.ToolSpec, error)
	Call(ctx context.Context, name string, args json.RawMessage) (core.ToolResult, error)
}
