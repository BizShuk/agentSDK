package tool_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/tool"
	sdktool "github.com/bizshuk/agentsdk/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProposeFixIsHighRisk(t *testing.T) {
	reg := sdktool.NewRegistry()
	tool.NewProposeFix().Register(reg)
	tl, ok := reg.Get("propose_fix")
	require.True(t, ok)
	assert.Equal(t, core.RISK_LEVEL_HIGH, tl.Spec().Risk,
		"propose_fix must be HIGH risk so the ApprovalGate intercepts it")
}

func TestProposeFixDispatches(t *testing.T) {
	reg := sdktool.NewRegistry()
	tool.NewProposeFix().Register(reg)
	res := reg.Call(context.Background(), core.ToolCall{
		ID:   "c1",
		Name: "propose_fix",
		Args: map[string]any{"title": "restart db", "description": "fix leak"},
	})
	assert.True(t, res.OK)
}

func TestProposeFixRejectsMissingTitle(t *testing.T) {
	reg := sdktool.NewRegistry()
	tool.NewProposeFix().Register(reg)
	res := reg.Call(context.Background(), core.ToolCall{
		ID:   "c1",
		Name: "propose_fix",
		Args: map[string]any{},
	})
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "missing required field: title")
}
