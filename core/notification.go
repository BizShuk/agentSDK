package core

import "context"

// Notifier mirrors gosdk/notify.Notifier exactly so the gosdk Multi, Stdout,
// and Slack notifiers are structurally usable without an adapter.
type Notifier interface {
	Notify(ctx context.Context, message string) error
}
