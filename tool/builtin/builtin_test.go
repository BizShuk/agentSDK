package builtin_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bizshuk/agentsdk/tool"
	"github.com/bizshuk/agentsdk/tool/builtin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterDefaults_AllToolsRegistered(t *testing.T) {
	reg := tool.NewRegistry()
	policy := tool.DefaultPolicy()

	err := builtin.RegisterDefaults(reg, builtin.Options{
		Policy: policy,
	})
	require.NoError(t, err)

	names := builtin.BuiltinNames()
	assert.Len(t, names, 6)

	for _, name := range names {
		registered, ok := reg.Get(name)
		assert.True(t, ok, "tool %q should be registered", name)
		require.NotNil(t, registered.Spec().Parameters)

		res, callErr := registered.Call(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, callErr)
		assert.False(t, res.OK)
		assert.Equal(t, name, res.Name)
		assert.Contains(t, res.Error, "missing required field")
	}
}

func TestRegisterDefaults_MissingPolicy_Errors(t *testing.T) {
	reg := tool.NewRegistry()
	// Nil policy should cause RegisterDefaults to error on write/edit/bash
	err := builtin.RegisterDefaults(reg, builtin.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write:")
	assert.Contains(t, err.Error(), "edit:")
	assert.Contains(t, err.Error(), "bash:")
}

func TestRegister_Allowlist(t *testing.T) {
	reg := tool.NewRegistry()
	err := builtin.Register(reg, []string{builtin.NAME_READ, builtin.NAME_GREP}, builtin.Options{
		Policy: tool.DefaultPolicy(),
	})
	require.NoError(t, err)

	_, hasRead := reg.Get(builtin.NAME_READ)
	_, hasGrep := reg.Get(builtin.NAME_GREP)
	_, hasWrite := reg.Get(builtin.NAME_WRITE)
	assert.True(t, hasRead)
	assert.True(t, hasGrep)
	assert.False(t, hasWrite)
	assert.Len(t, reg.List(), 2)
}

func TestRegister_UnknownNameFailsBeforeMutation(t *testing.T) {
	reg := tool.NewRegistry()
	err := builtin.Register(reg, []string{builtin.NAME_READ, "unknown"}, builtin.Options{
		Policy: tool.DefaultPolicy(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown built-in tool "unknown"`)
	assert.Empty(t, reg.List())
}

func TestMustPolicy_PanicsOnNil(t *testing.T) {
	assert.Panics(t, func() {
		builtin.MustPolicy(nil)
	})

	pol := tool.DefaultPolicy()
	assert.Equal(t, pol, builtin.MustPolicy(pol))
}
