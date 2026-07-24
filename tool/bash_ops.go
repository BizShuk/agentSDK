package tool

import "time"

// BashOptions tunes the Bash tool.
type BashOptions struct {
	// DefaultTimeout caps command execution. 0 = 30 s.
	DefaultTimeout time.Duration

	// MaxOutputBytes caps the combined stdout + stderr. 0 = 1 MiB.
	MaxOutputBytes int64

	// Executor runs the command. nil = real os/exec implementation.
	Executor Executor

	// Env is the environment passed to the subprocess. nil = os.Environ().
	Env []string
}
