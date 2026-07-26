// Package tool holds the logdoctor sample's action-side primitives:
// read_log_tail, notify, and the M3+M4 ones (add_todo, propose_fix, ...).
package tool

import (
	"context"
	"encoding/json"
	"strings"

	sdkcore "github.com/bizshuk/agentsdk/core"
	sdktool "github.com/bizshuk/agentsdk/tool"
)

// Source is the minimal surface read_log_tail needs from a listener.
type Source interface {
	Observations(ctx context.Context) <-chan sdkcore.Observation
}

// ReadLogTailArgs argument shape. N is optional (defaults to 20).
type ReadLogTailArgs struct {
	N int `json:"n,omitempty"`
}

// ReadLogTailOutput output shape.
type ReadLogTailOutput struct {
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated,omitempty"`
}

// ReadLogTail returns up to N lines from the listener's first observation.
type ReadLogTail struct {
	src Source
}

// NewReadLogTail constructs a ReadLogTail tool.
func NewReadLogTail(src Source) *ReadLogTail {
	return &ReadLogTail{src: src}
}

var _ sdktool.Tool = (*ReadLogTail)(nil)

// Register registers ReadLogTail into the given sdktool.Registry.
func (r *ReadLogTail) Register(reg *sdktool.Registry) {
	reg.Register(r)
}

// Name returns the registry name.
func (r *ReadLogTail) Name() string { return "read_log_tail" }

// Spec returns metadata and the reflected argument schema.
func (r *ReadLogTail) Spec() sdkcore.ToolSpec {
	return sdktool.MustSchemaForTool[ReadLogTailArgs](
		r.Name(),
		"Read up to N lines from the watched log file",
		sdkcore.RISK_LEVEL_LOW,
	)
}

// Call converts raw JSON arguments and executes the tail operation.
func (r *ReadLogTail) Call(
	ctx context.Context,
	raw json.RawMessage,
) (sdkcore.ToolResult, error) {
	return sdktool.CallWithRawMessage(ctx, r.Name(), raw, r.execute)
}

func (r *ReadLogTail) execute(
	ctx context.Context,
	args ReadLogTailArgs,
) (ReadLogTailOutput, error) {
	if args.N <= 0 {
		args.N = 20
	}
	var payload string
	select {
	case p, ok := <-r.src.Observations(ctx):
		if !ok {
			return ReadLogTailOutput{Lines: []string{}}, nil
		}
		payload, _ = p.Payload.(string)
	case <-ctx.Done():
		return ReadLogTailOutput{}, ctx.Err()
	}
	lines := strings.Split(payload, "\n")
	truncated := false
	if len(lines) > args.N {
		lines = lines[:args.N]
		truncated = true
	}
	return ReadLogTailOutput{Lines: lines, Truncated: truncated}, nil
}
