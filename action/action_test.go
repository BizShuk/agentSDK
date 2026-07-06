package action_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type echoArgs struct {
	Message string `json:"message"`
}
type echoOut struct {
	Echo string `json:"echo"`
}

func TestTypedToolCall(t *testing.T) {
	tool := action.NewTypedTool("echo", "echo back the message",
		func(ctx context.Context, a echoArgs) (echoOut, error) {
			return echoOut{Echo: a.Message}, nil
		})

	raw := json.RawMessage(`{"message":"hi"}`)
	res, err := tool.Call(context.Background(), raw)
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.Equal(t, "echo", res.Name)
}

func TestTypedToolError(t *testing.T) {
	tool := action.NewTypedTool("fail", "always fails",
		func(ctx context.Context, a echoArgs) (echoOut, error) {
			return echoOut{}, errors.New("kaboom")
		})

	res, err := tool.Call(context.Background(), json.RawMessage(`{"message":"hi"}`))
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Equal(t, "kaboom", res.Error)
}

func TestTypedToolBadArgs(t *testing.T) {
	tool := action.NewTypedTool("echo", "echo back",
		func(ctx context.Context, a echoArgs) (echoOut, error) {
			return echoOut{Echo: a.Message}, nil
		})

	res, err := tool.Call(context.Background(), json.RawMessage(`not json`))
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "schema validation failed")
}

func TestRegistry(t *testing.T) {
	r := action.NewRegistry()
	tool := action.NewTypedTool("read_log_tail", "tail log",
		func(ctx context.Context, a echoArgs) (echoOut, error) { return echoOut{Echo: "ok"}, nil })

	r.Register(tool)

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
	r := action.NewRegistry()
	tool := action.NewTypedTool("add_todo", "add todo",
		func(ctx context.Context, a echoArgs) (echoOut, error) { return echoOut{Echo: a.Message}, nil })
	r.Register(tool)

	call := core.ToolCall{ID: "t1", Name: "add_todo", Args: map[string]any{"message": "investigate"}}
	res := r.Call(context.Background(), call)
	assert.True(t, res.OK)
	assert.Equal(t, "t1", res.CallID)
}

func TestRegistryCallUnknownTool(t *testing.T) {
	r := action.NewRegistry()
	res := r.Call(context.Background(), core.ToolCall{ID: "t1", Name: "ghost"})
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "tool not found")
}