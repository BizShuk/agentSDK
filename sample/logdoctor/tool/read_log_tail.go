// Package tool holds the logdoctor sample's action-side primitives:
// read_log_tail, notify, and the M3+M4 ones (add_todo, propose_fix, ...).
package tool

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
)

// Source is the minimal surface read_log_tail needs from a listener. Defined
// here so the tool does not depend on the listener's full type and tests can
// pass a stub source.
//
// The Percept comes from agentsdk/core — Percept is part of the SDK's
// public API; the listener (in sample/logdoctor/core) returns sdkcore.Percept.
type Source interface {
	Percepts(ctx context.Context) <-chan sdkcore.Percept
}

// ReadLogTailArgs — TypedTool argument shape. N is optional (defaults to 20).
type ReadLogTailArgs struct {
	N int `json:"n,omitempty"`
}

// ReadLogTailOutput — TypedTool output shape.
type ReadLogTailOutput struct {
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated,omitempty"`
}

// ReadLogTail is a TypedTool that returns up to N lines from the listener's
// first percept. In M1 the listener only emits one Percept (the entire
// file); M2 will let this tool page through a tail cursor.
type ReadLogTail struct {
	Inner *action.TypedTool[ReadLogTailArgs, ReadLogTailOutput]
	src   Source
}

// NewReadLogTail wires the TypedTool to a Source.
func NewReadLogTail(src Source) *ReadLogTail {
	t := action.NewTypedTool("read_log_tail", "Read up to N lines from the watched log file",
		func(ctx context.Context, a ReadLogTailArgs) (ReadLogTailOutput, error) {
			if a.N <= 0 {
				a.N = 20
			}
			var payload string
			select {
			case p, ok := <-src.Percepts(ctx):
				if !ok {
					return ReadLogTailOutput{Lines: []string{}}, nil
				}
				payload, _ = p.Payload.(string)
			case <-ctx.Done():
				return ReadLogTailOutput{}, ctx.Err()
			}
			lines := strings.Split(payload, "\n")
			truncated := false
			if len(lines) > a.N {
				lines = lines[:a.N]
				truncated = true
			}
			return ReadLogTailOutput{Lines: lines, Truncated: truncated}, nil
		})
	return &ReadLogTail{Inner: t, src: src}
}

// Name delegates to the TypedTool.
func (r *ReadLogTail) Name() string                { return r.Inner.Name() }
func (r *ReadLogTail) Description() string         { return r.Inner.Description() }
func (r *ReadLogTail) Schema() sdkcore.ToolSchema  { return r.Inner.Schema() }
func (r *ReadLogTail) Risk() sdkcore.RiskLevel     { return r.Inner.Risk() }
func (r *ReadLogTail) Call(ctx context.Context, args json.RawMessage) (sdkcore.ToolResult, error) {
	return r.Inner.Call(ctx, args)
}