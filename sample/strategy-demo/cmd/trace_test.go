package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryStrategyReachesDone 確保 catalog 裡每一條策略都能在步數上限內
// 收斂到 DONE(不論是 rule 自己的 INSTRUCTION_DONE,還是環境短路),
// 而不是卡在 FSM 裡。這是 demo 的核心不變式。
func TestEveryStrategyReachesDone(t *testing.T) {
	for _, st := range strategies() {
		t.Run(st.id, func(t *testing.T) {
			var buf bytes.Buffer
			traceStrategy(&buf, st)
			out := buf.String()
			assert.Contains(t, out, "DONE", "strategy %q never reached DONE:\n%s", st.id, out)
			assert.NotContains(t, out, "did not reach DONE",
				"strategy %q hit the step ceiling:\n%s", st.id, out)
		})
	}
}

// TestStrategiesHaveUniqueIDs 保護 CLI 的 run <id> 路徑不會撞名。
func TestStrategiesHaveUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, st := range strategies() {
		require.False(t, seen[st.id], "duplicate strategy id %q", st.id)
		seen[st.id] = true
	}
	assert.Len(t, seen, 6, "expected exactly six strategies")
}

// TestRunCmdUnknownIDErrors 確認 run 對未知 id 回報錯誤而非默默成功。
func TestRunCmdUnknownIDErrors(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"run", "nope"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown strategy")
}

// TestReactShortCircuits 針對 ReAct 驗證它是靠環境 end_turn 收尾,
// 而非 rule 自己發 DONE — 對應 runtime 的 end_turn short-circuit。
func TestReactShortCircuits(t *testing.T) {
	var buf bytes.Buffer
	traceStrategy(&buf, reactStrategy())
	out := buf.String()
	assert.Contains(t, out, "end_turn short-circuit")
	assert.Contains(t, out, "call_tool")
	assert.True(t, strings.Contains(out, "read_log_tail"), "expected the tool call in the trace")
}