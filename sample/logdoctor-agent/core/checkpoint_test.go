package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckpointRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "log-cursors.json")
	want := checkpoint{
		Version:    CHECKPOINT_VERSION,
		NextSource: "beta/app.log",
		Files: map[string]fileCursor{
			"alpha/app.log": {
				Offset:       42,
				AnchorBytes:  4,
				AnchorSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
	}

	require.NoError(t, saveCheckpoint(context.Background(), path, want))
	got, err := loadCheckpoint(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	tempFiles, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".log-cursors.json.*.tmp"))
	require.NoError(t, err)
	assert.Empty(t, tempFiles)
}

func TestCheckpointMissingStartsEmpty(t *testing.T) {
	got, err := loadCheckpoint(
		context.Background(),
		filepath.Join(t.TempDir(), "missing.json"),
	)
	require.NoError(t, err)
	assert.Equal(t, CHECKPOINT_VERSION, got.Version)
	assert.Empty(t, got.NextSource)
	assert.Empty(t, got.Files)
}

func TestCheckpointValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value checkpoint
	}{
		{
			name: "unsupported version",
			value: checkpoint{
				Version: 2,
				Files:   map[string]fileCursor{},
			},
		},
		{
			name: "absolute source",
			value: checkpoint{
				Version: CHECKPOINT_VERSION,
				Files: map[string]fileCursor{
					"/alpha/app.log": {},
				},
			},
		},
		{
			name: "negative offset",
			value: checkpoint{
				Version: CHECKPOINT_VERSION,
				Files: map[string]fileCursor{
					"alpha/app.log": {Offset: -1},
				},
			},
		},
		{
			name: "invalid hash",
			value: checkpoint{
				Version: CHECKPOINT_VERSION,
				Files: map[string]fileCursor{
					"alpha/app.log": {
						AnchorBytes:  4,
						AnchorSHA256: "not-a-sha256",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, validateCheckpoint(tt.value))
		})
	}
}
