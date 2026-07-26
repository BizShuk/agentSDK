package tool_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type echoArgs struct {
	Message string `json:"message"`
}
type echoOut struct {
	Echo string `json:"echo"`
}

func TestRegisterFuncCall(t *testing.T) {
	r := tool.NewRegistry()
	tool.RegisterFunc(r, "echo", "echo back the message", core.RISK_LEVEL_LOW,
		func(ctx context.Context, a echoArgs) (echoOut, error) {
			return echoOut{Echo: a.Message}, nil
		})

	res := r.Call(context.Background(), core.ToolCall{ID: "c1", Name: "echo", Args: map[string]any{"message": "hi"}})
	assert.True(t, res.OK)
	assert.Equal(t, "echo", res.Name)
	assert.Equal(t, "c1", res.CallID)
}

func TestRegisterFuncError(t *testing.T) {
	r := tool.NewRegistry()
	tool.RegisterFunc(r, "fail", "always fails", core.RISK_LEVEL_LOW,
		func(ctx context.Context, a echoArgs) (echoOut, error) {
			return echoOut{}, errors.New("kaboom")
		})

	res := r.Call(context.Background(), core.ToolCall{ID: "c1", Name: "fail", Args: map[string]any{"message": "hi"}})
	assert.False(t, res.OK)
	assert.Equal(t, "kaboom", res.Error)
}

func TestRegistry(t *testing.T) {
	r := tool.NewRegistry()
	tool.RegisterFunc(r, "read_log_tail", "tail log", core.RISK_LEVEL_LOW,
		func(ctx context.Context, a echoArgs) (echoOut, error) { return echoOut{Echo: "ok"}, nil })

	got, ok := r.Get("read_log_tail")
	require.True(t, ok)
	assert.Equal(t, "read_log_tail", got.Name())

	_, ok = r.Get("missing")
	assert.False(t, ok)

	schemas := r.List()
	require.Len(t, schemas, 1)
	assert.Equal(t, "read_log_tail", schemas[0].Name)
}

func TestRegistryCall(t *testing.T) {
	r := tool.NewRegistry()
	tool.RegisterFunc(r, "add_todo", "add todo", core.RISK_LEVEL_LOW,
		func(ctx context.Context, a echoArgs) (echoOut, error) { return echoOut{Echo: a.Message}, nil })

	call := core.ToolCall{ID: "t1", Name: "add_todo", Args: map[string]any{"message": "investigate"}}
	res := r.Call(context.Background(), call)
	assert.True(t, res.OK)
	assert.Equal(t, "t1", res.CallID)
}

func TestRegistryCallUnknownTool(t *testing.T) {
	r := tool.NewRegistry()
	res := r.Call(context.Background(), core.ToolCall{ID: "t1", Name: "ghost"})
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "tool not found")
}
