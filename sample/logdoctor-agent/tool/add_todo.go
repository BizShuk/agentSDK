package tool

import (
	"context"
	"encoding/json"

	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	sdktool "github.com/bizshuk/agentsdk/tool"
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

var _ sdktool.Tool = (*AddTodo)(nil)

// Register registers AddTodo into the given sdktool.Registry.
func (a *AddTodo) Register(reg *sdktool.Registry) {
	reg.Register(a)
}

// Name returns the registry name.
func (a *AddTodo) Name() string { return "add_todo" }

// Spec returns metadata and the reflected argument schema.
func (a *AddTodo) Spec() sdkcore.ToolSpec {
	return sdktool.MustSchemaForTool[AddTodoArgs](
		a.Name(),
		"Add a remediation todo to the operator's queue",
		sdkcore.RISK_LEVEL_LOW,
	)
}

// Call converts raw JSON arguments and executes the add operation.
func (a *AddTodo) Call(
	ctx context.Context,
	raw json.RawMessage,
) (sdkcore.ToolResult, error) {
	return sdktool.CallWithRawMessage(ctx, a.Name(), raw, a.execute)
}

func (a *AddTodo) execute(_ context.Context, args AddTodoArgs) (AddTodoOutput, error) {
	t := a.Store.Add(args.Title)
	return AddTodoOutput{ID: t.ID, Status: string(t.Status)}, nil
}
