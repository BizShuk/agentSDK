package tool

import (
	"context"
	"errors"

	sdkcore "github.com/bizshuk/agentsdk/core"
	sdktool "github.com/bizshuk/agentsdk/tool"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
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

// Register registers CompleteTodo into the given sdktool.Registry.
func (c *CompleteTodo) Register(reg *sdktool.Registry) {
	sdktool.RegisterFunc(reg, "complete_todo", "Mark a todo as done by ID", sdkcore.RISK_LEVEL_LOW, c.Handle)
}

// Handle is pure business logic.
func (c *CompleteTodo) Handle(_ context.Context, args CompleteTodoArgs) (CompleteTodoOutput, error) {
	updated, ok := c.Store.Complete(args.ID)
	if !ok {
		return CompleteTodoOutput{}, errors.New("todo not found: " + args.ID)
	}
	return CompleteTodoOutput{ID: updated.ID, Status: string(updated.Status)}, nil
}
