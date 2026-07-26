// Package tool holds the greet-agent sample's action-side primitives.
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
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

// Greet wraps a TypedTool so callers get a concrete type that satisfies
// core.Tool. The inner TypedTool handles JSON marshal / unmarshal / schema
// generation; the wrapper just delegates.
type Greet struct {
	Inner *action.TypedTool[GreetArgs, GreetOutput]
}

// NewGreet constructs a greet tool. The function is pure business logic —
// no JSON, no schema, no error wrapping. TypedTool takes care of all of that.
func NewGreet() *Greet {
	t := action.NewTypedTool("greet",
		"Greet someone by name and return a friendly reply",
		func(_ context.Context, a GreetArgs) (GreetOutput, error) {
			if a.Name == "" {
				return GreetOutput{}, fmt.Errorf("name is required")
			}
			return GreetOutput{
				Reply: fmt.Sprintf("Hello, %s! Nice to meet you.", a.Name),
			}, nil
		})
	return &Greet{Inner: t}
}

// SetRisk lets the caller mark this tool as high-risk (triggers HITL approval).
func (g *Greet) SetRisk(r sdkcore.RiskLevel) { g.Inner.SetRisk(r) }

// --- core.Tool interface (all delegate to Inner) ---

func (g *Greet) Name() string             { return g.Inner.Name() }
func (g *Greet) Description() string      { return g.Inner.Description() }
func (g *Greet) Schema() sdkcore.ToolSpec { return g.Inner.Schema() }
func (g *Greet) Risk() sdkcore.RiskLevel  { return g.Inner.Risk() }
func (g *Greet) Call(ctx context.Context, raw json.RawMessage) (sdkcore.ToolResult, error) {
	return g.Inner.Call(ctx, raw)
}
