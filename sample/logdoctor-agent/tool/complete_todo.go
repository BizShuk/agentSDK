package tool

import (
	"context"
	"encoding/json"
	"errors"

	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	sdktool "github.com/bizshuk/agentsdk/tool"
)

// CompleteTodoArgs argument shape.
type CompleteTodoArgs struct {
	ID string `json:"id"`
}

// CompleteTodoOutput output shape.
type CompleteTodoOutput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// CompleteTodo marks a todo as DONE.
type CompleteTodo struct {
	Store *domain.TodoStore
}

// NewCompleteTodo constructs a CompleteTodo tool.
func NewCompleteTodo(s *domain.TodoStore) *CompleteTodo {
	return &CompleteTodo{Store: s}
}

var _ sdktool.Tool = (*CompleteTodo)(nil)

// Register registers CompleteTodo into the given sdktool.Registry.
func (c *CompleteTodo) Register(reg *sdktool.Registry) {
	reg.Register(c)
}

// Name returns the registry name.
func (c *CompleteTodo) Name() string { return "complete_todo" }

// Spec returns metadata and the reflected argument schema.
func (c *CompleteTodo) Spec() sdkcore.ToolSpec {
	return sdktool.MustSchemaForTool[CompleteTodoArgs](
		c.Name(),
		"Mark a todo as done by ID",
		sdkcore.RISK_LEVEL_LOW,
	)
}

// Call converts raw JSON arguments and executes the completion operation.
func (c *CompleteTodo) Call(
	ctx context.Context,
	raw json.RawMessage,
) (sdkcore.ToolResult, error) {
	return sdktool.CallWithRawMessage(ctx, c.Name(), raw, c.execute)
}

func (c *CompleteTodo) execute(
	_ context.Context,
	args CompleteTodoArgs,
) (CompleteTodoOutput, error) {
	updated, ok := c.Store.Complete(args.ID)
	if !ok {
		return CompleteTodoOutput{}, errors.New("todo not found: " + args.ID)
	}
	return CompleteTodoOutput{ID: updated.ID, Status: string(updated.Status)}, nil
}
