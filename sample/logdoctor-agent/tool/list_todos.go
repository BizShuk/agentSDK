package tool

import (
	"context"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
)

// ListTodosArgs — TypedTool argument shape. Both fields optional
// (Status defaults to "open" when omitted).
type ListTodosArgs struct {
	Status string `json:"status,omitempty"`
}

// ListTodosOutput — TypedTool output shape.
type ListTodosOutput struct {
	Todos []domain.Todo `json:"todos"`
}

// ListTodos is a TypedTool that lists todos filtered by status.
type ListTodos struct {
	Inner *action.TypedTool[ListTodosArgs, ListTodosOutput]
	Store *domain.TodoStore
}

// NewListTodos wires the TypedTool to a TodoStore.
func NewListTodos(s *domain.TodoStore) *ListTodos {
	t := action.NewTypedTool("list_todos", "List todos, optionally filtered by status",
		func(_ context.Context, a ListTodosArgs) (ListTodosOutput, error) {
			all := s.List()
			out := all[:0]
			status := domain.TodoStatus(a.Status)
			if status == "" {
				status = domain.TODO_STATUS_OPEN
			}
			for _, it := range all {
				if status == "" || it.Status == status {
					out = append(out, it)
				}
			}
			return ListTodosOutput{Todos: out}, nil
		})
	t.SetRisk(sdkcore.RISK_LEVEL_LOW)
	return &ListTodos{Inner: t, Store: s}
}

func (l *ListTodos) Name() string              { return l.Inner.Name() }
func (l *ListTodos) Description() string       { return l.Inner.Description() }
func (l *ListTodos) Schema() sdkcore.ToolSpec { return l.Inner.Schema() }
func (l *ListTodos) Risk() sdkcore.RiskLevel    { return l.Inner.Risk() }
func (l *ListTodos) Call(ctx context.Context, args []byte) (sdkcore.ToolResult, error) {
	return l.Inner.Call(ctx, args)
}