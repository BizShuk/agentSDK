package tool

import (
	"context"

	sdkcore "github.com/bizshuk/agentsdk/core"
	sdktool "github.com/bizshuk/agentsdk/tool"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
)

// ListTodosArgs is the input shape for the list_todos tool.
type ListTodosArgs struct {
	Status string `json:"status,omitempty"` // "pending", "done", or "" for all
}

// ListTodosOutput is the output shape returned to the LLM.
type ListTodosOutput struct {
	Todos []domain.Todo `json:"todos"`
}

// ListTodos lists remediation todos from the Store.
type ListTodos struct {
	Store *domain.TodoStore
}

// NewListTodos constructs a list_todos tool backed by store.
func NewListTodos(s *domain.TodoStore) *ListTodos {
	return &ListTodos{Store: s}
}

// Register registers ListTodos into the given sdktool.Registry.
func (l *ListTodos) Register(reg *sdktool.Registry) {
	sdktool.RegisterFunc(reg, "list_todos", "List todos, optionally filtered by status", sdkcore.RISK_LEVEL_LOW, l.Handle)
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
