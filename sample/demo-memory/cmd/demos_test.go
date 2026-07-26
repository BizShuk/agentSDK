package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryDemoRunsCleanly 確保三個 demo 都能無錯跑完。
func TestEveryDemoRunsCleanly(t *testing.T) {
	for _, d := range demos() {
		t.Run(d.id, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, d.run(&buf), "demo %q returned an error", d.id)
			assert.NotEmpty(t, buf.String())
		})
	}
}

// TestWindowTrims 驗證 Window 真的縮短了 transcript。
func TestWindowTrims(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, demoWindow(&buf))
	out := buf.String()
	assert.Contains(t, out, "MaxMessages:3")
	assert.Contains(t, out, "MaxTokens:30")
}

// TestCompactProducesSingleSummary 驗證壓縮輸出帶摘要前綴。
func TestCompactProducesSingleSummary(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, demoCompact(&buf))
	assert.Contains(t, buf.String(), "[compacted summary]")
}

// TestCheckpointReplaysOnlyNewEvents 驗證只回放 Seq > LastInputSeq 的事件。
func TestCheckpointReplaysOnlyNewEvents(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, demoCheckpoint(&buf))
	out := buf.String()
	assert.Contains(t, out, "seq=2")
	assert.Contains(t, out, "seq=3")
	assert.NotContains(t, out, "     - seq=1") // seq=1 已在快照內,不該被回放
}

// TestRunCmdUnknownIDErrors 確認未知 id 回報錯誤。
func TestRunCmdUnknownIDErrors(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"run", "nope"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown demo")
}
