package tool

import (
	"context"
	"errors"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor/core"
)

// CompleteTodoArgs — TypedTool argument shape.
type CompleteTodoArgs struct {
	ID string `json:"id"`
}

// CompleteTodoOutput — TypedTool output shape.
type CompleteTodoOutput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// CompleteTodo is a TypedTool that marks a todo as DONE.
type CompleteTodo struct {
	Inner *action.TypedTool[CompleteTodoArgs, CompleteTodoOutput]
	Store *domain.TodoStore
}

// NewCompleteTodo wires the TypedTool to a TodoStore.
func NewCompleteTodo(s *domain.TodoStore) *CompleteTodo {
	t := action.NewTypedTool("complete_todo", "Mark a todo as done by ID",
		func(_ context.Context, a CompleteTodoArgs) (CompleteTodoOutput, error) {
			updated, ok := s.Complete(a.ID)
			if !ok {
				return CompleteTodoOutput{}, errors.New("todo not found: " + a.ID)
			}
			return CompleteTodoOutput{ID: updated.ID, Status: string(updated.Status)}, nil
		})
	t.SetRisk(sdkcore.RISK_LEVEL_LOW)
	return &CompleteTodo{Inner: t, Store: s}
}

func (c *CompleteTodo) Name() string              { return c.Inner.Name() }
func (c *CompleteTodo) Description() string       { return c.Inner.Description() }
func (c *CompleteTodo) Schema() sdkcore.ToolSpec { return c.Inner.Schema() }
func (c *CompleteTodo) Risk() sdkcore.RiskLevel    { return c.Inner.Risk() }
func (c *CompleteTodo) Call(ctx context.Context, args []byte) (sdkcore.ToolResult, error) {
	return c.Inner.Call(ctx, args)
}