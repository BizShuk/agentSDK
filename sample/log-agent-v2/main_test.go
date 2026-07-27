package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/utils/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogAgentConfigUsesMiniMaxOneShot(t *testing.T) {
	config := logAgentConfig()

	assert.Equal(t, APP_NAME, config.Name)
	assert.Equal(t, spec.TIER_ONESHOT, config.Tier)
	assert.Equal(t, MINIMAX_PROVIDER_NAME, config.Model.Provider)
	assert.NotEmpty(t, config.Persona)
}

func TestNewLogAgentSendsListenerBatchInFirstRequest(t *testing.T) {
	content := []byte("level=ERROR component=worker message=boom\n")
	batch := Batch{
		Parts: []LogPart{{
			Source:    "worker/daemon.log",
			EndOffset: int64(len(content)),
			Content:   content,
		}},
		Bytes: len(content),
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink, err := newOutputSink(&stdout, &stderr)
	require.NoError(t, err)

	model := testutil.NewScriptedProvider()
	model.EnqueueEndTurn("# Diagnosis\nRestart the worker after verification.")
	runner, err := newLogAgent(
		batch,
		time.Unix(123, 456),
		sink,
		agent.WithProvider(model),
	)
	require.NoError(t, err)

	host := &agent.Host{
		DataDir: t.TempDir(),
		RunID:   "logs-integration-1",
		Logger: slog.New(
			slog.NewTextHandler(io.Discard, nil),
		),
	}
	require.NoError(t, agent.Run(context.Background(), runner, host))
	require.NoError(t, sink.Err())
	assert.Equal(t, 1, model.RequestCount())
	assert.Equal(
		t,
		"# Diagnosis\nRestart the worker after verification.\n",
		stdout.String(),
	)

	var requestText strings.Builder
	request := model.LastRequest()
	for _, message := range request.Messages {
		for _, part := range message.Parts {
			requestText.WriteString(part.Text)
		}
	}
	assert.Contains(t, requestText.String(), "<UNTRUSTED_LOG_DATA>")
	assert.Contains(t, requestText.String(), "message=boom")
	assert.Contains(t, stderr.String(), `"kind":"run_start"`)
	assert.Contains(t, stderr.String(), `"run_id":"logs-integration-1"`)
	assert.Contains(t, stderr.String(), `"kind":"run_end"`)
}

func TestWatchLoopRetriesFailedBatchBeforeCommit(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "worker", "logs", "daemon.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	require.NoError(t, os.WriteFile(logPath, []byte("level=ERROR boom\n"), 0o600))

	reader, err := NewReader(root, filepath.Join(t.TempDir(), "cursor.json"))
	require.NoError(t, err)
	ticks := make(chan time.Time, 2)
	ticks <- time.Unix(1, 0)
	ticks <- time.Unix(2, 0)
	close(ticks)

	var stderr bytes.Buffer
	var contents []string
	var runIDs []string
	attempt := 0
	analyze := func(
		_ context.Context,
		batch Batch,
		runID string,
		_ time.Time,
	) error {
		attempt++
		runIDs = append(runIDs, runID)
		contents = append(contents, string(batch.Parts[0].Content))
		if attempt == 1 {
			return errors.New("temporary provider failure")
		}
		return nil
	}

	err = watchLoop(
		context.Background(),
		reader,
		ticks,
		&stderr,
		analyze,
	)
	require.ErrorIs(t, err, errScheduleClosed)
	require.Len(t, contents, 2)
	assert.Equal(t, contents[0], contents[1])
	require.Len(t, runIDs, 2)
	assert.NotEqual(t, runIDs[0], runIDs[1])
	assert.Contains(t, stderr.String(), "watch cycle 1 analysis failed")

	next, _, err := reader.Next(context.Background())
	require.NoError(t, err)
	assert.Zero(t, next.Bytes)
}

func TestWatchLoopRunsOneAnalysisAtATime(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "worker", "logs", "daemon.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	require.NoError(t, os.WriteFile(logPath, []byte("first\n"), 0o600))

	reader, err := NewReader(root, filepath.Join(t.TempDir(), "cursor.json"))
	require.NoError(t, err)
	ticks := make(chan time.Time, 2)
	ticks <- time.Unix(1, 0)
	ticks <- time.Unix(2, 0)
	close(ticks)

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	analyze := func(
		_ context.Context,
		_ Batch,
		_ string,
		_ time.Time,
	) error {
		select {
		case <-firstStarted:
			close(secondStarted)
		default:
			close(firstStarted)
			if err := os.WriteFile(
				logPath,
				[]byte("first\nsecond\n"),
				0o600,
			); err != nil {
				return err
			}
			<-releaseFirst
		}
		return nil
	}

	result := make(chan error, 1)
	go func() {
		result <- watchLoop(
			context.Background(),
			reader,
			ticks,
			io.Discard,
			analyze,
		)
	}()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first analysis did not run")
	}
	select {
	case <-secondStarted:
		t.Fatal("second analysis overlapped the first")
	default:
	}
	close(releaseFirst)

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second analysis did not run")
	}
	require.ErrorIs(t, <-result, errScheduleClosed)
}

func TestWatchLoopHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader, err := NewReader(t.TempDir(), filepath.Join(t.TempDir(), "cursor.json"))
	require.NoError(t, err)

	err = watchLoop(
		ctx,
		reader,
		make(chan time.Time),
		io.Discard,
		func(context.Context, Batch, string, time.Time) error {
			t.Fatal("analysis ran after cancellation")
			return nil
		},
	)
	assert.ErrorIs(t, err, context.Canceled)
}
