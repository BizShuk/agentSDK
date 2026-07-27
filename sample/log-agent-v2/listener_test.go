package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchListenerEmitsOneRedactedObservation(t *testing.T) {
	content := []byte("level=ERROR password=hunter2\n")
	batch := Batch{
		Parts: []LogPart{{
			Source:      "alpha/daemon.log",
			StartOffset: 10,
			EndOffset:   10 + int64(len(content)),
			Content:     content,
		}},
		Bytes:   len(content),
		Backlog: true,
	}
	observedAt := time.Unix(123, 456).UTC()

	listener, err := newBatchListener(batch, observedAt)
	require.NoError(t, err)
	observations := listener.Observations(context.Background())
	observation, ok := <-observations
	require.True(t, ok)
	_, open := <-observations
	assert.False(t, open)
	assert.Equal(t, OBSERVATION_SOURCE, observation.Source)
	assert.Equal(t, observedAt, observation.ObservedAt)

	payload, ok := observation.Payload.(string)
	require.True(t, ok, "payload type = %T", observation.Payload)
	for _, want := range []string{
		"raw_bytes=",
		"backlog=true",
		"<UNTRUSTED_LOG_DATA>",
		`source="alpha/daemon.log" offsets=10-`,
		"[REDACTED]",
		"</UNTRUSTED_LOG_DATA>",
	} {
		assert.Contains(t, payload, want)
	}
	assert.NotContains(t, payload, "hunter2")
}

func TestBatchListenerHonorsCancelledContext(t *testing.T) {
	content := []byte("error\n")
	listener, err := newBatchListener(Batch{
		Parts: []LogPart{{
			Source:    "alpha/daemon.log",
			EndOffset: int64(len(content)),
			Content:   content,
		}},
		Bytes: len(content),
	}, time.Now())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := <-listener.Observations(ctx); ok {
		t.Fatal("listener emitted after context cancellation")
	}
}

func TestBatchListenerRejectsInvalidBatch(t *testing.T) {
	cases := []struct {
		name  string
		batch Batch
	}{
		{name: "empty"},
		{
			name: "wrong byte count",
			batch: Batch{
				Parts: []LogPart{{
					Source:    "alpha/daemon.log",
					EndOffset: 1,
					Content:   []byte("x"),
				}},
				Bytes: 2,
			},
		},
		{
			name: "invalid offsets",
			batch: Batch{
				Parts: []LogPart{{
					Source:      "alpha/daemon.log",
					StartOffset: 5,
					EndOffset:   5,
					Content:     []byte("x"),
				}},
				Bytes: 1,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newBatchListener(tc.batch, time.Now())
			require.Error(t, err)
		})
	}
}
