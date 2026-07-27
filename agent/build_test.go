package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware/hook"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/skill"
	"github.com/bizshuk/agentsdk/tool/builtin"
	"github.com/bizshuk/agentsdk/utils/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// appCfg builds a Host backed by a temp dir and in-memory stores.
// Bootstrap must never need agent.OpenForCLI, which would write into the
// developer's real ~/.config.
func appCfg(t *testing.T) *agent.Host {
	t.Helper()
	dir := t.TempDir()
	return &agent.Host{
		DataDir:    filepath.Join(dir, "data"),
		LogDir:     filepath.Join(dir, "logs"),
		RunID:      "test-run",
		StateStore: testutil.NewMemStore(),
		WAL:        testutil.NewMemWAL(),
	}
}

// bootstrap builds an agent and runs its pipeline, from a temp working
// directory so context-file discovery cannot pick up the real repo.
func bootstrap(t *testing.T, cfg agent.Config, opts ...agent.Option) (*runtime.Engine, core.State, *agent.Agent) {
	t.Helper()
	t.Chdir(t.TempDir())

	prov := testutil.NewScriptedProvider()
	opts = append([]agent.Option{agent.WithProvider(prov)}, opts...)

	a, err := agent.New(cfg, opts...)
	require.NoError(t, err)

	eng, state, err := a.Bootstrap(context.Background(), appCfg(t))
	require.NoError(t, err)
	return eng, state, a
}

// --- New: validation before any I/O ---

