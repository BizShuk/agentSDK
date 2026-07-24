package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const defMarkdown = `---
name: reviewer
description: Reviews diffs for bugs
provider: anthropic
model: claude-sonnet-5
tools: [Read, Grep]
---

You are a meticulous code reviewer.`

func TestParseDef(t *testing.T) {
	d := ParseDef("fallback", defMarkdown)
	assert.Equal(t, "reviewer", d.Name)
	assert.Equal(t, "Reviews diffs for bugs", d.Description)
	assert.Equal(t, "anthropic", d.Provider)
	assert.Equal(t, "claude-sonnet-5", d.Model)
	assert.Equal(t, []string{"Read", "Grep"}, d.Tools)
	assert.Equal(t, "You are a meticulous code reviewer.", d.Prompt)

	plain := ParseDef("explore", "just a prompt body")
	assert.Equal(t, "explore", plain.Name)
	assert.Equal(t, "just a prompt body", plain.Prompt)
}

func TestDiscoverDefs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte(defMarkdown), 0o640))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "explore.md"), []byte("go explore"), 0o640))

	defs, err := DiscoverDefs(dir)
	require.NoError(t, err)
	require.Len(t, defs, 2)
	assert.Equal(t, "explore", defs[0].Name, "sorted by name")
	assert.Equal(t, "reviewer", defs[1].Name)

	none, err := DiscoverDefs(filepath.Join(dir, "missing"))
	require.NoError(t, err)
	assert.Nil(t, none)
}

func TestSpawnerCall(t *testing.T) {
	var gotDepth int
	var gotDef Def
	spawner := NewSpawner(func(ctx context.Context, def Def, prompt string) (string, error) {
		gotDepth = Depth(ctx)
		gotDef = def
		return "sub result: " + prompt, nil
	}, ParseDef("reviewer", defMarkdown))

	args, _ := json.Marshal(map[string]string{"agent": "reviewer", "prompt": "check auth"})
	res, err := spawner.Call(context.Background(), args)
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.Equal(t, "sub result: check auth", res.Output)
	assert.Equal(t, 1, gotDepth, "child runs at depth 1")
	assert.Equal(t, "reviewer", gotDef.Name)
}

func TestSpawnerFailuresEncodedInResult(t *testing.T) {
	spawner := NewSpawner(func(_ context.Context, _ Def, _ string) (string, error) {
		return "", errors.New("boom")
	}, ParseDef("reviewer", defMarkdown))

	tests := []struct {
		name    string
		ctx     context.Context
		args    string
		wantErr string
	}{
		{"unknown agent", context.Background(), `{"agent":"ghost","prompt":"x"}`, "unknown agent"},
		{"missing fields", context.Background(), `{"agent":"reviewer"}`, "requires both"},
		{"bad json", context.Background(), `{`, "bad task args"},
		{"depth limit", WithDepth(context.Background(), 1), `{"agent":"reviewer","prompt":"x"}`, "depth limit"},
		{"runner error", context.Background(), `{"agent":"reviewer","prompt":"x"}`, "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := spawner.Call(tt.ctx, json.RawMessage(tt.args))
			require.NoError(t, err, "failures must be encoded in the result, not the error")
			assert.False(t, res.OK)
			assert.Contains(t, res.Error, tt.wantErr)
		})
	}
}

func TestSpawnerSchemaAndDescription(t *testing.T) {
	spawner := NewSpawner(nil, ParseDef("reviewer", defMarkdown))
	spec := spawner.Schema()
	assert.Equal(t, TOOL_NAME, spec.Name)
	assert.Contains(t, spawner.Description(), "reviewer: Reviews diffs for bugs")
}
