package source_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/prompt/source"
	sdkskill "github.com/bizshuk/agentsdk/skill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time guarantee that *skill.Registry satisfies SkillProvider.
// Putting the assertion here (rather than in skill/) keeps the rule
// "skill never imports prompt/*" intact — the dependency direction is
// test-only and never shipped.
var _ source.SkillProvider = (*sdkskill.Registry)(nil)

func sectionsOfSkill(t *testing.T, s prompt.Source, req prompt.Req) []prompt.Section {
	t.Helper()
	got, err := s.Sections(context.Background(), req)
	require.NoError(t, err)
	return got
}

func TestSkillSourceHandlesNilProvider(t *testing.T) {
	// nil interface is a valid input: it means "no skill registry wired
	// in", and the source must contribute nothing rather than panic.
	got := sectionsOfSkill(t, source.SkillSource(nil), prompt.Req{})
	assert.Empty(t, got, "no skill registry means no skill index, not a failure")
}

// fakeSkillProvider is a hand-rolled SkillProvider used to verify that
// SkillSource delegates SystemPrompt() through the interface, not via
// type assertion. If a future refactor reaches inside the provider, this
// test breaks.
type fakeSkillProvider struct {
	text string
}

func (f fakeSkillProvider) SystemPrompt() string { return f.text }

func TestSkillSourceDelegatesSystemPrompt(t *testing.T) {
	prov := fakeSkillProvider{text: "index of skills"}
	got := sectionsOfSkill(t, source.SkillSource(prov), prompt.Req{})
	require.Len(t, got, 1)
	assert.Equal(t, prompt.SLOT_SYSTEM, got[0].Slot)
	assert.Equal(t, prompt.ORDER_SKILLS, got[0].Order)
	assert.Equal(t, "index of skills", got[0].Text)
}

// Note on the nil contract: SkillSource(nil) — a nil interface — is
// supported and contributes nothing. A typed-nil pointer (e.g.
// `var reg *skill.Registry; source.SkillSource(reg)`) is a non-nil
// interface wrapping a nil pointer, and the source will panic when it
// calls SystemPrompt. That is the standard Go interface-nil pitfall
// and matches the prior agent.SkillSource contract; callers must pass
// a real value or a true nil interface.
