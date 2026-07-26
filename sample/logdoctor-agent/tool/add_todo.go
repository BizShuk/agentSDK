package tool

import (
	"context"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
)

// AddTodoArgs argument shape.
type AddTodoArgs struct {
	Title string `json:"title"`
}

// AddTodoOutput output shape.
type AddTodoOutput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// AddTodo adds a todo to the shared in-memory store.
type AddTodo struct {
	Store *domain.TodoStore
}

// NewAddTodo constructs an AddTodo tool.
func NewAddTodo(s *domain.TodoStore) *AddTodo {
	return &AddTodo{Store: s}
}

// Register registers AddTodo into the given action.Registry.
func (a *AddTodo) Register(reg *action.Registry) {
	action.RegisterFunc(reg, "add_todo", "Add a remediation todo to the operator's queue", sdkcore.RISK_LEVEL_LOW, a.Handle)
}

// Handle is pure business logic.
func (a *AddTodo) Handle(_ context.Context, args AddTodoArgs) (AddTodoOutput, error) {
	t := a.Store.Add(args.Title)
	return AddTodoOutput{ID: t.ID, Status: string(t.Status)}, nil
}
