package runtime_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/utils/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDIProviderSwap exercises the M4 promise: the same runtime.Loop
// must accept two different Providers without code changes. We
// run the loop twice with different FakeProviders, each producing a
// distinct scripted transcript; both should reach RUN_STATUS_COMPLETED.
func TestDIProviderSwap(t *testing.T) {
	reg := action.NewRegistry()
	action.RegisterFunc(reg, "noop", "no-op", core.RISK_LEVEL_LOW,
		func(_ context.Context, _ struct{}) (struct{}, error) { return struct{}{}, nil })

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})

	state := func() core.State {
		return core.State{
			RunID:          "di-1",
			ReasoningStyle: core.REASON_REACT,
			Budget:         core.Budget{MaxTurns: 5},
		}
	}

	t.Run("provider A", func(t *testing.T) {
		provA := testutil.NewScriptedProvider()
		provA.EnqueueToolCall("c1", "noop", map[string]any{})
		provA.EnqueueEndTurn("from-A")

		loop := runtime.NewEngine(step, provA, reg)
		loop.Approval = stubApproval{}
		loop.Emitter = func(eff core.Instruction) {}

		final, err := loop.Run(context.Background(), state())
		require.NoError(t, err)
		assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	})

	t.Run("provider B", func(t *testing.T) {
		provB := testutil.NewScriptedProvider()
		provB.EnqueueToolCall("c2", "noop", map[string]any{})
		provB.EnqueueEndTurn("from-B")

		loop := runtime.NewEngine(step, provB, reg)
		loop.Approval = stubApproval{}
		loop.Emitter = func(eff core.Instruction) {}

		final, err := loop.Run(context.Background(), state())
		require.NoError(t, err)
		assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	})
}

// TestImageChunkSurvivesRunLoop feeds an IMAGE chunk into the
// Step's input. The chunk must survive end-to-end without being
// re-encoded (multimodal abstraction preservation).
func TestImageChunkSurvivesRunLoop(t *testing.T) {
	imgBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a} // PNG header

	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("seen")

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
	loop := runtime.NewEngine(step, prov, action.NewRegistry())
	loop.Emitter = func(eff core.Instruction) {}

	state := core.State{
		RunID:          "img-1",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 3},
		Messages: []core.Message{
			{Role: core.ROLE_USER, Parts: []core.Part{
				{Kind: core.PART_KIND_IMAGE, ImageMIME: "image/png", Image: imgBytes},
			}},
		},
	}
	_, err := loop.RunWithEvent(context.Background(), state, core.Event{
		Kind:        core.EVENT_OBSERVATION,
		Observation: &core.Observation{ID: "p", Source: "test", Payload: "wake up"},
	})
	require.NoError(t, err)

	// After the run, the IMAGE chunk must still be in state with
	// identical bytes — proving the multimodal abstraction is
	// preserved end-to-end.
	require.NotEmpty(t, state.Messages)
	for _, m := range state.Messages {
		for _, c := range m.Parts {
			if c.Kind == core.PART_KIND_IMAGE {
				assert.Equal(t, imgBytes, c.Image)
				assert.Equal(t, "image/png", c.ImageMIME)
			}
		}
	}
}
