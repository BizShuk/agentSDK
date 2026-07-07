package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
)

// BashArgs is the input shape the LLM sees for the bash tool.
type BashArgs struct {
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
	Cwd       string `json:"cwd,omitempty"`
}

// BashOutput is the output shape returned to the LLM.
type BashOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
}

// Executor abstracts subprocess execution so tests can inject a fake.
type Executor interface {
	Run(ctx context.Context, cmd string, env []string, cwd string) (stdout, stderr []byte, exitCode int, err error)
}

// Bash executes a shell command via /bin/sh -c. Commands are checked
// against the sandbox denylist before execution. Output is capped.
type Bash struct {
	Inner          *action.TypedTool[BashArgs, BashOutput]
	policy         Sandbox
	rootDir        string
	defaultTimeout time.Duration
	maxOutputBytes int64
	executor       Executor
	env            []string
}

// NewBash constructs a Bash tool. policy must be non-nil.
func NewBash(opts BashOptions, policy Sandbox, rootDir string) (*Bash, error) {
	if policy == nil {
		return nil, errPolicyRequired("bash")
	}
	if opts.DefaultTimeout <= 0 {
		opts.DefaultTimeout = defaultBashTimeout()
	}
	if opts.MaxOutputBytes <= 0 {
		opts.MaxOutputBytes = defaultMaxBytes()
	}
	exec := opts.Executor
	if exec == nil {
		exec = &realExecutor{}
	}
	b := &Bash{
		policy:         policy,
		rootDir:        rootDir,
		defaultTimeout: opts.DefaultTimeout,
		maxOutputBytes: opts.MaxOutputBytes,
		executor:       exec,
		env:            opts.Env,
	}
	b.Inner = action.NewTypedTool("bash",
		"Execute a shell command via /bin/sh -c. Commands are checked against the sandbox denylist. Use absolute paths. Output is capped at 1 MiB. Non-zero exit codes are returned as output (not errors). Timeout defaults to 30 seconds.",
		b.handle)
	b.Inner.SetRisk(sdkcore.RISK_LEVEL_HIGH)
	return b, nil
}

// SetRisk changes the risk level. Call before Register.
func (b *Bash) SetRisk(level sdkcore.RiskLevel) { b.Inner.SetRisk(level) }

// --- core.Tool interface ---

func (b *Bash) Name() string                       { return b.Inner.Name() }
func (b *Bash) Description() string                { return b.Inner.Description() }
func (b *Bash) Schema() sdkcore.ToolSpec         { return b.Inner.Schema() }
func (b *Bash) Risk() sdkcore.RiskLevel            { return b.Inner.Risk() }
func (b *Bash) Call(ctx context.Context, raw json.RawMessage) (sdkcore.ToolResult, error) {
	return b.Inner.Call(ctx, raw)
}

func (b *Bash) handle(ctx context.Context, a BashArgs) (BashOutput, error) {
	// Sandbox check on command content.
	if err := checkCommandArgs(b.policy, "bash", a.Command); err != nil {
		return BashOutput{}, err
	}

	// Resolve CWD.
	wd, err := safeCwd(b.rootDir)
	if err != nil {
		return BashOutput{}, err
	}
	if a.Cwd != "" {
		cwd, cwdErr := resolveCwd(b.policy, "bash", a.Cwd, wd)
		if cwdErr != nil {
			return BashOutput{}, fmt.Errorf("bash: invalid cwd: %w", cwdErr)
		}
		wd = cwd
	}

	// Determine timeout.
	timeout := b.defaultTimeout
	if a.TimeoutMs > 0 {
		timeout = time.Duration(a.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	stdout, stderr, exitCode, runErr := b.executor.Run(ctx, a.Command, b.env, wd)
	dur := time.Since(start)

	if runErr != nil {
		return BashOutput{
			Stdout:   string(stdout),
			Stderr:   string(stderr),
			ExitCode: exitCode,
			Duration: dur.String(),
		}, fmt.Errorf("bash: %w", runErr)
	}

	// Cap output.
	outStr := string(stdout)
	errStr := string(stderr)
	if int64(len(outStr)) > b.maxOutputBytes {
		outStr = outStr[:b.maxOutputBytes] + "\n... [stdout truncated]"
	}
	if int64(len(errStr)) > b.maxOutputBytes {
		errStr = errStr[:b.maxOutputBytes] + "\n... [stderr truncated]"
	}

	return BashOutput{
		Stdout:   outStr,
		Stderr:   errStr,
		ExitCode: exitCode,
		Duration: dur.String(),
	}, nil
}

// --- realExecutor ---

type realExecutor struct{}

// Run implements Executor using os/exec.
func (realExecutor) Run(ctx context.Context, command string, env []string, cwd string) (stdout, stderr []byte, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if env != nil {
		cmd.Env = env
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout = outBuf.Bytes()
	stderr = errBuf.Bytes()

	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
			// Non-zero exit is not a tool error — surface in exit_code.
			return stdout, stderr, exitCode, nil
		}
		// ctx deadline, binary not found, etc.
		return stdout, stderr, -1, runErr
	}
	return stdout, stderr, 0, nil
}

// limitWriter stops writing after n bytes. Used internally to cap
// subprocess output without buffering the full stream.
type limitWriter struct {
	w   *bytes.Buffer
	n   int64
	max int64
}

func (lw *limitWriter) Write(p []byte) (int, error) {
	if lw.n >= lw.max {
		return len(p), nil // discard silently
	}
	remain := lw.max - lw.n
	if int64(len(p)) > remain {
		p = p[:remain]
	}
	n, _ := lw.w.Write(p)
	lw.n += int64(n)
	return len(p), nil // never report short write — caller must not error
}

// String returns the buffered content, with a truncation notice if needed.
func (lw *limitWriter) String() string {
	s := lw.w.String()
	if lw.n >= lw.max {
		s += "\n... [truncated]"
	}
	return s
}



