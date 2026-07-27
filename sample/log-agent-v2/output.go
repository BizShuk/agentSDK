package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// outputSink keeps human-readable analysis separate from machine events.
type outputSink struct {
	stdout        io.Writer
	events        *json.Encoder
	wroteAnalysis bool
	err           error
}

var _ core.EventSink = (*outputSink)(nil)

func newOutputSink(stdout, stderr io.Writer) (*outputSink, error) {
	if stdout == nil {
		return nil, fmt.Errorf("output sink stdout must not be nil")
	}
	if stderr == nil {
		return nil, fmt.Errorf("output sink stderr must not be nil")
	}
	return &outputSink{
		stdout: stdout,
		events: json.NewEncoder(stderr),
	}, nil
}

func (s *outputSink) OnStreamEvent(event core.StreamEvent) {
	if s.err != nil {
		return
	}
	if err := s.events.Encode(event); err != nil {
		s.err = fmt.Errorf("write stream event: %w", err)
		return
	}
	if event.Kind != core.STREAM_MESSAGE ||
		strings.TrimSpace(event.Text) == "" {
		return
	}
	if err := writeMessage(s.stdout, event.Text); err != nil {
		s.err = fmt.Errorf("write analysis: %w", err)
		return
	}
	s.wroteAnalysis = true
}

func (s *outputSink) Err() error {
	if s.err == nil && !s.wroteAnalysis {
		return fmt.Errorf("analysis output is empty")
	}
	return s.err
}

func writeMessage(writer io.Writer, text string) error {
	if _, err := io.WriteString(writer, text); err != nil {
		return err
	}
	if strings.HasSuffix(text, "\n") {
		return nil
	}
	_, err := io.WriteString(writer, "\n")
	return err
}
