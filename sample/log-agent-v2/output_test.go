package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputSinkRoutesAnalysisAndEvents(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink, err := newOutputSink(&stdout, &stderr)
	require.NoError(t, err)

	events := []core.StreamEvent{
		{Kind: core.STREAM_RUN_START, RunID: "run-1"},
		{
			Kind:  core.STREAM_MESSAGE,
			RunID: "run-1",
			Turn:  1,
			Text:  "# Analysis\nCheck the listener.",
		},
		{
			Kind:   core.STREAM_RUN_END,
			RunID:  "run-1",
			Status: core.RUN_STATUS_COMPLETED,
		},
	}
	for _, event := range events {
		sink.OnStreamEvent(event)
	}

	require.NoError(t, sink.Err())
	assert.Equal(t, "# Analysis\nCheck the listener.\n", stdout.String())

	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	require.Len(t, lines, len(events))
	for i, line := range lines {
		want, err := json.Marshal(events[i])
		require.NoError(t, err)
		assert.JSONEq(t, string(want), line)
	}
}

func TestOutputSinkReportsWriterFailures(t *testing.T) {
	writeErr := errors.New("writer failed")
	cases := []struct {
		name   string
		stdout io.Writer
		stderr io.Writer
		event  core.StreamEvent
	}{
		{
			name:   "analysis",
			stdout: failingWriter{err: writeErr},
			stderr: &bytes.Buffer{},
			event: core.StreamEvent{
				Kind: core.STREAM_MESSAGE,
				Text: "diagnosis",
			},
		},
		{
			name:   "event",
			stdout: &bytes.Buffer{},
			stderr: failingWriter{err: writeErr},
			event:  core.StreamEvent{Kind: core.STREAM_RUN_START},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink, err := newOutputSink(tc.stdout, tc.stderr)
			require.NoError(t, err)
			sink.OnStreamEvent(tc.event)
			require.Error(t, sink.Err())
			assert.ErrorIs(t, sink.Err(), writeErr)
		})
	}
}

func TestNewOutputSinkRejectsNilWriters(t *testing.T) {
	cases := []struct {
		name   string
		stdout io.Writer
		stderr io.Writer
	}{
		{name: "stdout", stderr: &bytes.Buffer{}},
		{name: "stderr", stdout: &bytes.Buffer{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newOutputSink(tc.stdout, tc.stderr)
			require.Error(t, err)
		})
	}
}

func TestOutputSinkRejectsEmptyAnalysis(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink, err := newOutputSink(&stdout, &stderr)
	require.NoError(t, err)

	sink.OnStreamEvent(core.StreamEvent{
		Kind: core.STREAM_RUN_START,
	})
	sink.OnStreamEvent(core.StreamEvent{
		Kind: core.STREAM_MESSAGE,
		Text: " \n ",
	})
	sink.OnStreamEvent(core.StreamEvent{
		Kind: core.STREAM_RUN_END,
	})

	require.Error(t, sink.Err())
	assert.ErrorContains(t, sink.Err(), "analysis output is empty")
	assert.Empty(t, stdout.String())
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}
