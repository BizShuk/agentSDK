package tool

import (
	"context"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
)

// AddTodoArgs — TypedTool argument shape.
type AddTodoArgs struct {
	Title string `json:"title"`
}

// AddTodoOutput — TypedTool output shape.
type AddTodoOutput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// AddTodo is a TypedTool that adds a todo to the shared in-memory store.
type AddTodo struct {
	Inner *action.TypedTool[AddTodoArgs, AddTodoOutput]
	Store *domain.TodoStore
}

// NewAddTodo wires the TypedTool to a TodoStore.
func NewAddTodo(s *domain.TodoStore) *AddTodo {
	t := action.NewTypedTool("add_todo", "Add a remediation todo to the operator's queue",
		func(_ context.Context, a AddTodoArgs) (AddTodoOutput, error) {
			t := s.Add(a.Title)
			return AddTodoOutput{ID: t.ID, Status: string(t.Status)}, nil
		})
	t.SetRisk(sdkcore.RISK_LEVEL_LOW)
	return &AddTodo{Inner: t, Store: s}
}

func (a *AddTodo) Name() string             { return a.Inner.Name() }
func (a *AddTodo) Description() string      { return a.Inner.Description() }
func (a *AddTodo) Schema() sdkcore.ToolSpec { return a.Inner.Schema() }
func (a *AddTodo) Risk() sdkcore.RiskLevel  { return a.Inner.Risk() }
func (a *AddTodo) Call(ctx context.Context, args []byte) (sdkcore.ToolResult, error) {
	return a.Inner.Call(ctx, args)
}
