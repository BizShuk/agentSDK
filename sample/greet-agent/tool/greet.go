// Package tool holds the greet-agent sample's action-side primitives.
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	sdkcore "github.com/bizshuk/agentsdk/core"
	sdktool "github.com/bizshuk/agentsdk/tool"
)

// GreetArgs is the input shape the LLM sees for the greet tool.
// Fields without `omitempty` are inferred as required by the schema generator.
type GreetArgs struct {
	Name string `json:"name"`
}

// GreetOutput is the output shape returned to the LLM.
type GreetOutput struct {
	Reply string `json:"reply"`
}

// Greet holds the greet tool state and logic.
type Greet struct {
	risk sdkcore.RiskLevel
}

// NewGreet constructs a greet tool.
func NewGreet() *Greet {
	return &Greet{risk: sdkcore.RISK_LEVEL_LOW}
}

var _ sdktool.Tool = (*Greet)(nil)

// SetRisk lets the caller mark this tool as high-risk (triggers HITL approval).
func (g *Greet) SetRisk(r sdkcore.RiskLevel) { g.risk = r }

// Register registers Greet into the given sdktool.Registry.
func (g *Greet) Register(reg *sdktool.Registry) {
	reg.Register(g)
}

// Name returns the registry name.
func (g *Greet) Name() string { return "greet" }

// Spec returns metadata and the reflected argument schema.
func (g *Greet) Spec() sdkcore.ToolSpec {
	return sdktool.MustSchemaForTool[GreetArgs](
		g.Name(),
		"Greet someone by name and return a friendly reply",
		g.risk,
	)
}

// Call converts raw JSON arguments and executes the greet operation.
func (g *Greet) Call(
	ctx context.Context,
	raw json.RawMessage,
) (sdkcore.ToolResult, error) {
	return sdktool.CallWithRawMessage(ctx, g.Name(), raw, g.execute)
}

func (g *Greet) execute(_ context.Context, a GreetArgs) (GreetOutput, error) {
	if a.Name == "" {
		return GreetOutput{}, fmt.Errorf("name is required")
	}
	return GreetOutput{
		Reply: fmt.Sprintf("Hello, %s! Nice to meet you.", a.Name),
	}, nil
}
