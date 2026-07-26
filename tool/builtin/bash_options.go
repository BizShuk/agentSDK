package builtin

import "time"

// BashOption customizes a Bash tool instance.
type BashOption func(*Bash)

// WithBashDefaultTimeout sets the default command execution timeout.
func WithBashDefaultTimeout(d time.Duration) BashOption {
	return func(b *Bash) {
		if d > 0 {
			b.defaultTimeout = d
		}
	}
}

// WithBashExecutor overrides command execution (useful for testing).
func WithBashExecutor(exec BashExecutor) BashOption {
	return func(b *Bash) {
		if exec != nil {
			b.executor = exec
		}
	}
}
