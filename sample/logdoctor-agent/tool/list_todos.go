package tool

import (
	"context"
	"encoding/json"

	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	sdktool "github.com/bizshuk/agentsdk/tool"
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

var _ sdktool.Tool = (*ListTodos)(nil)

// Register registers ListTodos into the given sdktool.Registry.
func (l *ListTodos) Register(reg *sdktool.Registry) {
	reg.Register(l)
}

// Name returns the registry name.
func (l *ListTodos) Name() string { return "list_todos" }

// Spec returns metadata and the reflected argument schema.
func (l *ListTodos) Spec() sdkcore.ToolSpec {
	return sdktool.MustSchemaForTool[ListTodosArgs](
		l.Name(),
		"List todos, optionally filtered by status",
		sdkcore.RISK_LEVEL_LOW,
	)
}

// Call converts raw JSON arguments and executes the list operation.
func (l *ListTodos) Call(
	ctx context.Context,
	raw json.RawMessage,
) (sdkcore.ToolResult, error) {
	return sdktool.CallWithRawMessage(ctx, l.Name(), raw, l.execute)
}

func (l *ListTodos) execute(
	_ context.Context,
	args ListTodosArgs,
) (ListTodosOutput, error) {
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
