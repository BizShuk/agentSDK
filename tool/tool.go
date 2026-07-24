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

func defaultMaxBytes() int64 { return 1 << 20 } // 1 MiB
func defaultBashTimeout() time.Duration { return 30 * time.Second }
func defaultMaxMatches() int { return 100 }

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
