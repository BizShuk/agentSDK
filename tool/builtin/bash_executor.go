package builtin

import (
	"bytes"
	"context"
	"os/exec"
)

// BashExecutor abstracts shell command execution for testing.
type BashExecutor interface {
	Run(ctx context.Context, command string, env []string, cwd string) (stdout []byte, stderr []byte, exitCode int, err error)
}

// defaultExecutor runs commands via /bin/sh -c.
type defaultExecutor struct{}

func (e *defaultExecutor) Run(ctx context.Context, command string, env []string, cwd string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if len(env) > 0 {
		cmd.Env = env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			err = nil // Process completed with non-zero exit code — not an execution failure.
		}
	}
	return stdout.Bytes(), stderr.Bytes(), exitCode, err
}
