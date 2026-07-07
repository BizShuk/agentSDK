package tool

import (
	"context"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubExecutor returns controlled output.
type stubExecutor struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (s *stubExecutor) Run(_ context.Context, _ string, _ []string, _ string) ([]byte, []byte, int, error) {
	return []byte(s.stdout), []byte(s.stderr), s.exitCode, s.err
}

func TestBash_EchoCommand(t *testing.T) {
	b, err := NewBash(BashOptions{
		DefaultTimeout: 5 * time.Second,
		Executor:       &stubExecutor{stdout: "hello\n", exitCode: 0},
	}, action.DefaultPolicy(), t.TempDir())
	require.NoError(t, err)

	res, cerr := b.Call(context.Background(), mustMarshal(BashArgs{Command: "echo hello"}))
	require.NoError(t, cerr)
	assert.True(t, res.OK)

	out := unmarshalOutput[BashOutput](t, res)
	assert.Contains(t, out.Stdout, "hello")
	assert.Equal(t, 0, out.ExitCode)
}

func TestBash_NonZeroExit(t *testing.T) {
	b, err := NewBash(BashOptions{
		DefaultTimeout: 5 * time.Second,
		Executor:       &stubExecutor{stderr: "error!\n", exitCode: 1},
	}, action.DefaultPolicy(), t.TempDir())
	require.NoError(t, err)

	res, cerr := b.Call(context.Background(), mustMarshal(BashArgs{Command: "false"}))
	require.NoError(t, cerr)
	assert.True(t, res.OK)

	out := unmarshalOutput[BashOutput](t, res)
	assert.Equal(t, 1, out.ExitCode)
	assert.Contains(t, out.Stderr, "error!")
}

func TestBash_CommandDenied(t *testing.T) {
	b, err := NewBash(BashOptions{
		DefaultTimeout: 5 * time.Second,
	}, action.DefaultPolicy(), t.TempDir())
	require.NoError(t, err)

	res, cerr := b.Call(context.Background(), mustMarshal(BashArgs{Command: "rm -rf /"}))
	require.NoError(t, cerr)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "sandbox denied")
}

func TestBash_NilPolicy_Error(t *testing.T) {
	_, err := NewBash(BashOptions{}, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox Policy")
}

func TestBash_RiskLevel(t *testing.T) {
	b, err := NewBash(BashOptions{
		DefaultTimeout: 5 * time.Second,
	}, action.DefaultPolicy(), t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, sdkcore.RISK_LEVEL_HIGH, b.Risk())
}

func TestBash_StubTimeout(t *testing.T) {
	exec := &stubExecutor{err: context.DeadlineExceeded}
	b, err := NewBash(BashOptions{
		DefaultTimeout: 1 * time.Millisecond,
		Executor:       exec,
	}, action.DefaultPolicy(), t.TempDir())
	require.NoError(t, err)

	res, cerr := b.Call(context.Background(), mustMarshal(BashArgs{Command: "sleep 10", TimeoutMs: 1}))
	require.NoError(t, cerr)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "bash:")
}

func TestBash_CustomCwd(t *testing.T) {
	dir := t.TempDir()
	b, err := NewBash(BashOptions{
		DefaultTimeout: 5 * time.Second,
		Executor:       &stubExecutor{stdout: dir + "\n", exitCode: 0},
	}, action.DefaultPolicy(), dir)
	require.NoError(t, err)

	res, cerr := b.Call(context.Background(), mustMarshal(BashArgs{Command: "pwd", Cwd: dir}))
	require.NoError(t, cerr)
	assert.True(t, res.OK)
}
