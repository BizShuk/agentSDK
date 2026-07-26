package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	sdkcore "github.com/bizshuk/agentsdk/core"
	sdktool "github.com/bizshuk/agentsdk/tool"
)

// NotifyArgs argument shape.
type NotifyArgs struct {
	Level   string `json:"level,omitempty"`
	Message string `json:"message"`
}

// NotifyOutput output shape.
type NotifyOutput struct {
	Delivered bool `json:"delivered"`
}

// Notify emits a notification line on the bound writer.
type Notify struct {
	w io.Writer
}

// NewNotify constructs a Notify tool.
func NewNotify(w io.Writer) *Notify {
	return &Notify{w: w}
}

var _ sdktool.Tool = (*Notify)(nil)

// Register registers Notify into the given sdktool.Registry.
func (n *Notify) Register(reg *sdktool.Registry) {
	reg.Register(n)
}

// Name returns the registry name.
func (n *Notify) Name() string { return "notify" }

// Spec returns metadata and the reflected argument schema.
func (n *Notify) Spec() sdkcore.ToolSpec {
	return sdktool.MustSchemaForTool[NotifyArgs](
		n.Name(),
		"Print a notification line to the operator",
		sdkcore.RISK_LEVEL_LOW,
	)
}

// Call converts raw JSON arguments and executes the notification operation.
func (n *Notify) Call(
	ctx context.Context,
	raw json.RawMessage,
) (sdkcore.ToolResult, error) {
	return sdktool.CallWithRawMessage(ctx, n.Name(), raw, n.execute)
}

func (n *Notify) execute(_ context.Context, args NotifyArgs) (NotifyOutput, error) {
	level := args.Level
	if level == "" {
		level = "info"
	}
	fmt.Fprintf(n.w, "[notify][%s] %s\n", level, args.Message)
	return NotifyOutput{Delivered: true}, nil
}
