package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/utils/testutil"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingHooks is an inline core.Hooks double.
type recordingHooks struct {
	events []core.HookEventName
	onPre  func(ev core.HookEvent) core.HookDecision
	onPost func(ev core.HookEvent) core.HookDecision
}

func (h *recordingHooks) Fire(_ context.Context, ev core.HookEvent) (core.HookDecision, error) {
	h.events = append(h.events, ev.Name)
	switch {
	case ev.Name == core.HOOK_PRE_TOOL_USE && h.onPre != nil:
		return h.onPre(ev), nil
	case ev.Name == core.HOOK_POST_TOOL_USE && h.onPost != nil:
		return h.onPost(ev), nil
	}
	return core.HookDecision{}, nil
}

type collectSink struct{ kinds []core.StreamEventKind }

func (c *collectSink) OnStreamEvent(ev core.StreamEvent) { c.kinds = append(c.kinds, ev.Kind) }

func newHarnessEngine(prov *testutil.ScriptedProvider, reg *action.Registry) *runtime.Engine {
	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
	loop := runtime.NewEngine(step, prov, reg)
	loop.Emitter = func(core.Instruction) {}
	return loop
}

func reactState(runID string) core.State {
	return core.State{
		RunID:          runID,
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 10},
	}
}

func addCounterTool(reg *action.Registry, calls *int) {
	tool := action.NewTypedTool("add", "add two ints",
		func(_ context.Context, a struct {
			N1 int `json:"n1"`
			N2 int `json:"n2"`
		}) (map[string]int, error) {
			*calls++
			return map[string]int{"sum": a.N1 + a.N2}, nil
		})
	tool.RiskV = core.RISK_LEVEL_LOW
	reg.Register(tool)
}

func messagesText(s core.State) string {
	var sb strings.Builder
	for _, m := range s.Messages {
		for _, p := range m.Parts {
			sb.WriteString(p.Text)
			if p.ToolResult != nil {
				sb.WriteString(p.ToolResult.Error)
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func TestPreToolUseHookBlocksExecution(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCall("c1", "add", map[string]any{"n1": 2, "n2": 3})
	prov.EnqueueEndTurn("stopped")

	reg := action.NewRegistry()
	calls := 0
	addCounterTool(reg, &calls)

	loop := newHarnessEngine(prov, reg)
	hooks := &recordingHooks{onPre: func(core.HookEvent) core.HookDecision {
		return core.HookDecision{Block: true, Reason: "nope"}
	}}
	loop.Hooks = hooks

	final, err := loop.Run(context.Background(), reactState("hook-block"))
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	assert.Equal(t, 0, calls, "blocked tool must not execute")
	assert.Contains(t, messagesText(final), "blocked by hook: nope", "model sees the refusal")
	assert.Contains(t, hooks.events, core.HOOK_SESSION_START)
	assert.Contains(t, hooks.events, core.HOOK_PRE_TOOL_USE)
	assert.Contains(t, hooks.events, core.HOOK_STOP)
}

func TestPostToolUseHookAppendsSystemNote(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCall("c1", "add", map[string]any{"n1": 2, "n2": 3})
	prov.EnqueueEndTurn("done")

	reg := action.NewRegistry()
	calls := 0
	addCounterTool(reg, &calls)

	loop := newHarnessEngine(prov, reg)
	loop.Hooks = &recordingHooks{onPost: func(ev core.HookEvent) core.HookDecision {
		require.NotNil(t, ev.ToolResult)
		return core.HookDecision{SystemNote: "note from post hook"}
	}}

	final, err := loop.Run(context.Background(), reactState("hook-note"))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Contains(t, messagesText(final), "note from post hook")
}

func TestSteeringMessageReachesConversation(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("done")

	loop := newHarnessEngine(prov, action.NewRegistry())
	loop.Steer("also check the logs")

	final, err := loop.Run(context.Background(), reactState("steer"))
	require.NoError(t, err)
	assert.Contains(t, messagesText(final), "also check the logs")
}

func TestFollowUpExtendsRun(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("first answer")
	prov.EnqueueEndTurn("second answer")

	loop := newHarnessEngine(prov, action.NewRegistry())
	loop.FollowUp("one more thing")

	final, err := loop.Run(context.Background(), reactState("follow-up"))
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	assert.Equal(t, 2, prov.RequestCount(), "follow-up drives a second model call")
	assert.Contains(t, messagesText(final), "one more thing")
}

func TestSinkReceivesStreamEvents(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCall("c1", "add", map[string]any{"n1": 1, "n2": 1})
	prov.EnqueueEndTurn("done")

	reg := action.NewRegistry()
	calls := 0
	addCounterTool(reg, &calls)

	loop := newHarnessEngine(prov, reg)
	sink := &collectSink{}
	loop.Sink = sink

	_, err := loop.Run(context.Background(), reactState("sink"))
	require.NoError(t, err)
	assert.Contains(t, sink.kinds, core.STREAM_RUN_START)
	assert.Contains(t, sink.kinds, core.STREAM_TOOL_START)
	assert.Contains(t, sink.kinds, core.STREAM_TOOL_RESULT)
	assert.Contains(t, sink.kinds, core.STREAM_RUN_END)
}
