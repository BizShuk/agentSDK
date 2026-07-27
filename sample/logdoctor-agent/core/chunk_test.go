package core_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkReaderDiscoversOnlyDirectRegularLogs(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "log-cursors.json")

	writeLog(t, root, "alpha", "app.log", []byte("alpha\n"))
	writeFile(t, filepath.Join(root, "alpha", "log", "singular.log"), []byte("ignored\n"))
	writeFile(t, filepath.Join(root, "alpha", "logs", "nested", "deep.log"), []byte("ignored\n"))
	writeLog(t, root, "logdoctor", "self.log", []byte("ignored\n"))

	target := filepath.Join(root, "alpha", "logs", "app.log")
	require.NoError(t, os.Symlink(
		target,
		filepath.Join(root, "alpha", "logs", "linked.log"),
	))
	externalApp := filepath.Join(t.TempDir(), "external-app")
	writeFile(t, filepath.Join(externalApp, "logs", "app.log"), []byte("ignored\n"))
	require.NoError(t, os.Symlink(externalApp, filepath.Join(root, "linked-app")))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "linked-logs"), 0o700))
	require.NoError(t, os.Symlink(
		filepath.Join(root, "alpha", "logs"),
		filepath.Join(root, "linked-logs", "logs"),
	))

	reader := newChunkReader(t, root, checkpointPath)
	chunk, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, chunk.Sources, 1)
	assert.Equal(t, "alpha/app.log", chunk.Sources[0].Source)
	assert.Equal(t, []byte("alpha\n"), chunk.Sources[0].Content)
	assert.Equal(t, len("alpha\n"), chunk.Bytes)
}

func TestChunkReaderDoesNotAdvanceBeforeCommit(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "data", "log-cursors.json")
	logPath := writeLog(t, root, "alpha", "app.log", []byte("first\n"))
	reader := newChunkReader(t, root, checkpointPath)

	first, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, first.Sources, 1)

	retry, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, retry.Sources, 1)
	assert.Equal(t, first.Sources[0].Content, retry.Sources[0].Content)
	assert.Equal(t, first.Sources[0].StartOffset, retry.Sources[0].StartOffset)

	require.NoError(t, reader.Commit(context.Background(), first))

	idle, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Zero(t, idle.Bytes)
	assert.Empty(t, idle.Sources)

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = file.WriteString("second\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	appended, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, appended.Sources, 1)
	assert.Equal(t, int64(len("first\n")), appended.Sources[0].StartOffset)
	assert.Equal(t, []byte("second\n"), appended.Sources[0].Content)

	info, err := os.Stat(checkpointPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestChunkReaderTailsOnlyLastSourceSliceOnFirstRead(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "log-cursors.json")
	content := bytes.Repeat([]byte("x"), domain.SOURCE_SLICE_BYTES+10)
	writeLog(t, root, "alpha", "large.log", content)
	reader := newChunkReader(t, root, checkpointPath)

	chunk, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, chunk.Sources, 1)
	assert.Equal(t, domain.SOURCE_SLICE_BYTES, chunk.Bytes)
	assert.Equal(t, int64(10), chunk.Sources[0].StartOffset)
	assert.Equal(t, int64(len(content)), chunk.Sources[0].EndOffset)
	assert.False(t, chunk.Backlog)
}

func TestChunkReaderCanCommitEmptyFileBeforeItGrows(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "log-cursors.json")
	logPath := writeLog(t, root, "alpha", "empty.log", nil)
	reader := newChunkReader(t, root, checkpointPath)

	idle, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Zero(t, idle.Bytes)
	require.NoError(t, reader.Commit(context.Background(), idle))

	content := bytes.Repeat([]byte("x"), domain.SOURCE_SLICE_BYTES+10)
	require.NoError(t, os.WriteFile(logPath, content, 0o600))

	chunk, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, len(content), chunk.Bytes)
	require.Len(t, chunk.Sources, 1)
	assert.Zero(t, chunk.Sources[0].StartOffset)
	assert.Equal(t, content, chunk.Sources[0].Content)
}

