// Package builtin ships built-in agent tools — Read, Write, Edit, Bash, Glob, Grep —
// that any agent can register as a complete set or an allowlist.
package builtin

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
)

// BuiltinNames lists every built-in tool name in a stable order.
func BuiltinNames() []string {
	return []string{NAME_READ, NAME_WRITE, NAME_EDIT, NAME_BASH, NAME_GLOB, NAME_GREP}
}

// Register constructs and registers the named built-in tools. An empty
// allowlist selects every built-in. Construction is all-or-nothing: invalid
// names or options leave reg unchanged.
func Register(reg *tool.Registry, allow []string, opts Options) error {
	if reg == nil {
		return errors.New("builtin.Register: registry is required")
	}
	if len(allow) == 0 {
		allow = BuiltinNames()
	}

	built := make([]core.Tool, 0, len(allow))
	var errs []string
	for _, name := range allow {
		t, err := newTool(name, opts)
		if err != nil {
			errs = append(errs, name+": "+err.Error())
			continue
		}
		built = append(built, t)
	}
	if len(errs) > 0 {
		return fmt.Errorf("builtin.Register: %s", strings.Join(errs, "; "))
	}
	for _, t := range built {
		reg.Register(t)
	}
	return nil
}

// RegisterDefaults registers every built-in tool.
func RegisterDefaults(reg *tool.Registry, opts Options) error {
	return Register(reg, nil, opts)
}

func newTool(name string, opts Options) (core.Tool, error) {
	switch name {
	case NAME_READ:
		return NewRead(opts.Policy, opts.WorkingDir, opts.ReadOpts...), nil
	case NAME_WRITE:
		return NewWrite(opts.Policy, opts.WorkingDir, opts.WriteOpts...)
	case NAME_EDIT:
		return NewEdit(opts.Policy, opts.WorkingDir, opts.EditOpts...)
	case NAME_BASH:
		return NewBash(opts.Policy, opts.WorkingDir, opts.BashOpts...)
	case NAME_GLOB:
		return NewGlob(opts.Policy, opts.WorkingDir, opts.GlobOpts...), nil
	case NAME_GREP:
		return NewGrep(opts.Policy, opts.WorkingDir, opts.GrepOpts...), nil
	default:
		return nil, fmt.Errorf("unknown built-in tool %q", name)
	}
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