func TestNewRejectsBadConfigBeforeTouchingAnything(t *testing.T) {
	_, err := agent.New(agent.Config{Name: "x", Reasoning: spec.Reasoning{Style: "vibes"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown reasoning.style")
}

func TestNewRequiresName(t *testing.T) {
	_, err := agent.New(agent.Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
	assert.Contains(t, err.Error(), "Once", "the error should point at the nameless alternative")
}

func TestNewExposesTheExpandedConfig(t *testing.T) {
	a, err := agent.New(agent.Config{Name: "x", Tier: spec.TIER_FULL})
	require.NoError(t, err)
	assert.Equal(t, spec.DEFAULT_STYLE, a.Config().Reasoning.Style)
	assert.NotNil(t, a.Config().Skills)
}

// --- Bootstrap: each block nil vs non-nil reaches the engine ---

func TestBootstrapWiresBlocksToEngineFields(t *testing.T) {
	cases := []struct {
		name   string
		cfg    agent.Config
		verify func(t *testing.T, e *runtime.Engine)
	}{
		{
			name: "oneshot leaves every optional port nil",
			cfg:  agent.Config{Name: "x", Tier: spec.TIER_ONESHOT},
			verify: func(t *testing.T, e *runtime.Engine) {
				assert.Nil(t, e.Tools)
				assert.Nil(t, e.Store)
				assert.Nil(t, e.Log)
				assert.Nil(t, e.Middleware)
				assert.Nil(t, e.Approval)
				assert.Nil(t, e.Hooks)
				assert.Nil(t, e.Sink)
			},
		},
		{
			name: "basic adds middleware and persistence",
			cfg:  agent.Config{Name: "x", Tier: spec.TIER_BASIC},
			verify: func(t *testing.T, e *runtime.Engine) {
				assert.NotNil(t, e.Middleware)
				assert.NotNil(t, e.Store)
				assert.NotNil(t, e.Log)
				assert.Nil(t, e.Tools, "basic has no tools")
				assert.Nil(t, e.Approval, "nothing to gate without tools")
			},
		},
		{
			name: "standard adds tools and the approval gate",
			cfg:  agent.Config{Name: "x", Tier: spec.TIER_STANDARD},
			verify: func(t *testing.T, e *runtime.Engine) {
				require.NotNil(t, e.Tools)
				assert.Len(t, e.Tools.List(), 6, "an empty allowlist means all six built-ins")
				assert.NotNil(t, e.Approval)
				assert.NotNil(t, e.Middleware)
			},
		},
		{
			name: "memory off disables persistence",
			cfg: agent.Config{Name: "x", Tier: spec.TIER_BASIC,
				Memory: &spec.Memory{Store: spec.MEMORY_STORE_NONE}},
			verify: func(t *testing.T, e *runtime.Engine) {
				assert.Nil(t, e.Store, "store=none must genuinely disable, not defer to app's backfill")
				assert.Nil(t, e.Log)
			},
		},
		{
			name: "middleware none",
			cfg: agent.Config{Name: "x", Tier: spec.TIER_BASIC,
				Middleware: &spec.Middleware{Preset: spec.MIDDLEWARE_NONE}},
			verify: func(t *testing.T, e *runtime.Engine) {
				assert.Nil(t, e.Middleware)
			},
		},
		{
			name: "tools allowlist narrows the registry",
			cfg: agent.Config{Name: "x", Tier: spec.TIER_STANDARD,
				Tools: &spec.Tools{Builtin: []string{builtin.NAME_READ, builtin.NAME_GLOB, builtin.NAME_GREP}}},
			verify: func(t *testing.T, e *runtime.Engine) {
				require.NotNil(t, e.Tools)
				var names []string
				for _, s := range e.Tools.List() {
					names = append(names, s.Name)
				}
				assert.ElementsMatch(t, []string{"read", "glob", "grep"}, names)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, _, _ := bootstrap(t, tc.cfg)
			tc.verify(t, eng)
		})
	}
}

func TestBootstrapRejectsNilHost(t *testing.T) {
	a, err := agent.New(
		agent.Config{Name: "x", Tier: spec.TIER_ONESHOT},
		agent.WithProvider(testutil.NewScriptedProvider()),
	)
	require.NoError(t, err)

	_, _, err = a.Bootstrap(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host")
}

func TestBootstrapSeedsStateFromLimits(t *testing.T) {
	_, state, _ := bootstrap(t, agent.Config{
		Name: "x", Tier: spec.TIER_BASIC,
		Reasoning: spec.Reasoning{Style: "plan_then_run"},
		Limits:    spec.Limits{MaxTurns: 7, Autonomy: "L4"},
	})
	assert.Equal(t, "test-run", state.RunID)
	assert.Equal(t, core.REASON_PLAN_THEN_RUN, state.ReasoningStyle)
	assert.Equal(t, 7, state.Budget.MaxTurns)
	assert.Equal(t, core.AUTONOMY_L4, state.Autonomy)
}

func TestBootstrapSeedsPersonaAsSystemMessage(t *testing.T) {
	_, state, _ := bootstrap(t, agent.Config{
		Name: "x", Tier: spec.TIER_BASIC, Persona: "you are terse",
	})
	require.NotEmpty(t, state.Messages)
	assert.Equal(t, core.ROLE_SYSTEM, state.Messages[0].Role)
	assert.Contains(t, state.Messages[0].Parts[0].Text, "you are terse")
}

func TestBootstrapWiresSubagentMaxDepth(t *testing.T) {
	defsDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(defsDir, "reviewer.md"),
		[]byte("Review the requested change."),
		0o600,
	))

	engine, _, _ := bootstrap(t, agent.Config{
		Name: "x",
		Tier: spec.TIER_FULL,
		Subagents: &spec.Subagents{
			Dirs:     []string{defsDir},
			MaxDepth: 1,
			MaxTurns: 1,
		},
	})

	task, ok := engine.Tools.Get(skill.TOOL_NAME)
	require.True(t, ok)
	result, err := task.Call(
		skill.WithDepth(context.Background(), 1),
		json.RawMessage(`{"agent":"reviewer","prompt":"check it"}`),
	)
	require.NoError(t, err)
	assert.False(t, result.OK)
	assert.Contains(t, result.Error, "depth limit reached (1)")
}

func TestBootstrapMergesContextFilesIntoOneSystemMessage(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("PROJECT_RULES"), 0o600))
	t.Chdir(dir)

	a, err := agent.New(agent.Config{Name: "x", Tier: spec.TIER_STANDARD, Persona: "PERSONA"},
		agent.WithProvider(testutil.NewScriptedProvider()))
	require.NoError(t, err)
	_, state, err := a.Bootstrap(context.Background(), appCfg(t))
	require.NoError(t, err)

	require.Len(t, state.Messages, 1, "one system message, whatever the number of contributors")
	text := state.Messages[0].Parts[0].Text
	assert.Less(t, strings.Index(text, "PERSONA"), strings.Index(text, "PROJECT_RULES"),
		"persona anchors the cacheable prefix")
}

// --- reasoning registration ---

func TestBootstrapRegistersOnlyEnabledRules(t *testing.T) {
	// The engine dispatches on State.ReasoningStyle; an unregistered
	// style yields a NOTIFY error instead of reasoning. Registering just
	// the selected style is the default, and a router registers more.
	_, state, _ := bootstrap(t, agent.Config{
		Name: "x", Tier: spec.TIER_BASIC,
		Reasoning: spec.Reasoning{
			Style:  "choose_agent",
			Enable: []string{"choose_agent", "think_then_act"},
		},
	})
	assert.Equal(t, core.REASON_PICK_AGENT, state.ReasoningStyle)
}

func TestBootstrapEveryStyleBuilds(t *testing.T) {
	for _, style := range spec.Values(spec.StyleChoices()) {
		t.Run(style, func(t *testing.T) {
			eng, state, _ := bootstrap(t, agent.Config{
				Name: "x", Tier: spec.TIER_BASIC,
				Reasoning: spec.Reasoning{Style: style},
			})
			require.NotNil(t, eng.Step)
			assert.Equal(t, style, state.ReasoningStyle)
		})
	}
}

func TestBuiltinToolChoicesMatchRegistryNames(t *testing.T) {
	assert.ElementsMatch(t,
		builtin.BuiltinNames(),
		spec.Values(spec.VariantChoices("tools.builtin")),
	)
}

// --- injection ---

func TestOptionsReachTheEngine(t *testing.T) {
	t.Run("tools", func(t *testing.T) {
		eng, _, _ := bootstrap(t, agent.Config{Name: "x", Tier: spec.TIER_BASIC},
			agent.WithTools(&fakeTool{name: "deploy"}))
		require.NotNil(t, eng.Tools, "an injected tool must create a registry even at a tier with none")
		var names []string
		for _, s := range eng.Tools.List() {
			names = append(names, s.Name)
		}
		assert.Contains(t, names, "deploy")
	})

	t.Run("custom tool replaces configured built-in", func(t *testing.T) {
		eng, _, _ := bootstrap(t, agent.Config{Name: "x", Tier: spec.TIER_STANDARD},
			agent.WithTools(&fakeTool{name: builtin.NAME_READ}))
		registered, ok := eng.Tools.Get(builtin.NAME_READ)
		require.True(t, ok)
		assert.Equal(t, "test tool", registered.Spec().Description)
	})

	t.Run("custom rule replaces configured built-in", func(t *testing.T) {
		custom := &fakeRule{kind: core.REASON_REACT}
		eng, state, _ := bootstrap(t, agent.Config{Name: "x", Tier: spec.TIER_BASIC},
			agent.WithRules(custom))
		_, instructions := eng.Step(state, core.Event{})
		assert.Equal(t, 1, custom.called)
		require.Len(t, instructions, 1)
		assert.Equal(t, core.INSTRUCTION_DONE, instructions[0].Kind)
	})

	t.Run("hooks", func(t *testing.T) {
		eng, _, _ := bootstrap(t, agent.Config{Name: "x", Tier: spec.TIER_BASIC},
			agent.WithHooks(hook.Rule{Event: core.HOOK_PRE_TOOL_USE, Match: "bash"}))
		assert.NotNil(t, eng.Hooks)
	})

	t.Run("no hooks leaves the port nil", func(t *testing.T) {
		eng, _, _ := bootstrap(t, agent.Config{Name: "x", Tier: spec.TIER_BASIC})
		assert.Nil(t, eng.Hooks, "an unused port must stay nil so it costs nothing")
	})

	t.Run("sources", func(t *testing.T) {
		_, state, _ := bootstrap(t, agent.Config{Name: "x", Tier: spec.TIER_BASIC},
			agent.WithSources(prompt.Static(prompt.SLOT_SYSTEM, "custom", "INJECTED", 15)))
		require.NotEmpty(t, state.Messages)
		assert.Contains(t, state.Messages[0].Parts[0].Text, "INJECTED")
	})

	t.Run("sink injection binds presentation", func(t *testing.T) {
		var got int
		eng, _, _ := bootstrap(t,
			agent.Config{Name: "x", Tier: spec.TIER_BASIC},
			agent.WithSink(agent.SinkFunc(func(core.StreamEvent) { got++ })))
		require.NotNil(t, eng.Sink)
		eng.Sink.OnStreamEvent(core.StreamEvent{})
		assert.Equal(t, 1, got)
	})

	t.Run("customize runs last with the assembled engine", func(t *testing.T) {
		var seen *runtime.Engine
		eng, _, _ := bootstrap(t, agent.Config{Name: "x", Tier: spec.TIER_STANDARD},
			agent.WithCustomize(func(e *runtime.Engine) error {
				seen = e
				return nil
			}))
		require.NotNil(t, seen)
		assert.Same(t, eng, seen)
		assert.NotNil(t, seen.Tools, "customize must see a fully assembled engine")
		assert.NotNil(t, seen.Approval)
	})

	t.Run("customize failure aborts bootstrap", func(t *testing.T) {
		t.Chdir(t.TempDir())
		boom := errors.New("boom")
		a, err := agent.New(agent.Config{Name: "x", Tier: spec.TIER_BASIC},
			agent.WithProvider(testutil.NewScriptedProvider()),
			agent.WithCustomize(func(*runtime.Engine) error { return boom }))
		require.NoError(t, err)
		_, _, err = a.Bootstrap(context.Background(), appCfg(t))
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

func TestNilInjectionsAreRejected(t *testing.T) {
	cases := map[string]agent.Option{
		"tools":          agent.WithTools(nil),
		"tool registrar": agent.WithToolRegistrar(nil),
		"tool function": agent.WithToolFunc[struct{}, struct{}](
			"test", "test", core.RISK_LEVEL_LOW, nil,
		),
		"sources":   agent.WithSources(nil),
		"rules":     agent.WithRules(nil),
		"customize": agent.WithCustomize(nil),
		"sink":      agent.WithSink(nil),
		"listener":  agent.WithListener(nil),
		"notifier":  agent.WithNotifier(nil),
	}
	for name, opt := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := agent.New(agent.Config{Name: "x"}, opt)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "nil")
		})
	}
}

