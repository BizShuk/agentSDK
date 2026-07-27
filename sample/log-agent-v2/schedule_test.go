package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingReader struct {
	called   chan struct{}
	batch    Batch
	warnings []error
	err      error
}

func (r *recordingReader) Next(context.Context) (Batch, []error, error) {
	close(r.called)
	return r.batch, r.warnings, r.err
}

func TestReadScheduledBatchWaitsForTickBeforeReader(t *testing.T) {
	ticks := make(chan time.Time)
	reader := &recordingReader{
		called: make(chan struct{}),
		batch:  Batch{Bytes: 7},
	}
	type result struct {
		batch Batch
		err   error
	}
	results := make(chan result, 1)
	go func() {
		batch, _, err := readScheduledBatch(context.Background(), ticks, reader)
		results <- result{batch: batch, err: err}
	}()

	select {
	case <-reader.called:
		t.Fatal("reader ran before the schedule tick")
	default:
	}

	select {
	case ticks <- time.Unix(123, 0):
	case <-time.After(time.Second):
		t.Fatal("scheduler did not begin waiting for a tick")
	}

	select {
	case got := <-results:
		require.NoError(t, got.err)
		assert.Equal(t, 7, got.batch.Bytes)
	case <-time.After(time.Second):
		t.Fatal("scheduled read did not complete")
	}
}

func TestReadScheduledBatchHonorsCancellationBeforeRead(t *testing.T) {
	reader := &recordingReader{called: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := readScheduledBatch(ctx, make(chan time.Time), reader)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	select {
	case <-reader.called:
		t.Fatal("reader ran after cancellation")
	default:
	}
}

func TestReadScheduledBatchRejectsClosedTickChannel(t *testing.T) {
	ticks := make(chan time.Time)
	close(ticks)

	_, _, err := readScheduledBatch(
		context.Background(),
		ticks,
		&recordingReader{called: make(chan struct{})},
	)
	require.Error(t, err)
}
