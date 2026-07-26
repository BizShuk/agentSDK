package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
)

// NAME_BASH is the registry name of the bash tool.
const NAME_BASH = "bash"

// BashArgs is the input shape the LLM sees for the bash tool.
type BashArgs struct {
	Command   string `json:"command"`
	Cwd       string `json:"cwd,omitempty"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// BashOutput is the output shape returned to the LLM.
type BashOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// BashDesc is the tool description sent to the LLM.
const BashDesc = "Run a bash shell command. Output includes stdout, stderr, and exit code. Command content is checked against the sandbox denylist."

// Bash runs shell commands. Output includes stdout, stderr, and exit code.
// Requires a sandbox Policy — registration fails without one.
type Bash struct {
	policy         tool.Sandbox
	rootDir        string
	defaultTimeout time.Duration
	executor       BashExecutor
}

// NewBash constructs a Bash tool. policy must be non-nil.
func NewBash(policy tool.Sandbox, rootDir string, opts ...BashOption) (*Bash, error) {
	if policy == nil {
		return nil, errPolicyRequired("bash")
	}
	b := &Bash{
		policy:         policy,
		rootDir:        rootDir,
		defaultTimeout: defaultBashTimeout(),
		executor:       &defaultExecutor{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}
	return b, nil
}

func defaultBashTimeout() time.Duration { return 30 * time.Second }

var _ tool.Tool = (*Bash)(nil)

// Name returns the tool's registry name.
func (b *Bash) Name() string { return NAME_BASH }

// Spec returns the tool's metadata and reflected argument schema.
func (b *Bash) Spec() core.ToolSpec {
	return tool.MustSchemaForTool[BashArgs](
		NAME_BASH,
		BashDesc,
		core.RISK_LEVEL_HIGH,
	)
}

// Call converts raw JSON arguments and executes the bash operation.
func (b *Bash) Call(
	ctx context.Context,
	raw json.RawMessage,
) (core.ToolResult, error) {
	return tool.CallWithRawMessage(ctx, b.Name(), raw, b.execute)
}

func (b *Bash) execute(ctx context.Context, a BashArgs) (BashOutput, error) {
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

	stdout, stderr, exitCode, err := b.executor.Run(ctx, a.Command, nil, wd)
	if err != nil {
		return BashOutput{}, fmt.Errorf("bash: execute %q: %w", a.Command, err)
	}

	return BashOutput{
		Stdout:   string(stdout),
		Stderr:   string(stderr),
		ExitCode: exitCode,
	}, nil
}
