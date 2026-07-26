package tool

import (
	"context"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
)

// ListTodosArgs argument shape.
type ListTodosArgs struct {
	Status string `json:"status,omitempty"`
}

// ListTodosOutput output shape.
type ListTodosOutput struct {
	Todos []domain.Todo `json:"todos"`
}

// ListTodos lists todos filtered by status.
type ListTodos struct {
	Store *domain.TodoStore
}

// NewListTodos constructs a ListTodos tool.
func NewListTodos(s *domain.TodoStore) *ListTodos {
	return &ListTodos{Store: s}
}

// Register registers ListTodos into the given action.Registry.
func (l *ListTodos) Register(reg *action.Registry) {
	action.RegisterFunc(reg, "list_todos", "List todos, optionally filtered by status", sdkcore.RISK_LEVEL_LOW, l.Handle)
}

// Handle is pure business logic.
func (l *ListTodos) Handle(_ context.Context, args ListTodosArgs) (ListTodosOutput, error) {
	all := l.Store.List()
	out := make([]domain.Todo, 0, len(all))
	status := domain.TodoStatus(args.Status)
	if status == "" {
		status = domain.TODO_STATUS_OPEN
	}
	for _, it := range all {
		if status == "" || it.Status == status {
			out = append(out, it)
		}
	}
	return ListTodosOutput{Todos: out}, nil
}
