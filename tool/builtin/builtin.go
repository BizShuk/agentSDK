// Package builtin ships built-in agent tools — Read, Write, Edit, Bash, Glob, Grep —
// that any agent can register via RegisterDefaults.
package builtin

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/tool"
)

// BuiltinNames lists every built-in tool name in a stable order.
func BuiltinNames() []string {
	return []string{NAME_READ, NAME_WRITE, NAME_EDIT, NAME_BASH, NAME_GLOB, NAME_GREP}
}

// RegisterDefaults constructs all 6 built-in tools and registers them into reg via RegisterFunc.
// Errors if a required invariant is broken (e.g. Write without a Policy).
func RegisterDefaults(reg *tool.Registry, opts Options) error {
	var errs []string

	// --- Read ---
	r := NewRead(opts.Policy, opts.WorkingDir, opts.ReadOpts...)
	tool.RegisterTyped(reg, r)

	// --- Write ---
	w, err := NewWrite(opts.Policy, opts.WorkingDir, opts.WriteOpts...)
	if err != nil {
		errs = append(errs, "write: "+err.Error())
	} else {
		tool.RegisterTyped(reg, w)
	}

	// --- Edit ---
	e, err := NewEdit(opts.Policy, opts.WorkingDir, opts.EditOpts...)
	if err != nil {
		errs = append(errs, "edit: "+err.Error())
	} else {
		tool.RegisterTyped(reg, e)
	}

	// --- Bash ---
	b, err := NewBash(opts.Policy, opts.WorkingDir, opts.BashOpts...)
	if err != nil {
		errs = append(errs, "bash: "+err.Error())
	} else {
		tool.RegisterTyped(reg, b)
	}

	// --- Glob ---
	g := NewGlob(opts.Policy, opts.WorkingDir, opts.GlobOpts...)
	tool.RegisterTyped(reg, g)

	// --- Grep ---
	gr := NewGrep(opts.Policy, opts.WorkingDir, opts.GrepOpts...)
	tool.RegisterTyped(reg, gr)

	if len(errs) > 0 {
		return fmt.Errorf("builtin.RegisterDefaults: %s", strings.Join(errs, "; "))
	}
	return nil
}

// MustPolicy panics if policy is nil — a convenience for callers that
// want to fail fast at startup rather than on first tool use.
func MustPolicy(policy tool.Sandbox) tool.Sandbox {
	if policy == nil {
		panic("builtin: sandbox policy is required but nil")
	}
	return policy
}

// errPolicyRequired is used by Write/Edit/Bash constructors.
func errPolicyRequired(toolName string) error {
	return fmt.Errorf("%s requires a non-nil sandbox Policy", toolName)
}

func defaultMaxBytes() int64 { return 1 << 20 } // 1 MiB
func defaultMaxMatches() int { return 100 }

func checkPathArgs(policy tool.Sandbox, toolName, path string) error {
	if policy == nil {
		return nil // no policy → no restriction
	}
	v := policy.Check(toolName, map[string]any{"path": path})
	if v == tool.VERDICT_ALLOW {
		return nil
	}
	return fmt.Errorf("sandbox denied tool %q: path %q is not allowed", toolName, path)
}

func checkCommandArgs(policy tool.Sandbox, toolName, cmd string) error {
	if policy == nil {
		return errors.New("bash requires a non-nil sandbox Policy (command denylist enforced)")
	}
	v := policy.Check(toolName, map[string]any{"command": cmd})
	if v == tool.VERDICT_ALLOW {
		return nil
	}
	return fmt.Errorf("sandbox denied tool %q: command %q is not allowed", toolName, cmd)
}
