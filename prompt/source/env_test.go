package source_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/prompt/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sectionsOfEnv(t *testing.T, s prompt.Source, req prompt.Req) []prompt.Section {
	t.Helper()
	got, err := s.Sections(context.Background(), req)
	require.NoError(t, err)
	return got
}

func TestEnvSourceIsLastAmongSystemSections(t *testing.T) {
	got := sectionsOfEnv(t, source.EnvSource(), prompt.Req{Cwd: t.TempDir()})
	require.Len(t, got, 1)
	assert.Equal(t, prompt.ORDER_ENV, got[0].Order)
	assert.Greater(t, got[0].Order, prompt.ORDER_SKILLS,
		"env changes every run, so it must not sit inside the cached prefix")
	assert.Contains(t, got[0].Text, "working directory:")
	assert.Contains(t, got[0].Text, "date:")
}
