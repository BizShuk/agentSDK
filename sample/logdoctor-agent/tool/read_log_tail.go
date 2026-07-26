// Package tool holds the logdoctor sample's action-side primitives:
// read_log_tail, notify, and the M3+M4 ones (add_todo, propose_fix, ...).
package tool

import (
	"context"
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

// Register registers ReadLogTail into the given sdktool.Registry.
func (r *ReadLogTail) Register(reg *sdktool.Registry) {
	sdktool.RegisterFunc(reg, "read_log_tail", "Read up to N lines from the watched log file", sdkcore.RISK_LEVEL_LOW, r.Handle)
}

// Handle is pure business logic.
func (r *ReadLogTail) Handle(ctx context.Context, args ReadLogTailArgs) (ReadLogTailOutput, error) {
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
