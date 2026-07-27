package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaderRetriesUntilCommitThenReadsOnlyAppendedBytes(t *testing.T) {
	root := t.TempDir()
	logPath := writeTestLog(t, root, "alpha", "daemon.log", []byte("first\n"))
	cursorPath := filepath.Join(t.TempDir(), "cursor.json")

	reader, err := NewReader(root, cursorPath)
	require.NoError(t, err)

	first, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assertSinglePart(t, first, "alpha/daemon.log", 0, []byte("first\n"))
	if _, err := os.Stat(cursorPath); !os.IsNotExist(err) {
		t.Fatalf("Next wrote cursor before Commit: %v", err)
	}

	retry, _, err := reader.Next(context.Background())
	require.NoError(t, err)
	assertSinglePart(t, retry, "alpha/daemon.log", 0, []byte("first\n"))

	require.NoError(t, reader.Commit(context.Background(), first))
	info, err := os.Stat(cursorPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	if _, err := file.WriteString("second\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	require.NoError(t, file.Close())

	appended, _, err := reader.Next(context.Background())
	require.NoError(t, err)
	assertSinglePart(t, appended, "alpha/daemon.log", 6, []byte("second\n"))
}

func TestReaderBoundsAndSharesBatchAcrossSources(t *testing.T) {
	root := t.TempDir()
	writeTestLog(
		t,
		root,
		"alpha",
		"daemon.log",
		bytes.Repeat([]byte("a"), MAX_BATCH_BYTES),
	)
	writeTestLog(
		t,
		root,
		"beta",
		"daemon.log",
		bytes.Repeat([]byte("b"), MAX_BATCH_BYTES),
	)

	reader, err := NewReader(root, filepath.Join(t.TempDir(), "cursor.json"))
	require.NoError(t, err)
	batch, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, MAX_BATCH_BYTES, batch.Bytes)
	require.Len(t, batch.Parts, 2)
	assert.NotEmpty(t, batch.Parts[0].Content, "a noisy source starved alpha")
	assert.NotEmpty(t, batch.Parts[1].Content, "a noisy source starved beta")
	assert.True(t, batch.Backlog)
}

func TestReaderRestartsAtZeroAfterTruncate(t *testing.T) {
	root := t.TempDir()
	logPath := writeTestLog(t, root, "alpha", "daemon.log", []byte("old-content\n"))

	reader, err := NewReader(root, filepath.Join(t.TempDir(), "cursor.json"))
	require.NoError(t, err)
	first, _, err := reader.Next(context.Background())
	require.NoError(t, err)
	require.NoError(t, reader.Commit(context.Background(), first))

	require.NoError(t, os.WriteFile(logPath, []byte("new\n"), 0o600))
	truncated, _, err := reader.Next(context.Background())
	require.NoError(t, err)
	assertSinglePart(t, truncated, "alpha/daemon.log", 0, []byte("new\n"))
}

func TestReaderIgnoresOwnLogsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	validPath := writeTestLog(t, root, "alpha", "daemon.log", []byte("valid\n"))
	writeTestLog(t, root, APP_NAME, "daemon.log", []byte("feedback\n"))

	linkDir := filepath.Join(root, "beta", "logs")
	require.NoError(t, os.MkdirAll(linkDir, 0o700))
	if err := os.Symlink(validPath, filepath.Join(linkDir, "linked.log")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	require.NoError(t, os.Mkdir(filepath.Join(linkDir, "nested"), 0o700))

	reader, err := NewReader(root, filepath.Join(t.TempDir(), "cursor.json"))
	require.NoError(t, err)
	batch, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assertSinglePart(t, batch, "alpha/daemon.log", 0, []byte("valid\n"))
}

func TestReaderRejectsBatchWithoutPendingCursor(t *testing.T) {
	reader, err := NewReader(t.TempDir(), filepath.Join(t.TempDir(), "cursor.json"))
	require.NoError(t, err)
	require.Error(t, reader.Commit(context.Background(), Batch{}))
}

func writeTestLog(
	t *testing.T,
	root string,
	app string,
	name string,
	content []byte,
) string {
	t.Helper()
	dir := filepath.Join(root, app, "logs")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	filePath := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(filePath, content, 0o600))
	return filePath
}

func assertSinglePart(
	t *testing.T,
	batch Batch,
	source string,
	start int64,
	content []byte,
) {
	t.Helper()
	assert.Equal(t, len(content), batch.Bytes)
	require.Len(t, batch.Parts, 1)
	part := batch.Parts[0]
	assert.Equal(t, source, part.Source)
	assert.Equal(t, start, part.StartOffset)
	assert.Equal(t, start+int64(len(content)), part.EndOffset)
	assert.True(t, bytes.Equal(content, part.Content))
}
