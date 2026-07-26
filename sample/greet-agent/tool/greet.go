// Package tool holds the greet-agent sample's action-side primitives.
package tool

import (
	"context"
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

// Greet holds the greet tool state and logic.
type Greet struct {
	risk sdkcore.RiskLevel
}

// NewGreet constructs a greet tool.
func NewGreet() *Greet {
	return &Greet{risk: sdkcore.RISK_LEVEL_LOW}
}

// SetRisk lets the caller mark this tool as high-risk (triggers HITL approval).
func (g *Greet) SetRisk(r sdkcore.RiskLevel) { g.risk = r }

// Register registers Greet into the given action.Registry.
func (g *Greet) Register(reg *action.Registry) {
	action.RegisterFunc(reg, "greet", "Greet someone by name and return a friendly reply", g.risk, g.Handle)
}

// Handle is pure business logic.
func (g *Greet) Handle(_ context.Context, a GreetArgs) (GreetOutput, error) {
	if a.Name == "" {
		return GreetOutput{}, fmt.Errorf("name is required")
	}
	return GreetOutput{
		Reply: fmt.Sprintf("Hello, %s! Nice to meet you.", a.Name),
	}, nil
}
