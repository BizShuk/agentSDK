package hook

import (
	"context"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchTool(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		tool    string
		want    bool
	}{
		{"empty matches all", "", "Bash", true},
		{"star matches all", "*", "", true},
		{"exact", "Bash", "Bash", true},
		{"exact miss", "Bash", "Edit", false},
		{"alternation hit", "Edit|Write", "Write", true},
		{"alternation miss", "Edit|Write", "Bash", false},
		{"glob prefix", "mcp__*", "mcp__slack__send", true},
		{"glob prefix miss", "mcp__*", "Bash", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MatchTool(tt.pattern, tt.tool))
		})
	}
}

func TestRunnerFireMergesDecisions(t *testing.T) {
	r := NewRunner(
		Rule{Event: core.HOOK_PRE_TOOL_USE, Match: "Bash", Handlers: []Handler{
			Func(func(_ context.Context, _ core.HookEvent) (core.HookDecision, error) {
				return core.HookDecision{SystemNote: "note-a"}, nil
			}),
			Func(func(_ context.Context, _ core.HookEvent) (core.HookDecision, error) {
				return core.HookDecision{Block: true, Reason: "dangerous"}, nil
			}),
		}},
		Rule{Event: core.HOOK_PRE_TOOL_USE, Match: "Edit", Handlers: []Handler{
			Func(func(_ context.Context, _ core.HookEvent) (core.HookDecision, error) {
				return core.HookDecision{Block: true, Reason: "should not fire"}, nil
			}),
		}},
	)

	dec, err := r.Fire(context.Background(), core.HookEvent{
		Name: core.HOOK_PRE_TOOL_USE, ToolName: "Bash",
	})
	require.NoError(t, err)
	assert.True(t, dec.Block)
	assert.Equal(t, "dangerous", dec.Reason)
	assert.Equal(t, "note-a", dec.SystemNote)
}

func TestRunnerFireNoMatchIsZero(t *testing.T) {
	r := NewRunner(Rule{Event: core.HOOK_STOP, Handlers: []Handler{
		Func(func(_ context.Context, _ core.HookEvent) (core.HookDecision, error) {
			return core.HookDecision{Block: true}, nil
		}),
	}})
	dec, err := r.Fire(context.Background(), core.HookEvent{Name: core.HOOK_PRE_TOOL_USE})
	require.NoError(t, err)
	assert.False(t, dec.Block)
}

func TestCommandHandler(t *testing.T) {
	ctx := context.Background()
	ev := core.HookEvent{Name: core.HOOK_PRE_TOOL_USE, ToolName: "Bash"}

	t.Run("exit 2 blocks with stderr reason", func(t *testing.T) {
		c := Command{Path: "/bin/sh", Args: []string{"-c", `echo "no way" >&2; exit 2`}}
		dec, err := c.Handle(ctx, ev)
		require.NoError(t, err)
		assert.True(t, dec.Block)
		assert.Equal(t, "no way", dec.Reason)
	})

	t.Run("exit 0 with JSON decision", func(t *testing.T) {
		c := Command{Path: "/bin/sh", Args: []string{"-c", `echo '{"system_note":"from-cmd"}'`}}
		dec, err := c.Handle(ctx, ev)
		require.NoError(t, err)
		assert.False(t, dec.Block)
		assert.Equal(t, "from-cmd", dec.SystemNote)
	})

	t.Run("exit 0 no output proceeds", func(t *testing.T) {
		c := Command{Path: "/bin/sh", Args: []string{"-c", "cat > /dev/null"}}
		dec, err := c.Handle(ctx, ev)
		require.NoError(t, err)
		assert.Equal(t, core.HookDecision{}, dec)
	})

	t.Run("other exit codes are errors", func(t *testing.T) {
		c := Command{Path: "/bin/sh", Args: []string{"-c", "exit 1"}}
		_, err := c.Handle(ctx, ev)
		require.Error(t, err)
	})

	t.Run("timeout is an error", func(t *testing.T) {
		c := Command{Path: "/bin/sh", Args: []string{"-c", "sleep 5"}, Timeout: 50 * time.Millisecond}
		_, err := c.Handle(ctx, ev)
		require.Error(t, err)
	})
}
