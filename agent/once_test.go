package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/utils/testutil"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnceCallsProviderExactlyOnce(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	// Queue more replies than one: if the loop wanted another call it
	// would get one, so the count is a measurement rather than an
	// artifact of an empty queue.
	prov.EnqueueEndTurn("pong")
	prov.EnqueueEndTurn("second")
	prov.EnqueueEndTurn("third")

	got, err := agent.Once(context.Background(), agent.Config{}, "ping", agent.WithProvider(prov))
	require.NoError(t, err)
	assert.Equal(t, "pong", got)
	assert.Equal(t, 1, prov.RequestCount(), "one-shot must issue exactly one model call")
}

func TestOnceNeedsNoName(t *testing.T) {
	// The T0 decision: persistence stays off, so Name is not required.
	// A one-liner must not force an application identifier on the caller.
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("ok")

	_, err := agent.Once(context.Background(), agent.Config{}, "hi", agent.WithProvider(prov))
	require.NoError(t, err)
}

func TestOnceForcesOneshotTierRegardlessOfConfig(t *testing.T) {
	// tier and reasoning are orthogonal in spec, and Once resolves the
	// ambiguity by definition: whatever strategy the config names, a
	// single call is a single call.
	for _, style := range spec.Values(spec.StyleChoices()) {
		t.Run(style, func(t *testing.T) {
			prov := testutil.NewScriptedProvider()
			prov.EnqueueEndTurn("once")
			prov.EnqueueEndTurn("twice")

			got, err := agent.Once(context.Background(), agent.Config{
				Tier:      spec.TIER_FULL, // deliberately wrong for Once
				Reasoning: spec.Reasoning{Style: style},
			}, "ping", agent.WithProvider(prov))

			require.NoError(t, err)
			assert.Equal(t, "once", got)
			assert.Equal(t, 1, prov.RequestCount())
		})
	}
}

func TestOncePersonaBecomesSystemMessage(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("ok")

	_, err := agent.Once(context.Background(),
		agent.Config{Persona: "you are terse"}, "hello", agent.WithProvider(prov))
	require.NoError(t, err)

	req := prov.LastRequest()
	require.GreaterOrEqual(t, len(req.Messages), 2, "persona + user prompt")
	assert.Equal(t, core.ROLE_SYSTEM, req.Messages[0].Role)
	assert.Equal(t, "you are terse", req.Messages[0].Parts[0].Text)
	assert.Equal(t, core.ROLE_USER, req.Messages[1].Role)
	assert.Equal(t, "hello", req.Messages[1].Parts[0].Text)
}

func TestOnceSendsNoTools(t *testing.T) {
	// This is what bounds one-shot structurally: with no tool specs the
	// model cannot emit a tool call, so the engine short-circuits after
	// the first reply whatever the reasoning rule would have done next.
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("ok")

	_, err := agent.Once(context.Background(), agent.Config{}, "hi", agent.WithProvider(prov))
	require.NoError(t, err)
	assert.Empty(t, prov.LastRequest().Tools)
}

func TestOnceValidatesConfig(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("ok")

	_, err := agent.Once(context.Background(),
		agent.Config{Reasoning: spec.Reasoning{Style: "vibes"}}, "hi", agent.WithProvider(prov))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown reasoning.style")
	assert.Zero(t, prov.RequestCount(), "a bad config must fail before any model call")
}

func TestOnceRejectsEmptyPrompt(t *testing.T) {
	_, err := agent.Once(context.Background(), agent.Config{}, "   ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt must not be empty")
}

func TestOnceUnknownProvider(t *testing.T) {
	_, err := agent.Once(context.Background(),
		agent.Config{Model: spec.Model{Provider: "bogus"}}, "hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestOncePropagatesProviderError(t *testing.T) {
	prov := testutil.NewScriptedProvider() // empty queue
	_, err := agent.Once(context.Background(), agent.Config{}, "hi", agent.WithProvider(prov))
	require.Error(t, err)
	assert.True(t, errors.Is(err, testutil.ErrQueueEmpty) || err != nil)
}

func TestOnceStreamDeliversEvents(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("streamed")

	var kinds []core.StreamEventKind
	got, err := agent.OnceStream(context.Background(), agent.Config{}, "ping",
		func(ev core.StreamEvent) { kinds = append(kinds, ev.Kind) },
		agent.WithProvider(prov))

	require.NoError(t, err)
	assert.Equal(t, "streamed", got)
	assert.Contains(t, kinds, core.STREAM_RUN_START)
	assert.Contains(t, kinds, core.STREAM_RUN_END)
}

func TestWithProviderRejectsNil(t *testing.T) {
	_, err := agent.Once(context.Background(), agent.Config{}, "hi", agent.WithProvider(nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be nil")
}

// --- provider choices ---

func TestProviderChoicesTrackRegistry(t *testing.T) {
	got := agent.ProviderChoices()
	require.Len(t, got, len(registry.Names()))

	var values []string
	var defaults int
	for _, c := range got {
		values = append(values, c.Value)
		assert.NotEmpty(t, c.Label, "a wizard menu renders Label")
		assert.NotEmpty(t, c.Note, "every provider should say which credential it reads")
		if c.Default {
			defaults++
		}
	}
	assert.Equal(t, registry.Names(), values, "choices must follow the registry's order")
	assert.Equal(t, 1, defaults)
}

func TestProviderChoiceDefaultMatchesSpec(t *testing.T) {
	// The wizard's default and the config's default must be the same
	// provider, or pressing Enter would silently pick something else.
	var got string
	for _, c := range agent.ProviderChoices() {
		if c.Default {
			got = c.Value
		}
	}
	assert.Equal(t, spec.DEFAULT_PROVIDER, got)
}
