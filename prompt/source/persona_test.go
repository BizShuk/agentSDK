package source_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/prompt/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sectionsOf(t *testing.T, s prompt.Source, req prompt.Req) []prompt.Section {
	t.Helper()
	got, err := s.Sections(context.Background(), req)
	require.NoError(t, err)
	return got
}

func TestPersonaSourceUsesTheStableOrder(t *testing.T) {
	got := sectionsOf(t, source.PersonaSource("you are terse"), prompt.Req{})
	require.Len(t, got, 1)
	assert.Equal(t, prompt.SLOT_SYSTEM, got[0].Slot)
	assert.Equal(t, prompt.ORDER_PERSONA, got[0].Order,
		"persona changes least often, so it anchors the cacheable prefix")
	assert.Equal(t, "you are terse", got[0].Text)
}

func TestPersonaSourceEmptyContributesNothing(t *testing.T) {
	assert.Empty(t, sectionsOf(t, source.PersonaSource(""), prompt.Req{}))
}
