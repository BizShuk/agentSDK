package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
)

// NotifyArgs — TypedTool argument shape.
type NotifyArgs struct {
	Level   string `json:"level,omitempty"`
	Message string `json:"message"`
}

// NotifyOutput — TypedTool output shape.
type NotifyOutput struct {
	Delivered bool `json:"delivered"`
}

// Notify is a TypedTool that emits a notification line on the bound writer.
// In M1 the writer is os.Stdout (the CLI surface); M4 swaps it for
// notify.NewMulti for production channels.
type Notify struct {
	Inner *action.TypedTool[NotifyArgs, NotifyOutput]
	w     io.Writer
}

// NewNotify wires the TypedTool to an io.Writer.
func NewNotify(w io.Writer) *Notify {
	t := action.NewTypedTool("notify", "Print a notification line to the operator",
		func(_ context.Context, a NotifyArgs) (NotifyOutput, error) {
			if a.Level == "" {
				a.Level = "info"
			}
			fmt.Fprintf(w, "[notify][%s] %s\n", a.Level, a.Message)
			return NotifyOutput{Delivered: true}, nil
		})
	return &Notify{Inner: t, w: w}
}

// Name delegates to the TypedTool.
func (n *Notify) Name() string             { return n.Inner.Name() }
func (n *Notify) Description() string      { return n.Inner.Description() }
func (n *Notify) Schema() sdkcore.ToolSpec { return n.Inner.Schema() }
func (n *Notify) Risk() sdkcore.RiskLevel  { return n.Inner.Risk() }
func (n *Notify) Call(ctx context.Context, args json.RawMessage) (sdkcore.ToolResult, error) {
	return n.Inner.Call(ctx, args)
}
