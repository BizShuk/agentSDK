package agent_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The built-in Sources are tested in prompt, where they live. What is
// left here is what this layer actually owns: turning the config's list
// of source NAMES into live Sources.

func TestBuildSourcesFollowsTheConfig(t *testing.T) {
	cases := []struct {
		name  string
		cfg   agent.Config
		count int
	}{
		{
			name:  "oneshot with persona only",
			cfg:   agent.Config{Tier: spec.TIER_ONESHOT, Persona: "p"},
			count: 1,
		},
		{
			name:  "oneshot with no persona",
			cfg:   agent.Config{Tier: spec.TIER_ONESHOT},
			count: 0,
		},
		{
			name:  "standard adds files",
			cfg:   agent.Config{Name: "x", Tier: spec.TIER_STANDARD},
			count: 1,
		},
		{
			name:  "full adds skills, env, reminder",
			cfg:   agent.Config{Name: "x", Tier: spec.TIER_FULL, Persona: "p"},
			count: 5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prepared, err := tc.cfg.Prepare()
			require.NoError(t, err)
			got, err := agent.BuildSources(prepared, nil, t.TempDir())
			require.NoError(t, err)
			assert.Len(t, got, tc.count)
		})
	}
}

func TestBuildSourcesPersonaWorksAtEveryTier(t *testing.T) {
	// Persona lives outside the Prompt block precisely so that one-shot,
	// which has no Prompt block, can still set an identity.
	for _, tier := range spec.Tiers() {
		t.Run(tier, func(t *testing.T) {
			cfg := agent.Config{Name: "x", Tier: tier, Persona: "IDENTITY"}
			prepared, err := cfg.Prepare()
			require.NoError(t, err)

			sources, err := agent.BuildSources(prepared, nil, t.TempDir())
			require.NoError(t, err)

			msgs, err := prompt.Builder{Sources: sources}.Seed(context.Background(), prompt.Req{Cwd: t.TempDir()})
			require.NoError(t, err)
			require.NotEmpty(t, msgs)
			assert.Contains(t, msgs[0].Parts[0].Text, "IDENTITY")
		})
	}
}

// SkillSource is the one Source that must live here, because it is the
// one that needs to know two packages exist. If a future refactor adds
// another Source to this package that only needs prompt, it belongs in
// prompt instead.
func TestSkillSourceHandlesNilRegistry(t *testing.T) {
	got, err := agent.SkillSource(nil).Sections(context.Background(), prompt.Req{})
	require.NoError(t, err)
	assert.Empty(t, got, "no skill registry means no skill index, not a failure")
}
