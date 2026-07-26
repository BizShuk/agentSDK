package builtin

import (
	"context"
	"testing"

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
	b, err := NewBash(testPolicy(t.TempDir()), t.TempDir(),
		WithBashExecutor(&stubExecutor{stdout: "hello\n", exitCode: 0}))
	require.NoError(t, err)

	out, herr := b.Handle(context.Background(), BashArgs{Command: "echo hello"})
	require.NoError(t, herr)
	assert.Contains(t, out.Stdout, "hello")
	assert.Equal(t, 0, out.ExitCode)
}

func TestBash_NonZeroExit(t *testing.T) {
	b, err := NewBash(testPolicy(t.TempDir()), t.TempDir(),
		WithBashExecutor(&stubExecutor{stderr: "error!\n", exitCode: 1}))
	require.NoError(t, err)

	out, herr := b.Handle(context.Background(), BashArgs{Command: "false"})
	require.NoError(t, herr)
	assert.Equal(t, 1, out.ExitCode)
	assert.Contains(t, out.Stderr, "error!")
}

func TestBash_CommandDenied(t *testing.T) {
	b, err := NewBash(testPolicy(t.TempDir()), t.TempDir())
	require.NoError(t, err)

	_, herr := b.Handle(context.Background(), BashArgs{Command: "rm -rf /"})
	require.Error(t, herr)
	assert.Contains(t, herr.Error(), "sandbox denied")
}

func TestBash_NilPolicy_Error(t *testing.T) {
	_, err := NewBash(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox Policy")
}

func TestBash_RiskLevel(t *testing.T) {
	b, err := NewBash(testPolicy(t.TempDir()), t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, sdkcore.RISK_LEVEL_HIGH, b.Risk())
}

func TestBash_StubTimeout(t *testing.T) {
	exec := &stubExecutor{err: context.DeadlineExceeded}
	b, err := NewBash(testPolicy(t.TempDir()), t.TempDir(), WithBashExecutor(exec))
	require.NoError(t, err)

	_, herr := b.Handle(context.Background(), BashArgs{Command: "sleep 10", TimeoutMs: 1})
	require.Error(t, herr)
	assert.Contains(t, herr.Error(), "bash:")
}

func TestBash_CustomCwd(t *testing.T) {
	dir := t.TempDir()
	b, err := NewBash(testPolicy(dir), dir,
		WithBashExecutor(&stubExecutor{stdout: dir + "\n", exitCode: 0}))
	require.NoError(t, err)

	out, herr := b.Handle(context.Background(), BashArgs{Command: "pwd", Cwd: dir})
	require.NoError(t, herr)
	assert.Contains(t, out.Stdout, dir)
}
