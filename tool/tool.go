// Package tool ships built-in agent tools — Read, Write, Edit, Bash, Glob, Grep —
// that any agent can register via RegisterDefaults.
package tool

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/action"
)

// BuiltinNames lists every built-in tool name in a stable order.
func BuiltinNames() []string {
	return []string{NAME_READ, NAME_WRITE, NAME_EDIT, NAME_BASH, NAME_GLOB, NAME_GREP}
}

// RegisterDefaults constructs all 6 built-in tools and registers them into reg via RegisterFunc.
// Errors if a required invariant is broken (e.g. Write without a Policy).
func RegisterDefaults(reg *action.Registry, opts Options) error {
	var errs []string

	// --- Read ---
	r := NewRead(opts.Read, opts.Policy, opts.WorkingDir)
	action.RegisterFunc(reg, r.ToolName(), ReadDesc, r.ToolRisk(), r.Handle)

	// --- Write ---
	w, err := NewWrite(opts.Write, opts.Policy, opts.WorkingDir)
	if err != nil {
		errs = append(errs, "write: "+err.Error())
	} else {
		action.RegisterFunc(reg, w.ToolName(), WriteDesc, w.ToolRisk(), w.Handle)
	}

	// --- Edit ---
	e, err := NewEdit(opts.Edit, opts.Policy, opts.WorkingDir)
	if err != nil {
		errs = append(errs, "edit: "+err.Error())
	} else {
		action.RegisterFunc(reg, e.ToolName(), EditDesc, e.ToolRisk(), e.Handle)
	}

	// --- Bash ---
	b, err := NewBash(opts.Bash, opts.Policy, opts.WorkingDir)
	if err != nil {
		errs = append(errs, "bash: "+err.Error())
	} else {
		action.RegisterFunc(reg, b.ToolName(), BashDesc, b.ToolRisk(), b.Handle)
	}

	// --- Glob ---
	g := NewGlob(opts.Glob, opts.Policy, opts.WorkingDir)
	action.RegisterFunc(reg, g.ToolName(), GlobDesc, g.ToolRisk(), g.Handle)

	// --- Grep ---
	gr := NewGrep(opts.Grep, opts.Policy, opts.WorkingDir)
	action.RegisterFunc(reg, gr.ToolName(), GrepDesc, gr.ToolRisk(), gr.Handle)

	if len(errs) > 0 {
		return fmt.Errorf("tool.RegisterDefaults: %s", strings.Join(errs, "; "))
	}
	return nil
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
