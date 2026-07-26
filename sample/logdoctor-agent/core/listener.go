// Package core holds the logdoctor sample's domain layer (listener / dedupe
// / todo). Naming collision with the agentsdk/core pure state-machine package
// is intentional and documented in plans/plan-only-and-plan-breezy-pike.md;
// the two never share an import path.
package core

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// LogFileListener reads a log file once and emits a single Percept that
// carries its full content. M1 does not tail; M2 adds incremental tailing.
type LogFileListener struct {
	path string
}

// NewLogFileListener constructs a listener bound to a path. Validation is
// performed at construction time so the runtime never sees a missing file.
func NewLogFileListener(path string) (*LogFileListener, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return &LogFileListener{path: path}, nil
}

// Name returns a stable identifier for diagnostics.
func (l *LogFileListener) Name() string { return "logfile:" + l.path }

// Percepts returns a channel that emits one Percept (the full file content)
// and then closes. M2 will turn this into a long-lived tailer.
func (l *LogFileListener) Observations(_ context.Context) <-chan core.Observation {
	ch := make(chan core.Observation, 1)
	go func() {
		defer close(ch)
		data, err := os.ReadFile(l.path)
		if err != nil {
			// Surface via a Percept with Error payload so the LLM can see it.
			ch <- core.Observation{
				ID:         "err",
				Source:     l.Name(),
				ObservedAt: time.Now().UTC(),
				Payload:    "read_error: " + err.Error(),
			}
			return
		}
		ch <- core.Observation{
			ID:         "head",
			Source:     l.Name(),
			ObservedAt: time.Now().UTC(),
			Payload:    strings.TrimSpace(string(data)),
		}
	}()
	return ch
}
