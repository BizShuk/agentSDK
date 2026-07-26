package tool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProposeFixIsHighRisk(t *testing.T) {
	pf := tool.NewProposeFix()
	assert.Equal(t, core.RISK_LEVEL_HIGH, pf.Risk(),
		"propose_fix must be HIGH risk so the ApprovalGate intercepts it")
}

func TestProposeFixDispatches(t *testing.T) {
	pf := tool.NewProposeFix()
	raw, err := json.Marshal(struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}{
		Title: "restart db", Description: "fix the connection leak",
	})
	require.NoError(t, err)
	res, err := pf.Call(context.Background(), raw)
	require.NoError(t, err)
	assert.True(t, res.OK)
}

func TestProposeFixRejectsMissingTitle(t *testing.T) {
	pf := tool.NewProposeFix()
	res, err := pf.Call(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	// Schema validation rejects because title is required.
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "missing required field: title")
}
