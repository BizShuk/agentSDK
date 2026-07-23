package prompt_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sec(slot prompt.Slot, name, text string, order int) prompt.Section {
	return prompt.Section{Slot: slot, Name: name, Text: text, Order: order}
}

func src(secs ...prompt.Section) prompt.Source {
	return prompt.SourceFunc(func(context.Context, prompt.Req) ([]prompt.Section, error) {
		return secs, nil
	})
}

// --- seeding ---

func TestSeedMergesSystemSectionsIntoOneMessage(t *testing.T) {
	// One ROLE_SYSTEM message, not several: providers disagree on the
	// shape (Anthropic has a separate system param, OpenAI Chat uses a
	// message), and State should not carry that difference.
	b := prompt.Builder{Sources: []prompt.Source{
		src(sec(prompt.SLOT_SYSTEM, "persona", "you are terse", prompt.ORDER_PERSONA)),
		src(sec(prompt.SLOT_SYSTEM, "files", "project rules", prompt.ORDER_FILES)),
		src(sec(prompt.SLOT_SYSTEM, "env", "cwd: /tmp", prompt.ORDER_ENV)),
	}}

	msgs, err := b.Seed(context.Background(), prompt.Req{Input: "hello"})
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	assert.Equal(t, core.ROLE_SYSTEM, msgs[0].Role)
	require.Len(t, msgs[0].Parts, 1)
	assert.Equal(t, "you are terse\n\nproject rules\n\ncwd: /tmp", msgs[0].Parts[0].Text)

	assert.Equal(t, core.ROLE_USER, msgs[1].Role)
	assert.Equal(t, "hello", msgs[1].Parts[0].Text)
}

func TestSeedOrdersStableBeforeVolatile(t *testing.T) {
	// Registered in the worst possible order; Order must still put the
	// cache-stable content first.
	b := prompt.Builder{Sources: []prompt.Source{
		src(sec(prompt.SLOT_SYSTEM, "env", "ENV", prompt.ORDER_ENV)),
		src(sec(prompt.SLOT_SYSTEM, "skills", "SKILLS", prompt.ORDER_SKILLS)),
		src(sec(prompt.SLOT_SYSTEM, "persona", "PERSONA", prompt.ORDER_PERSONA)),
		src(sec(prompt.SLOT_SYSTEM, "files", "FILES", prompt.ORDER_FILES)),
	}}

	msgs, err := b.Seed(context.Background(), prompt.Req{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "PERSONA\n\nFILES\n\nSKILLS\n\nENV", msgs[0].Parts[0].Text)
}

func TestSeedTiesKeepRegistrationOrder(t *testing.T) {
	const same = 20
	b := prompt.Builder{Sources: []prompt.Source{
		src(sec(prompt.SLOT_SYSTEM, "a", "A", same)),
		src(sec(prompt.SLOT_SYSTEM, "b", "B", same)),
		src(sec(prompt.SLOT_SYSTEM, "c", "C", same)),
	}}
	msgs, err := b.Seed(context.Background(), prompt.Req{})
	require.NoError(t, err)
	assert.Equal(t, "A\n\nB\n\nC", msgs[0].Parts[0].Text)
}

func TestSeedZeroBuilderProducesNothing(t *testing.T) {
	msgs, err := prompt.Builder{}.Seed(context.Background(), prompt.Req{})
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestSeedInputOnly(t *testing.T) {
	msgs, err := prompt.Builder{}.Seed(context.Background(), prompt.Req{Input: "just this"})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, core.ROLE_USER, msgs[0].Role)
}

func TestSeedSkipsBlankSections(t *testing.T) {
	b := prompt.Builder{Sources: []prompt.Source{
		src(sec(prompt.SLOT_SYSTEM, "empty", "   \n  ", prompt.ORDER_FILES)),
		src(sec(prompt.SLOT_SYSTEM, "real", "kept", prompt.ORDER_SKILLS)),
	}}
	msgs, err := b.Seed(context.Background(), prompt.Req{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "kept", msgs[0].Parts[0].Text)
}

func TestSeedUserSlotMergesWithInput(t *testing.T) {
	b := prompt.Builder{Sources: []prompt.Source{
		src(sec(prompt.SLOT_USER, "expansion", "expanded command body", 10)),
	}}
	msgs, err := b.Seed(context.Background(), prompt.Req{Input: "raw input"})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "expanded command body\n\nraw input", msgs[0].Parts[0].Text)
}

// --- per-turn ---

func TestTurnCarriesRemindersWithTheUserMessage(t *testing.T) {
	// Reminders must NOT rewrite the system prompt — that would break the
	// provider's cached prefix on every single turn.
	b := prompt.Builder{Sources: []prompt.Source{
		src(sec(prompt.SLOT_SYSTEM, "persona", "SYSTEM", prompt.ORDER_PERSONA)),
		src(sec(prompt.SLOT_REMINDER, "budget", "2 turns left", prompt.ORDER_REMINDER)),
	}}

	msgs, err := b.Turn(context.Background(), prompt.Req{Turn: 3, Input: "carry on"})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, core.ROLE_USER, msgs[0].Role)
	assert.Equal(t, "2 turns left\n\ncarry on", msgs[0].Parts[0].Text)
	assert.NotContains(t, msgs[0].Parts[0].Text, "SYSTEM",
		"the system slot belongs to Seed, not to every turn")
}

