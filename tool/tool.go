// Package tool ships built-in agent tools — Read, Write, Edit, Bash, Glob, Grep —
// that any agent can register via RegisterDefaults. Every tool wraps
// *action.TypedTool[TArgs, TOut] and delegates the core.Tool interface.
//
// # Integration
//
// Built-in tools are wired at the composition root, NOT auto-injected by
// the runtime. Callers compose their own Registry:
//
//	reg := action.NewRegistry()
//	tool.RegisterDefaults(reg, tool.Options{
//	    Policy:     action.DefaultPolicy(),
//	    WorkingDir: ".",
//	})
//
// # Sandbox
//
// Write, Edit, and Bash require a non-nil Policy. Read, Glob, and Grep
// accept a nil Policy (unrestricted reads within WorkingDir). All tools
// defensively re-check Policy.Check before mutating or executing.
//
// # Risk
//
// Read, Glob, Grep → RISK_LEVEL_LOW (read-only)
// Write, Edit, Bash → RISK_LEVEL_HIGH (mutate or execute)
// Risk drives the ApprovalGate middleware — at L1/L2, high-risk tools
// pause for HITL approval.
package tool

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
)

// Options configure the built-in tool set. Zero value means "safe defaults"
// — Read/Glob/Grep unrestricted, Write/Edit/Bash will error if Policy is nil.
type Options struct {
	// Policy is the sandbox consulted by every tool before accessing the
	// filesystem or executing a command. Required for Write/Edit/Bash;
	// optional for Read/Glob/Grep (nil means no path check).
	Policy action.Sandbox

	// WorkingDir is the base directory for tools that accept a --cwd or
	// relative path argument. Defaults to "." (process CWD).
	WorkingDir string

	Read  ReadOptions
	Write WriteOptions
	Edit  EditOptions
	Bash  BashOptions
	Glob  GlobOptions
	Grep  GrepOptions
}

// ReadOptions tunes the Read tool.
type ReadOptions struct {
	// MaxBytes caps the bytes read per call. 0 = 1 MiB.
	MaxBytes int64
}

// WriteOptions tunes the Write tool.
type WriteOptions struct {
	// DefaultMode is the file permission applied when creating a new file.
	// 0 means 0o644.
	DefaultMode int
}

// EditOptions tunes the Edit tool.
type EditOptions struct{}

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

// GlobOptions tunes the Glob tool.
type GlobOptions struct {
	// MaxMatches caps the returned list. 0 = 100.
	MaxMatches int
}

// GrepOptions tunes the Grep tool.
type GrepOptions struct {
	// MaxResults caps the returned matches. 0 = 100.
	MaxResults int
}

// RegisterDefaults constructs all 6 tools and registers them into reg.
// Returns the list of registered tools (useful for logging / telemetry).
// Errors if a required invariant is broken (e.g. Write without a Policy).
func RegisterDefaults(reg *action.Registry, opts Options) ([]core.Tool, error) {
	var errs []string
	tools := make([]core.Tool, 0, 6)

	// --- Read ---
	r := NewRead(opts.Read, opts.Policy, opts.WorkingDir)
	tools = append(tools, r)
	reg.Register(r)

	// --- Write ---
	w, err := NewWrite(opts.Write, opts.Policy, opts.WorkingDir)
	if err != nil {
		errs = append(errs, "write: "+err.Error())
	} else {
		tools = append(tools, w)
		reg.Register(w)
	}

	// --- Edit ---
	e, err := NewEdit(opts.Edit, opts.Policy, opts.WorkingDir)
	if err != nil {
		errs = append(errs, "edit: "+err.Error())
	} else {
		tools = append(tools, e)
		reg.Register(e)
	}

	// --- Bash ---
	b, err := NewBash(opts.Bash, opts.Policy, opts.WorkingDir)
	if err != nil {
		errs = append(errs, "bash: "+err.Error())
	} else {
		tools = append(tools, b)
		reg.Register(b)
	}

	// --- Glob ---
	g := NewGlob(opts.Glob, opts.Policy, opts.WorkingDir)
	tools = append(tools, g)
	reg.Register(g)

	// --- Grep ---
	gr := NewGrep(opts.Grep, opts.Policy, opts.WorkingDir)
	tools = append(tools, gr)
	reg.Register(gr)

	if len(errs) > 0 {
		return tools, fmt.Errorf("tool.RegisterDefaults: %s", strings.Join(errs, "; "))
	}
	return tools, nil
}

// MustPolicy panics if policy is nil — a convenience for callers that
// want to fail fast at startup rather than on first tool use.
func MustPolicy(policy action.Sandbox) action.Sandbox {
	if policy == nil {
		panic("tool: sandbox policy is required but nil")
	}
	return policy
}

// errPolicyRequired is used by Write/Edit/Bash constructors.
func errPolicyRequired(toolName string) error {
	return fmt.Errorf("%s requires a non-nil sandbox Policy", toolName)
}

func defaultMaxBytes() int64          { return 1 << 20 }  // 1 MiB
func defaultBashTimeout() time.Duration { return 30 * time.Second }
func defaultMaxMatches() int          { return 100 }

// checkPathArgs is a helper for sandbox re-check in tool handlers. It
// builds a map with "path" -> p and runs it through policy.Check. If the
// policy denies, the returned error message includes the reason.
// Sandbox is the type alias for action.Sandbox — used by tool constructors
// so callers don't need to import action directly.
type Sandbox = action.Sandbox

func checkPathArgs(policy Sandbox, toolName, path string) error {
	if policy == nil {
		return nil // no policy → no restriction
	}
	v := policy.Check(toolName, map[string]any{"path": path})
	if v == action.VERDICT_ALLOW {
		return nil
	}
	return fmt.Errorf("sandbox denied tool %q: path %q is not allowed", toolName, path)
}

func checkCommandArgs(policy Sandbox, toolName, cmd string) error {
	if policy == nil {
		return errors.New("bash requires a non-nil sandbox Policy (command denylist enforced)")
	}
	v := policy.Check(toolName, map[string]any{"command": cmd})
	if v == action.VERDICT_ALLOW {
		return nil
	}
	return fmt.Errorf("sandbox denied tool %q: command %q is not allowed", toolName, cmd)
}
