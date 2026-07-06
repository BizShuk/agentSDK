package testutil

import (
	"context"
	"sync"
)

// CapturingNotifier records every message passed to Notify.
type CapturingNotifier struct {
	mu   sync.Mutex
	msgs []string
}

// Notify implements core.Notifier.
func (n *CapturingNotifier) Notify(_ context.Context, msg string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.msgs = append(n.msgs, msg)
	return nil
}

// Messages returns a copy of the captured messages.
func (n *CapturingNotifier) Messages() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, len(n.msgs))
	copy(out, n.msgs)
	return out
}