func TestTurnWithNothingToSayReturnsNothing(t *testing.T) {
	msgs, err := prompt.Builder{}.Turn(context.Background(), prompt.Req{})
	require.NoError(t, err)
	assert.Nil(t, msgs)
}

// --- budget ---

func TestBudgetDropsTheMostVolatileFirst(t *testing.T) {
	// Dropping from the tail is what protects the cache-stable prefix:
	// persona and project files survive, env is sacrificed.
	b := prompt.Builder{
		MaxBytes: 20,
		Sources: []prompt.Source{
			src(sec(prompt.SLOT_SYSTEM, "persona", "0123456789", prompt.ORDER_PERSONA)),
			src(sec(prompt.SLOT_SYSTEM, "files", "0123456789", prompt.ORDER_FILES)),
			src(sec(prompt.SLOT_SYSTEM, "env", "0123456789", prompt.ORDER_ENV)),
		},
	}

	var dropped []string
	b.OnBudget = func(s prompt.Section, limit int) { dropped = append(dropped, s.Name) }

	msgs, err := b.Seed(context.Background(), prompt.Req{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "0123456789\n\n0123456789", msgs[0].Parts[0].Text)
	assert.Equal(t, []string{"env"}, dropped, "the budget must report what it cut")
}

func TestBudgetNeverSplitsASection(t *testing.T) {
	// Truncating mid-text would corrupt whichever section straddled the
	// boundary, so a section is kept whole or not at all.
	b := prompt.Builder{
		MaxBytes: 5,
		Sources: []prompt.Source{
			src(sec(prompt.SLOT_SYSTEM, "big", strings.Repeat("x", 100), prompt.ORDER_PERSONA)),
		},
	}
	msgs, err := b.Seed(context.Background(), prompt.Req{})
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestBudgetDefaultAllowsNormalContent(t *testing.T) {
	b := prompt.Builder{Sources: []prompt.Source{
		src(sec(prompt.SLOT_SYSTEM, "files", strings.Repeat("y", 1000), prompt.ORDER_FILES)),
	}}
	msgs, err := b.Seed(context.Background(), prompt.Req{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Len(t, msgs[0].Parts[0].Text, 1000)
}

// --- failure handling ---

func TestSourceErrorAbortsTheBuild(t *testing.T) {
	// Half a system prompt is worse than an error: the model would run
	// without instructions it was supposed to have, and nothing would say so.
	boom := errors.New("boom")
	b := prompt.Builder{Sources: []prompt.Source{
		src(sec(prompt.SLOT_SYSTEM, "ok", "fine", prompt.ORDER_PERSONA)),
		prompt.SourceFunc(func(context.Context, prompt.Req) ([]prompt.Section, error) {
			return nil, boom
		}),
	}}
	_, err := b.Seed(context.Background(), prompt.Req{})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestNilSourceIsSkipped(t *testing.T) {
	b := prompt.Builder{Sources: []prompt.Source{
		nil,
		src(sec(prompt.SLOT_SYSTEM, "real", "kept", prompt.ORDER_PERSONA)),
	}}
	msgs, err := b.Seed(context.Background(), prompt.Req{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
}

// --- Static ---

func TestStaticSkipsBlankText(t *testing.T) {
	s := prompt.Static(prompt.SLOT_SYSTEM, "persona", "  ", prompt.ORDER_PERSONA)
	got, err := s.Sections(context.Background(), prompt.Req{})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSourcesSeeReadOnlyState(t *testing.T) {
	// Req carries State by value so a Source cannot write back into the
	// run — mutating history from a prompt contributor would make the
	// transcript depend on assembly order.
	state := core.State{RunID: "r1", Turn: 4, Budget: core.Budget{MaxTurns: 5}}
	var seen core.State
	b := prompt.Builder{Sources: []prompt.Source{
		prompt.SourceFunc(func(_ context.Context, req prompt.Req) ([]prompt.Section, error) {
			seen = req.State
			req.State.Turn = 99
			return nil, nil
		}),
	}}
	_, err := b.Seed(context.Background(), prompt.Req{State: state})
	require.NoError(t, err)
	assert.Equal(t, 4, seen.Turn)
	assert.Equal(t, 4, state.Turn, "the caller's State must be untouched")
}
