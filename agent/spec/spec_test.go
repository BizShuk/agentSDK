package spec_test

import (
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- tier expansion ---

func TestExpandTierLadderIsMonotonic(t *testing.T) {
	// Each tier must turn on a superset of the tier below it. Encoded as
	// block presence so a future tier cannot quietly drop a capability.
	blocks := func(c spec.Config) map[string]bool {
		return map[string]bool{
			"middleware": c.Middleware != nil,
			"memory":     c.Memory != nil,
			"tools":      c.Tools != nil,
			"safety":     c.Safety != nil,
			"prompt":     c.Prompt != nil,
			"skills":     c.Skills != nil,
			"subagents":  c.Subagents != nil,
			"sessions":   c.Sessions != nil,
			"output":     c.Output != nil,
		}
	}

	var prev map[string]bool
	var prevTier string
	for _, tier := range spec.Tiers() {
		got, err := spec.Config{Name: "x", Tier: tier}.Expand()
		require.NoError(t, err)
		cur := blocks(got)
		if prev != nil {
			for name, on := range prev {
				if on {
					assert.Truef(t, cur[name],
						"tier %q dropped block %q that tier %q had — ladder must be monotonic",
						tier, name, prevTier)
				}
			}
		}
		prev, prevTier = cur, tier
	}
}

func TestExpandTierBlocks(t *testing.T) {
	cases := []struct {
		tier        string
		wantOn      []string
		wantOff     []string
		wantTurns   int
		wantPreset  string
		wantSources []string
	}{
		{
			tier:      spec.TIER_ONESHOT,
			wantOff:   []string{"middleware", "memory", "tools", "prompt"},
			wantTurns: spec.DEFAULT_ONESHOT_TURNS,
		},
		{
			tier:       spec.TIER_BASIC,
			wantOn:     []string{"middleware", "memory"},
			wantOff:    []string{"tools", "safety", "prompt", "skills"},
			wantTurns:  spec.DEFAULT_MAX_TURNS,
			wantPreset: spec.MIDDLEWARE_DEFAULT,
		},
		{
			tier:        spec.TIER_STANDARD,
			wantOn:      []string{"middleware", "memory", "tools", "safety", "prompt", "sessions"},
			wantOff:     []string{"skills", "subagents"},
			wantTurns:   spec.DEFAULT_STANDARD_TURNS,
			wantPreset:  spec.MIDDLEWARE_SECURE,
			wantSources: []string{spec.SOURCE_FILES},
		},
		{
			tier:       spec.TIER_FULL,
			wantOn:     []string{"middleware", "memory", "tools", "safety", "prompt", "sessions", "skills", "subagents", "output"},
			wantTurns:  spec.DEFAULT_STANDARD_TURNS,
			wantPreset: spec.MIDDLEWARE_SECURE,
			wantSources: []string{
				spec.SOURCE_FILES, spec.SOURCE_SKILLS, spec.SOURCE_ENV, spec.SOURCE_REMINDER,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.tier, func(t *testing.T) {
			got, err := spec.Config{Name: "x", Tier: tc.tier}.Expand()
			require.NoError(t, err)

			on := map[string]bool{
				"middleware": got.Middleware != nil, "memory": got.Memory != nil,
				"tools": got.Tools != nil, "safety": got.Safety != nil,
				"prompt": got.Prompt != nil, "skills": got.Skills != nil,
				"subagents": got.Subagents != nil, "sessions": got.Sessions != nil,
				"output": got.Output != nil,
			}
			for _, name := range tc.wantOn {
				assert.Truef(t, on[name], "block %q should be on at tier %q", name, tc.tier)
			}
			for _, name := range tc.wantOff {
				assert.Falsef(t, on[name], "block %q should be off at tier %q", name, tc.tier)
			}
			assert.Equal(t, tc.wantTurns, got.Limits.MaxTurns)
			if tc.wantPreset != "" {
				require.NotNil(t, got.Middleware)
				assert.Equal(t, tc.wantPreset, got.Middleware.Preset)
			}
			if tc.wantSources != nil {
				require.NotNil(t, got.Prompt)
				assert.Equal(t, tc.wantSources, got.Prompt.Sources)
			}
			require.NoError(t, got.Validate(), "every tier default must validate")
		})
	}
}

func TestExpandOneshotLeavesPersistenceOff(t *testing.T) {
	// The decision this locks in: T0 must stay usable without a Name.
	got, err := spec.Config{Tier: spec.TIER_ONESHOT}.Prepare()
	require.NoError(t, err)
	assert.Nil(t, got.Memory, "oneshot must not persist by default")
	assert.Empty(t, got.Name)

	// One line opts back in — and then Name becomes required.
	_, err = spec.Config{Tier: spec.TIER_ONESHOT, Memory: &spec.Memory{}}.Prepare()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestExpandExplicitBlockWins(t *testing.T) {
	got, err := spec.Config{
		Name:       "x",
		Tier:       spec.TIER_FULL,
		Middleware: &spec.Middleware{Preset: spec.MIDDLEWARE_NONE},
		Memory:     &spec.Memory{Store: spec.MEMORY_STORE_NONE},
	}.Expand()
	require.NoError(t, err)
	assert.Equal(t, spec.MIDDLEWARE_NONE, got.Middleware.Preset)
	assert.Equal(t, spec.MEMORY_STORE_NONE, got.Memory.Store)
}

func TestExpandDoesNotMutateInput(t *testing.T) {
	in := spec.Config{Name: "x", Tier: spec.TIER_FULL}
	_, err := in.Expand()
	require.NoError(t, err)
	assert.Nil(t, in.Tools, "Expand must not write through to the caller's literal")
	assert.Empty(t, in.Reasoning.Style)
}

func TestExpandSkillsImpliesPromptSource(t *testing.T) {
	// Asking for skills at a tier that has no prompt block should still
	// deliver the skill index — the caller should not have to know that
	// skills reach the model through the prompt.
	got, err := spec.Config{Name: "x", Tier: spec.TIER_BASIC, Skills: &spec.Skills{}}.Prepare()
	require.NoError(t, err)
	require.NotNil(t, got.Prompt)
	assert.Contains(t, got.Prompt.Sources, spec.SOURCE_SKILLS)
}

func TestExpandUnknownTier(t *testing.T) {
	_, err := spec.Config{Name: "x", Tier: "turbo"}.Expand()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tier")
}

// --- reasoning: the two-layer decision ---

func TestExpandReasoningDefaults(t *testing.T) {
	got, err := spec.Config{Name: "x"}.Expand()
	require.NoError(t, err)
	assert.Equal(t, spec.DEFAULT_STYLE, got.Reasoning.Style)
	assert.Equal(t, []string{spec.DEFAULT_STYLE}, got.Reasoning.Enable,
		"empty enable must register exactly the selected style")
}

func TestOneshotTierWithExplicitReasoningIsLegal(t *testing.T) {
	// The decision: tier and reasoning are orthogonal axes. A one-shot
	// tier carrying a multi-step strategy is accepted, not rejected —
	// with no tools bound the engine short-circuits after one model call
	// regardless of the style.
	for _, style := range spec.Values(spec.StyleChoices()) {
		t.Run(style, func(t *testing.T) {
			got, err := spec.Config{
				Tier:      spec.TIER_ONESHOT,
				Reasoning: spec.Reasoning{Style: style},
			}.Prepare()
			require.NoError(t, err, "tier × reasoning must not be validated as a conflict")
			assert.Equal(t, style, got.Reasoning.Style)
		})
	}
}

func TestValidateReasoningErrors(t *testing.T) {
	cases := []struct {
		name    string
		in      spec.Reasoning
		wantErr string
	}{
		{
			name:    "unknown style",
			in:      spec.Reasoning{Style: "vibes"},
			wantErr: "unknown reasoning.style",
		},
		{
			name:    "unknown enable entry",
			in:      spec.Reasoning{Style: spec.DEFAULT_STYLE, Enable: []string{spec.DEFAULT_STYLE, "vibes"}},
			wantErr: "unknown reasoning.enable entry",
		},
		{
			name:    "style not registered",
			in:      spec.Reasoning{Style: "choose_agent", Enable: []string{"think_then_act"}},
			wantErr: "is not in reasoning.enable",
		},
		{
			name:    "duplicate enable entry",
			in:      spec.Reasoning{Style: spec.DEFAULT_STYLE, Enable: []string{spec.DEFAULT_STYLE, spec.DEFAULT_STYLE}},
			wantErr: "duplicate reasoning.enable entry",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := spec.Config{Name: "x", Reasoning: tc.in}.Prepare()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateRouterStyleWithEnableList(t *testing.T) {
	got, err := spec.Config{
		Name: "x",
		Reasoning: spec.Reasoning{
			Style:  "choose_agent",
			Enable: []string{"choose_agent", "think_then_act", "plan_then_run"},
		},
	}.Prepare()
	require.NoError(t, err)
	assert.Len(t, got.Reasoning.Enable, 3)
}

// --- validation ---

func TestValidateBlockVariants(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*spec.Config)
		wantErr string
	}{
		{"bad middleware preset", func(c *spec.Config) { c.Middleware = &spec.Middleware{Preset: "turbo"} }, "unknown middleware.preset"},
		{"bad memory store", func(c *spec.Config) { c.Memory = &spec.Memory{Store: "redis"} }, "unknown memory.store"},
		{"bad safety mode", func(c *spec.Config) { c.Safety = &spec.Safety{Mode: "yolo"} }, "unknown safety.mode"},
		{"bad output format", func(c *spec.Config) { c.Output = &spec.Output{Format: "xml"} }, "unknown output.format"},
		{"bad autonomy", func(c *spec.Config) { c.Limits.Autonomy = "L9" }, "unknown limits.autonomy"},
		{"bad builtin tool", func(c *spec.Config) { c.Tools = &spec.Tools{Builtin: []string{"read", "curl"}} }, "unknown tools.builtin"},
		{"bad prompt source", func(c *spec.Config) { c.Prompt = &spec.Prompt{Sources: []string{"telepathy"}} }, "unknown prompt.sources"},
		{"bad wall time", func(c *spec.Config) { c.Limits.MaxWallTime = "ten minutes" }, "not a Go duration"},
		{"malformed safety rule", func(c *spec.Config) {
			c.Tools = &spec.Tools{}
			c.Safety = &spec.Safety{Deny: []string{"sudo"}}
		}, "must look like tool(target)"},
		{"name with separator", func(c *spec.Config) { c.Name = "a/b" }, "must not contain a path separator"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := spec.Config{Name: "x", Tier: spec.TIER_STANDARD}
			tc.mutate(&c)
			_, err := c.Prepare()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateCrossBlockCoherence(t *testing.T) {
	t.Run("safety without tools", func(t *testing.T) {
		_, err := spec.Config{Name: "x", Tier: spec.TIER_BASIC, Safety: &spec.Safety{}}.Prepare()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nothing to gate")
	})

	t.Run("sessions without persistence", func(t *testing.T) {
		_, err := spec.Config{
			Name: "x", Tier: spec.TIER_BASIC,
			Memory:   &spec.Memory{Store: spec.MEMORY_STORE_NONE},
			Sessions: &spec.Sessions{},
		}.Prepare()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sessions needs memory.store")
	})
}

func TestValidateReportsEveryProblemAtOnce(t *testing.T) {
	_, err := spec.Config{
		Name:      "x",
		Tier:      spec.TIER_STANDARD,
		Reasoning: spec.Reasoning{Style: "vibes"},
		Memory:    &spec.Memory{Store: "redis"},
		Output:    &spec.Output{Format: "xml"},
	}.Prepare()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "reasoning.style")
	assert.Contains(t, msg, "memory.store")
	assert.Contains(t, msg, "output.format")
	assert.GreaterOrEqual(t, strings.Count(msg, "spec:"), 3,
		"errors must be joined so one run reports everything")
}

func TestValidateWithoutExpandComplains(t *testing.T) {
	err := spec.Config{Name: "x", Middleware: &spec.Middleware{}}.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "call Expand before Validate")
}

// --- serialization ---

func TestDecodeAbsentBlockIsOff(t *testing.T) {
	// The layer-1 rule in JSON terms: a missing key is off, an empty
	// object is on with defaults.
	off, err := spec.DecodeBytes([]byte(`{"name":"x","tier":"basic"}`))
	require.NoError(t, err)
	assert.Nil(t, off.Skills)

	on, err := spec.DecodeBytes([]byte(`{"name":"x","tier":"basic","skills":{}}`))
	require.NoError(t, err)
	require.NotNil(t, on.Skills)
}

func TestEncodeDecodeRoundTripIsFixedPoint(t *testing.T) {
	for _, tier := range spec.Tiers() {
		t.Run(tier, func(t *testing.T) {
			first, err := spec.Config{Name: "round", Tier: tier, Persona: "you are terse"}.Prepare()
			require.NoError(t, err)

			raw, err := spec.EncodeBytes(first)
			require.NoError(t, err)

			second, err := spec.DecodeBytes(raw)
			require.NoError(t, err)
			assert.Equal(t, first, second, "an expanded config must survive a write/read cycle unchanged")

			again, err := spec.EncodeBytes(second)
			require.NoError(t, err)
			assert.JSONEq(t, string(raw), string(again))
		})
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := spec.DecodeBytes([]byte(`{"name":"x","toolz":{}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "toolz")
}

func TestDecodeValidates(t *testing.T) {
	_, err := spec.DecodeBytes([]byte(`{"name":"x","reasoning":{"style":"vibes"}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown reasoning.style")
}

// --- choices ---

func TestChoicesCoverValidationVocabulary(t *testing.T) {
	// Choice lists and validation must not drift: every key the wizard
	// walks has to answer, and every list needs exactly one default
	// (except the multi-selects, which mark several).
	multi := map[string]bool{"prompt.sources": true, "tools.builtin": true}
	for _, key := range spec.VariantKeys() {
		t.Run(key, func(t *testing.T) {
			cs := spec.VariantChoices(key)
			require.NotEmpty(t, cs, "VariantKeys lists %q but VariantChoices has no answer", key)
			if multi[key] {
				return
			}
			var defaults int
			for _, c := range cs {
				if c.Default {
					defaults++
				}
			}
			assert.Equal(t, 1, defaults, "single-select %q needs exactly one default", key)
		})
	}
}

func TestStyleChoicesMatchCoreConstants(t *testing.T) {
	got := spec.Values(spec.StyleChoices())
	assert.ElementsMatch(t, []string{
		"think_then_act", "plan_then_run", "do_then_review",
		"one_shot", "learn_from_failure", "choose_agent",
	}, got, "StyleChoices must track core's ReasoningStyle constants")
}

func TestTierChoicesMatchLadder(t *testing.T) {
	assert.Equal(t, spec.Tiers(), spec.Values(spec.TierChoices()))
	assert.Equal(t, spec.DEFAULT_TIER, spec.DefaultOf(spec.TierChoices()))
}

func TestDefaultChoicesAreAcceptedByValidate(t *testing.T) {
	// Whatever the wizard offers as a default must produce a valid
	// config — otherwise pressing Enter through every stage yields
	// something the builder rejects.
	c := spec.Config{Name: "x", Tier: spec.TIER_STANDARD}
	c.Limits.Autonomy = spec.DefaultOf(spec.VariantChoices("limits.autonomy"))
	c.Middleware = &spec.Middleware{Preset: spec.DefaultOf(spec.VariantChoices("middleware.preset"))}
	c.Memory = &spec.Memory{
		Store:      spec.DefaultOf(spec.VariantChoices("memory.store")),
		Compaction: spec.DefaultOf(spec.VariantChoices("memory.compaction")),
	}
	c.Safety = &spec.Safety{
		Mode:     spec.DefaultOf(spec.VariantChoices("safety.mode")),
		Fallback: spec.DefaultOf(spec.VariantChoices("safety.fallback")),
	}
	c.Output = &spec.Output{Format: spec.DefaultOf(spec.VariantChoices("output.format"))}

	_, err := c.Prepare()
	require.NoError(t, err)
}