func TestChunkReaderCapsChunkAndContinuesFromNextSource(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "log-cursors.json")
	for i := range 17 {
		writeLog(
			t,
			root,
			"alpha",
			fmt.Sprintf("%02d.log", i),
			bytes.Repeat([]byte{byte('a' + i)}, domain.SOURCE_SLICE_BYTES),
		)
	}
	reader := newChunkReader(t, root, checkpointPath)

	first, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, domain.MAX_CHUNK_BYTES, first.Bytes)
	assert.True(t, first.Backlog)
	require.Len(t, first.Sources, 16)
	assert.Equal(t, "alpha/00.log", first.Sources[0].Source)
	assert.Equal(t, "alpha/15.log", first.Sources[15].Source)
	require.NoError(t, reader.Commit(context.Background(), first))

	second, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, domain.SOURCE_SLICE_BYTES, second.Bytes)
	require.Len(t, second.Sources, 1)
	assert.Equal(t, "alpha/16.log", second.Sources[0].Source)
	assert.False(t, second.Backlog)
}

func TestChunkReaderUsesFullLimitForOneHotSource(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "log-cursors.json")
	logPath := writeLog(t, root, "alpha", "hot.log", []byte("seed"))
	reader := newChunkReader(t, root, checkpointPath)

	seed, _, err := reader.Next(context.Background())
	require.NoError(t, err)
	require.NoError(t, reader.Commit(context.Background(), seed))

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = file.Write(bytes.Repeat([]byte("x"), domain.MAX_CHUNK_BYTES+10))
	require.NoError(t, err)
	require.NoError(t, file.Close())

	first, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, domain.MAX_CHUNK_BYTES, first.Bytes)
	assert.True(t, first.Backlog)
	require.Len(t, first.Sources, 1)
	assert.Equal(t, int64(len("seed")), first.Sources[0].StartOffset)
	require.NoError(t, reader.Commit(context.Background(), first))

	second, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, 10, second.Bytes)
	assert.False(t, second.Backlog)
}

func TestChunkReaderResetsAfterTruncateOrAnchorChange(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "log-cursors.json")
	logPath := writeLog(t, root, "alpha", "app.log", []byte("old-content\n"))
	reader := newChunkReader(t, root, checkpointPath)

	first, _, err := reader.Next(context.Background())
	require.NoError(t, err)
	require.NoError(t, reader.Commit(context.Background(), first))

	require.NoError(t, os.WriteFile(logPath, []byte("new\n"), 0o600))
	truncated, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, truncated.Sources, 1)
	assert.Zero(t, truncated.Sources[0].StartOffset)
	assert.Equal(t, []byte("new\n"), truncated.Sources[0].Content)
	require.NoError(t, reader.Commit(context.Background(), truncated))

	replacement := []byte("replacement-with-a-different-prefix\n")
	require.NoError(t, os.WriteFile(logPath, replacement, 0o600))
	replaced, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, replaced.Sources, 1)
	assert.Zero(t, replaced.Sources[0].StartOffset)
	assert.Equal(t, replacement, replaced.Sources[0].Content)
}

func TestChunkReaderFailsClosedOnMalformedCheckpoint(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "log-cursors.json")
	writeLog(t, root, "alpha", "app.log", []byte("content\n"))
	require.NoError(t, os.WriteFile(
		checkpointPath,
		[]byte(`{"version":1,"files":{},"unknown":true}`),
		0o600,
	))
	reader := newChunkReader(t, root, checkpointPath)

	_, _, err := reader.Next(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestChunkReaderHonorsCancellation(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "log-cursors.json")
	reader := newChunkReader(t, root, checkpointPath)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := reader.Next(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestChunkReaderRejectsForeignChunk(t *testing.T) {
	reader := newChunkReader(t, t.TempDir(), filepath.Join(t.TempDir(), "cursor.json"))
	err := reader.Commit(context.Background(), domain.Chunk{})
	require.Error(t, err)
}

func newChunkReader(t *testing.T, root, checkpointPath string) *domain.ChunkReader {
	t.Helper()
	reader, err := domain.NewChunkReader(root, checkpointPath)
	require.NoError(t, err)
	return reader
}

func writeLog(t *testing.T, root, app, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(root, app, "logs", name)
	writeFile(t, path, content)
	return path
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, content, 0o600))
}
