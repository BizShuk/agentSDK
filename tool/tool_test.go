package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rawArgs struct {
	Message string `json:"message"`
}

type rawOutput struct {
	Echo string `json:"echo"`
}

func TestCallWithRawMessage(t *testing.T) {
	res, err := tool.CallWithRawMessage(
		context.Background(),
		"echo",
		json.RawMessage(`{"message":"hello"}`),
		func(_ context.Context, args rawArgs) (rawOutput, error) {
			return rawOutput{Echo: args.Message}, nil
		},
	)
	require.NoError(t, err)
	require.True(t, res.OK)

	var output rawOutput
	require.NoError(t, json.Unmarshal(res.Output.(json.RawMessage), &output))
	assert.Equal(t, "hello", output.Echo)
}

func TestCallWithRawMessageRejectsInvalidArgs(t *testing.T) {
	called := false
	res, err := tool.CallWithRawMessage(
		context.Background(),
		"echo",
		json.RawMessage(`{}`),
		func(_ context.Context, args rawArgs) (rawOutput, error) {
			called = true
			return rawOutput{Echo: args.Message}, nil
		},
	)
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.False(t, called)
	assert.Contains(t, res.Error, "missing required field: message")
}

func TestCallWithRawMessageEncodesCallError(t *testing.T) {
	res, err := tool.CallWithRawMessage(
		context.Background(),
		"echo",
		json.RawMessage(`{"message":"hello"}`),
		func(context.Context, rawArgs) (rawOutput, error) {
			return rawOutput{}, errors.New("call failed")
		},
	)
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Equal(t, "call failed", res.Error)
}

func TestRegistryGetReturnsExecutableCoreTool(t *testing.T) {
	reg := tool.NewRegistry()
	tool.RegisterFunc(
		reg,
		"echo",
		"echo back the message",
		core.RISK_LEVEL_LOW,
		func(_ context.Context, args rawArgs) (rawOutput, error) {
			return rawOutput{Echo: args.Message}, nil
		},
	)

	registered, ok := reg.Get("echo")
	require.True(t, ok)

	res, err := registered.Call(
		context.Background(),
		json.RawMessage(`{"message":"from core.Tool"}`),
	)
	require.NoError(t, err)
	assert.True(t, res.OK)
}