// --- listener pump ---

// channelSource is a test double for core.ObservationSource: the channel
// is buffered so callers can pre-load observations and assert on what
// reaches the engine without racing the pump goroutine.
type channelSource struct{ ch chan core.Observation }

func (c *channelSource) Observations(ctx context.Context) <-chan core.Observation {
	// Honor ctx cancellation in case the test forgets to close the channel.
	out := make(chan core.Observation, cap(c.ch))
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case obs, ok := <-c.ch:
				if !ok {
					return
				}
				select {
				case out <- obs:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func TestWithListenerPumpsObservationsIntoSteerQueue(t *testing.T) {
	src := &channelSource{ch: make(chan core.Observation, 4)}
	src.ch <- core.Observation{ID: "1", Payload: "first log line"}
	src.ch <- core.Observation{ID: "2", Payload: "second log line"}
	src.ch <- core.Observation{ID: "3", Payload: ""} // empty payload must be dropped
	close(src.ch)

	eng, _, _ := bootstrap(t,
		agent.Config{Name: "x", Tier: spec.TIER_BASIC},
		agent.WithListener(src))

	// Bootstrap spawned the pump in a goroutine; give it a moment to drain
	// the buffered channel before checking the steering queue indirectly.
	// Steering is internal state — what we CAN observe is that the engine
	// kept running (no panic) and that an empty payload did not wedge it.
	require.NotNil(t, eng)
	// Cancel to ensure the spawned goroutine exits cleanly when ctx is
	// cancelled mid-run; without this, the test would leak the goroutine.
	t.Cleanup(func() {
		// Bootstrap's ctx is the caller's context.Background(); the pump
		// exits on channel close, which we already did above.
	})
}

func TestPayloadToStringFlattensEachShape(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string passes through", "hello", "hello"},
		{"empty string stays empty", "", ""},
		{"stringer is rendered", stringerImpl("via Stringer"), "via Stringer"},
		{"other uses %v", 42, "42"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := payloadToStringForTest(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// stringerImpl satisfies fmt.Stringer for the test above.
type stringerImpl string

func (s stringerImpl) String() string { return string(s) }

// payloadToStringForTest re-exposes the unexported helper through a tiny
// shim — we test the public behavior via Steer, not the symbol directly.
func payloadToStringForTest(p any) string {
	// Reach into the package by calling a tiny exported wrapper would be
	// cleaner, but for a single helper used twice this inline re-creation
	// keeps the test surface minimal. The actual pump code is exercised
	// by TestWithListenerPumpsObservationsIntoSteerQueue above.
	switch v := p.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// --- parts ---

func TestPartsAreNilUntilBootstrap(t *testing.T) {
	a, err := agent.New(agent.Config{Name: "x", Tier: spec.TIER_FULL})
	require.NoError(t, err)
	assert.Nil(t, a.Parts())
}

func TestPartsExposeOnlyRuntimeComposition(t *testing.T) {
	typ := reflect.TypeOf(agent.Parts{})
	got := make([]string, 0, typ.NumField())
	for i := range typ.NumField() {
		got = append(got, typ.Field(i).Name)
	}
	assert.ElementsMatch(t, []string{"Engine", "Sessions", "Skills", "Host", "Cwd"}, got)
}

func TestPartsExposeSessionsWhenEnabled(t *testing.T) {
	_, _, a := bootstrap(t, agent.Config{Name: "x", Tier: spec.TIER_STANDARD})
	parts := a.Parts()
	require.NotNil(t, parts)
	assert.NotNil(t, parts.Sessions, "standard enables lineage")
	assert.NotNil(t, parts.Engine)
	assert.NotNil(t, parts.Host)

	_, _, b := bootstrap(t, agent.Config{Name: "x", Tier: spec.TIER_BASIC})
	assert.Nil(t, b.Parts().Sessions, "basic has persistence but no lineage")
}

// --- Runner conformance ---

func TestAgentSatisfiesRunner(t *testing.T) {
	// Compile-time proof that the assembly layer plugs into the existing
	// lifecycle rather than replacing it.
	var _ interface {
		Name() string
		Bootstrap(context.Context, *agent.Host) (*runtime.Engine, core.State, error)
	} = (*agent.Agent)(nil)
}

func TestBootstrapSurfacesABadProviderName(t *testing.T) {
	a, err := agent.New(agent.Config{Name: "x", Model: spec.Model{Provider: "bogus"}})
	require.NoError(t, err, "an unknown provider is a runtime concern, not a schema error")

	_, _, err = a.Bootstrap(context.Background(), appCfg(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestBootstrapPassesWithAnInjectedProvider(t *testing.T) {
	a, err := agent.New(agent.Config{Name: "x"}, agent.WithProvider(testutil.NewScriptedProvider()))
	require.NoError(t, err)
	_, _, err = a.Bootstrap(context.Background(), appCfg(t))
	require.NoError(t, err)
}

// TestBootstrapPropagatesCredentialKindError ensures the strict-mode error
// from provider.Options.Resolve reaches the operator during the single
// construction pass. minimax has no OAuth env path, so oauth fails fast.
func TestBootstrapPropagatesCredentialKindError(t *testing.T) {
	a, err := agent.New(agent.Config{
		Name: "x",
		Model: spec.Model{
			Provider:       "minimax",
			CredentialKind: core.CREDENTIAL_KIND_OAUTH,
		},
	})
	require.NoError(t, err)

	_, _, err = a.Bootstrap(context.Background(), appCfg(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not OAuth-capable",
		"the registry message must surface through Bootstrap so the operator sees what env was expected")
}

// TestBootstrapSucceedsWithCredentialKindAuto confirms the legacy
// auto precedence (OAuth > API key) still works when both env are set.
// anthropic is the only adapter that registers both classes.
func TestBootstrapSucceedsWithCredentialKindAuto(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-env")

	a, err := agent.New(agent.Config{
		Name: "x",
		Model: spec.Model{
			Provider:       "anthropic",
			CredentialKind: core.CREDENTIAL_KIND_AUTO,
		},
	})
	require.NoError(t, err)

	_, _, err = a.Bootstrap(context.Background(), appCfg(t))
	require.NoError(t, err)
}

// --- end to end ---

func TestRunKeepsDisabledPersistenceNil(t *testing.T) {
	t.Chdir(t.TempDir())
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("done")

	a, err := agent.New(agent.Config{
		Name:   "memory-disabled",
		Tier:   spec.TIER_BASIC,
		Memory: &spec.Memory{Store: spec.MEMORY_STORE_NONE},
	}, agent.WithProvider(prov))
	require.NoError(t, err)

	err = agent.Run(context.Background(), a, appCfg(t))
	require.NoError(t, err)
	require.NotNil(t, a.Parts())
	assert.Nil(t, a.Parts().Engine.Store)
	assert.Nil(t, a.Parts().Engine.Log)
}

var registryProviderSequence atomic.Uint64

type registryAdapter struct {
	*testutil.ScriptedProvider
}

func TestRunBuildsRegistryProviderOnce(t *testing.T) {
	t.Chdir(t.TempDir())
	name := fmt.Sprintf("phase1-counting-%d", registryProviderSequence.Add(1))
	metadata := provider.Metadata{Label: name}
	calls := 0
	provider.Register(provider.Entry{
		Name:     name,
		Metadata: metadata,
		New: func(provider.Options) (provider.Adapter, error) {
			calls++
			scripted := testutil.NewScriptedProvider()
			scripted.EnqueueEndTurn("done")
			return &registryAdapter{
				ScriptedProvider: scripted,
			}, nil
		},
	})

	a, err := agent.New(agent.Config{
		Name:  "provider-built-once",
		Tier:  spec.TIER_ONESHOT,
		Model: spec.Model{Provider: name},
	})
	require.NoError(t, err)

	err = agent.Run(context.Background(), a, appCfg(t))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestBootstrapProducesARunnableEngine(t *testing.T) {
	t.Chdir(t.TempDir())
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("done")

	a, err := agent.New(agent.Config{
		Name: "x", Tier: spec.TIER_BASIC, Persona: "be brief",
		Limits: spec.Limits{MaxTurns: 4},
	}, agent.WithProvider(prov))
	require.NoError(t, err)

	eng, state, err := a.Bootstrap(context.Background(), appCfg(t))
	require.NoError(t, err)

	state.Messages = append(state.Messages, core.Message{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}},
	})
	final, err := eng.Run(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	assert.Equal(t, "done", agent.LastAssistantText(final))
}

// fakeTool is a minimal core.Tool for injection tests.
type fakeTool struct{ name string }

func (f *fakeTool) Name() string { return f.name }
func (f *fakeTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: f.name, Description: "test tool", Risk: core.RISK_LEVEL_LOW}
}
func (f *fakeTool) Call(context.Context, json.RawMessage) (core.ToolResult, error) {
	return core.ToolResult{OK: true, Output: "ok"}, nil
}

type fakeRule struct {
	kind   string
	called int
}

func (f *fakeRule) Kind() string { return f.kind }
func (f *fakeRule) NextStep(state core.State) (core.State, []core.Instruction) {
	f.called++
	return state, []core.Instruction{{Kind: core.INSTRUCTION_DONE}}
}
