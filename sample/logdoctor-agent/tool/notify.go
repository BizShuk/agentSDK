package tool

import (
	"context"
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

// Register registers Notify into the given sdktool.Registry.
func (n *Notify) Register(reg *sdktool.Registry) {
	sdktool.RegisterFunc(reg, "notify", "Print a notification line to the operator", sdkcore.RISK_LEVEL_LOW, n.Handle)
}

// Handle is pure business logic.
func (n *Notify) Handle(_ context.Context, args NotifyArgs) (NotifyOutput, error) {
	level := args.Level
	if level == "" {
		level = "info"
	}
	fmt.Fprintf(n.w, "[notify][%s] %s\n", level, args.Message)
	return NotifyOutput{Delivered: true}, nil
}
