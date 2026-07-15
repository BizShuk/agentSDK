package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryDemoRunsCleanly 確保五個 demo 都能無錯跑完。
func TestEveryDemoRunsCleanly(t *testing.T) {
	for _, d := range demos() {
		t.Run(d.id, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, d.run(&buf), "demo %q returned an error", d.id)
			assert.NotEmpty(t, buf.String())
		})
	}
}

// TestRetryRecoversThenSurfaces 驗證 retry 重試瞬時錯誤成功、對 fatal 立即上拋。
func TestRetryRecoversThenSurfaces(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, demoRetry(&buf))
	out := buf.String()
	assert.Contains(t, out, "共 3 次嘗試") // 情境 A:第三次才成功
	assert.Contains(t, out, "共 1 次嘗試") // 情境 B:fatal 只試一次
}

// TestBudgetBlocksBeforeBase 驗證超額時 base 不被呼叫。
func TestBudgetBlocksBeforeBase(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, demoBudget(&buf))
	out := buf.String()
	assert.Contains(t, out, "budget exceeded: turn_budget")
	assert.Contains(t, out, "base 是否被呼叫:false")
}

// TestTimeoutTripsOnSlowEffect 驗證慢 effect 觸發 DeadlineExceeded。
func TestTimeoutTripsOnSlowEffect(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, demoTimeout(&buf))
	out := buf.String()
	assert.Contains(t, out, "context deadline exceeded")
	assert.Contains(t, out, "OnTimeout 觸發:true")
}

// TestLoopguardRewritesAtThreshold 驗證第 3 次被改寫成 request_approval。
func TestLoopguardRewritesAtThreshold(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, demoLoopguard(&buf))
	out := buf.String()
	// 第 3 次那一行必須出現 request_approval。
	var line3 string
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "第 3 次") {
			line3 = ln
		}
	}
	require.NotEmpty(t, line3, "找不到第 3 次的輸出行")
	assert.Contains(t, line3, "request_approval")
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