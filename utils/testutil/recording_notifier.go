package testutil

import (
	"context"
	"sync"
)

// RecordingNotifier records every message passed to Notify.
type RecordingNotifier struct {
	mu   sync.Mutex
	msgs []string
}

// Notify implements core.Notifier.
func (n *RecordingNotifier) Notify(_ context.Context, msg string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.msgs = append(n.msgs, msg)
	return nil
}

// Messages returns a copy of the captured messages.
func (n *RecordingNotifier) Messages() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, len(n.msgs))
	copy(out, n.msgs)
	return out
}
