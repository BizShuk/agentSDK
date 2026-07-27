package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sdkcore "github.com/bizshuk/agentsdk/core"
	sdkprovider "github.com/bizshuk/agentsdk/provider"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommandHasOneMinuteWatchDefaultAndNoProviderFlags(t *testing.T) {
	root := New()
	watch, _, err := root.Find([]string{"watch"})
	require.NoError(t, err)

	interval, err := watch.Flags().GetDuration("interval")
	require.NoError(t, err)
	assert.Equal(t, time.Minute, interval)
	assert.Nil(t, watch.Flags().Lookup("fixture"))
	assert.Nil(t, root.PersistentFlags().Lookup("fake"))
	assert.Nil(t, root.PersistentFlags().Lookup("provider"))
	assert.Nil(t, root.PersistentFlags().Lookup("max-turns"))
	assert.True(t, root.SilenceErrors)
	assert.True(t, root.SilenceUsage)
	assert.True(t, root.CompletionOptions.DisableDefaultCmd)
}

func TestNewMiniMaxProvider(t *testing.T) {
	t.Setenv("MINIMAX_API_KEY", "test-key")

	assert.Equal(t, "minimax", sdkprovider.DEFAULT_NAME)
	selected, err := newMiniMaxProvider()
	require.NoError(t, err)
	assert.NotNil(t, selected)
}

func TestRunWatchCycleWritesMarkdownEventsAndCommits(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "data", "log-cursors.json")
	writeWatchLog(t, root, "alpha", "app.log", []byte("ERROR disk full\n"))
	reader := newWatchReader(t, root, checkpointPath)
	model := &capturingProvider{
		result: sdkcore.ModelResult{
			StopReason: "end_turn",
			Text:       "# Diagnosis\n\nFree disk space.",
		},
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runWatchCycle(
		context.Background(),
		reader,
		model,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	assert.Equal(t, "# Diagnosis\n\nFree disk space.\n", stdout.String())
	assert.Equal(t, 1, model.requestCount())

	events := decodeStreamEvents(t, stderr.String())
	require.Len(t, events, 3)
	assert.Equal(t, sdkcore.STREAM_RUN_START, events[0].Kind)
	assert.Equal(t, sdkcore.STREAM_MESSAGE, events[1].Kind)
	assert.Equal(t, sdkcore.STREAM_RUN_END, events[2].Kind)

	idle, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Zero(t, idle.Bytes)
}

func TestWatchLoopRunsImmediatelyAndCommitsIdleDiscovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "data", "log-cursors.json")
	writeWatchLog(t, root, "alpha", "empty.log", nil)
	reader := newWatchReader(t, root, checkpointPath)
	model := &capturingProvider{
		result: sdkcore.ModelResult{StopReason: "end_turn", Text: "unused"},
	}

	err := watchLoop(
		context.Background(),
		reader,
		model,
		&bytes.Buffer{},
		&bytes.Buffer{},
		make(chan time.Time),
		1,
	)
	require.NoError(t, err)
	assert.Zero(t, model.requestCount())

	info, err := os.Stat(checkpointPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWatchLoopRetriesFailedChunkOnNextTick(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "data", "log-cursors.json")
	writeWatchLog(t, root, "alpha", "app.log", []byte("ERROR retry me\n"))
	reader := newWatchReader(t, root, checkpointPath)
	sentinel := errors.New("temporary provider failure")
	model := &failOnceProvider{
		firstErr: sentinel,
		result: sdkcore.ModelResult{
			StopReason: "end_turn",
			Text:       "# Diagnosis\n\nRetry succeeded.",
		},
	}
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := watchLoop(
		context.Background(),
		reader,
		model,
		&stdout,
		&stderr,
		ticks,
		2,
	)
	require.NoError(t, err)
	assert.Equal(t, "# Diagnosis\n\nRetry succeeded.\n", stdout.String())
	assert.Contains(t, stderr.String(), "watch cycle 1 failed")
	assert.Contains(t, stderr.String(), sentinel.Error())

	requests := model.allRequests()
	require.Len(t, requests, 2)
	require.Len(t, requests[0].Messages, 2)
	require.Len(t, requests[1].Messages, 2)
	assert.Equal(
		t,
		messageText(requests[0].Messages[1]),
		messageText(requests[1].Messages[1]),
	)

	idle, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Zero(t, idle.Bytes)
}

func TestRunWatchCycleDoesNotCommitWhenStdoutFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	checkpointPath := filepath.Join(t.TempDir(), "data", "log-cursors.json")
	writeWatchLog(t, root, "alpha", "app.log", []byte("ERROR keep pending\n"))
	reader := newWatchReader(t, root, checkpointPath)
	model := &capturingProvider{
		result: sdkcore.ModelResult{
			StopReason: "end_turn",
			Text:       "# Diagnosis\n\nPending.",
		},
	}
	sentinel := errors.New("stdout closed")

	err := runWatchCycle(
		context.Background(),
		reader,
		model,
		failingWriter{err: sentinel},
		&bytes.Buffer{},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)

	pending, warnings, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, len("ERROR keep pending\n"), pending.Bytes)
}

func TestWatchLoopHonorsCancellation(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config")
	reader := newWatchReader(
		t,
		root,
		filepath.Join(t.TempDir(), "log-cursors.json"),
	)
	model := &capturingProvider{
		result: sdkcore.ModelResult{StopReason: "end_turn", Text: "unused"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := watchLoop(
		ctx,
		reader,
		model,
		&bytes.Buffer{},
		&bytes.Buffer{},
		make(chan time.Time),
		0,
	)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, model.requestCount())
}

func newWatchReader(
	t *testing.T,
	root string,
	checkpointPath string,
) *domain.ChunkReader {
	t.Helper()
	reader, err := domain.NewChunkReader(root, checkpointPath)
	require.NoError(t, err)
	return reader
}

func writeWatchLog(
	t *testing.T,
	root string,
	appName string,
	fileName string,
	content []byte,
) string {
	t.Helper()
	logPath := filepath.Join(root, appName, "logs", fileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o700))
	require.NoError(t, os.WriteFile(logPath, content, 0o600))
	return logPath
}

type failOnceProvider struct {
	mu       sync.Mutex
	firstErr error
	result   sdkcore.ModelResult
	requests []sdkcore.ModelRequest
}

func (p *failOnceProvider) Generate(
	ctx context.Context,
	request sdkcore.ModelRequest,
) (sdkcore.ModelResult, error) {
	if err := ctx.Err(); err != nil {
		return sdkcore.ModelResult{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, request)
	if len(p.requests) == 1 {
		return sdkcore.ModelResult{}, p.firstErr
	}
	return p.result, nil
}

func (p *failOnceProvider) allRequests() []sdkcore.ModelRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]sdkcore.ModelRequest(nil), p.requests...)
}
