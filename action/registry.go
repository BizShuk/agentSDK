package action

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// Registry is the in-memory tool registry. M3 will add ToolSource injection
// (MCP, dynamic discovery); M1 only handles static Register.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]core.Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]core.Tool, 8)}
}

// Register adds a tool. Re-registering the same name overwrites — caller's
// responsibility to avoid collisions.
func (r *Registry) Register(t core.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns the tool and whether it was found.
func (r *Registry) Get(name string) (core.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns schemas of all registered tools (copy).
func (r *Registry) List() []core.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]core.ToolSpec, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Schema())
	}
	return out
}

// Call dispatches to a tool by name.
func (r *Registry) Call(ctx context.Context, call core.ToolCall) core.ToolResult {
	r.mu.RLock()
	t, ok := r.tools[call.Name]
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
	res, _ := t.Call(ctx, raw)
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

// ToolSource is the dynamic discovery interface (MCP shape).
//
// Implementations live under mcp/ (added in M3).
type ToolSource interface {
	Discover(ctx context.Context) ([]core.ToolSpec, error)
	Call(ctx context.Context, name string, args json.RawMessage) (core.ToolResult, error)
}