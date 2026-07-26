// Package permission implements core.ApprovalPolicy as two orthogonal,
// injectable axes learned from codex and claude-code:
//
//   - Mode — who approves by default (default / acceptEdits / plan / bypass),
//     the claude-code permission-mode axis.
//   - Rules — allow / ask / deny entries with tool specifiers such as
//     "Bash(git:*)" or "Edit(src/**)", evaluated deny > ask > allow.
//
// What a tool may touch (sandbox) stays a separate axis in action/sandbox.
// The package depends only on core; wiring replaces action.DefaultApprovalPolicy
// at the composition root:
//
//	engine.Approval = &permission.Engine{Mode: permission.MODE_DEFAULT, Rules: rules}
package permission

import (
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// Mode is the claude-code style permission mode. The set of valid
// values lives in core so spec (config vocabulary) and permission
// (runtime engine) cannot drift apart — a PermissionMode that is not
// one of core's PERMISSION_MODE_* constants is a configuration
// vocabulary error.
type Mode = core.PermissionMode

// MODE_* are preserved on this package for callers that already import
// agent/permission rather than core. The values are the untyped string
// literals declared in core, identical to core.PERMISSION_MODE_*.
const (
	MODE_DEFAULT      = core.PERMISSION_MODE_DEFAULT
	MODE_ACCEPT_EDITS = core.PERMISSION_MODE_ACCEPT_EDITS
	MODE_PLAN         = core.PERMISSION_MODE_PLAN
	MODE_BYPASS       = core.PERMISSION_MODE_BYPASS
)

// Behavior is what a matched rule yields.
type Behavior string

const (
	BEHAVIOR_ALLOW Behavior = "allow"
	BEHAVIOR_ASK   Behavior = "ask"
	BEHAVIOR_DENY  Behavior = "deny"
)

// Rule is one specifier entry, e.g. {BEHAVIOR_DENY, "Bash(rm:*)"}.
type Rule struct {
	Behavior Behavior
	Spec     string // "Tool", "Tool(pattern)", or "*"
}

// TargetFunc extracts the matchable target string from a tool call —
// e.g. the shell command for Bash or the path for file tools.
type TargetFunc func(call core.ToolCall) string

// Engine implements core.ApprovalPolicy.
type Engine struct {
	Mode  Mode
	Rules []Rule

	// Targets overrides target extraction per tool name. Default: the first
	// present of args "command", "path", "file_path" (stringly).
	Targets map[string]TargetFunc

	// Fallback decides when no rule matches under MODE_DEFAULT — inject
	// action.DefaultApprovalPolicy to keep the autonomy grid. nil falls back
	// to risk-only: low → allow, high → ask.
	Fallback core.ApprovalPolicy
}

// Decide implements core.ApprovalPolicy.
func (e *Engine) Decide(ctx struct{}, autonomy core.AutonomyLevel, eff core.CallToolInstruction, schema core.ToolSpec) core.ApprovalAction {
	if e.Mode == MODE_BYPASS {
		return core.APPROVAL_ACTION_ALLOW
	}
	if b, ok := e.matchRules(eff.Call); ok {
		switch b {
		case BEHAVIOR_DENY:
			return core.APPROVAL_ACTION_DENY
		case BEHAVIOR_ASK:
			return core.APPROVAL_ACTION_ASK
		case BEHAVIOR_ALLOW:
			return core.APPROVAL_ACTION_ALLOW
		}
	}
	switch e.Mode {
	case MODE_PLAN:
		if schema.Risk == core.RISK_LEVEL_LOW {
			return core.APPROVAL_ACTION_ALLOW
		}
		return core.APPROVAL_ACTION_DENY
	case MODE_ACCEPT_EDITS:
		if schema.Risk == core.RISK_LEVEL_LOW {
			return core.APPROVAL_ACTION_ALLOW
		}
		return core.APPROVAL_ACTION_ASK
	default:
		if e.Fallback != nil {
			return e.Fallback.Decide(ctx, autonomy, eff, schema)
		}
		if schema.Risk == core.RISK_LEVEL_LOW {
			return core.APPROVAL_ACTION_ALLOW
		}
		return core.APPROVAL_ACTION_ASK
	}
}

// matchRules evaluates deny > ask > allow: the strongest behavior with any
// matching specifier wins, mirroring claude-code rule precedence.
func (e *Engine) matchRules(call core.ToolCall) (Behavior, bool) {
	target := e.target(call)
	for _, b := range []Behavior{BEHAVIOR_DENY, BEHAVIOR_ASK, BEHAVIOR_ALLOW} {
		for _, r := range e.Rules {
			if r.Behavior != b {
				continue
			}
			if MatchSpec(r.Spec, call.Name, target) {
				return b, true
			}
		}
	}
	return "", false
}

func (e *Engine) target(call core.ToolCall) string {
	if fn, ok := e.Targets[call.Name]; ok && fn != nil {
		return fn(call)
	}
	for _, key := range []string{"command", "path", "file_path"} {
		if v, ok := call.Args[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

// MatchSpec matches one specifier against a (toolName, target) pair.
//
//	"*"              → every call
//	"Bash"           → every Bash call
//	"Bash(git:*)"    → Bash calls whose command starts with "git"
//	"Edit(src/**)"   → Edit calls whose path matches src/** (** crosses "/")
func MatchSpec(spec, toolName, target string) bool {
	spec = strings.TrimSpace(spec)
	if spec == "*" {
		return true
	}
	name, pattern, hasPattern := splitSpec(spec)
	if name != toolName {
		return false
	}
	if !hasPattern {
		return true
	}
	return MatchTarget(pattern, target)
}

// splitSpec splits "Tool(pattern)" into name + pattern.
func splitSpec(spec string) (name, pattern string, hasPattern bool) {
	open := strings.IndexByte(spec, '(')
	if open < 0 || !strings.HasSuffix(spec, ")") {
		return spec, "", false
	}
	return spec[:open], spec[open+1 : len(spec)-1], true
}

// MatchTarget matches a specifier pattern against a target string.
//
// Two pattern families:
//   - command prefixes — "git:*" matches "git", "git status", "git push";
//     everything before ":*" is compared word-wise against the target.
//   - path globs — "*" does not cross "/", "**" does; "src/**" matches every
//     path under src/. (filepath.Match's "*" never crosses "/", hence the
//     custom matcher.)
func MatchTarget(pattern, target string) bool {
	if pattern == "*" || pattern == "**" {
		return true
	}
	if prefix, ok := strings.CutSuffix(pattern, ":*"); ok {
		return target == prefix || strings.HasPrefix(target, prefix+" ")
	}
	return matchGlob(pattern, target)
}

// matchGlob is a "/"-aware glob: "*" matches within one path segment, "**"
// matches any number of segments (including none), "?" one non-"/" byte.
func matchGlob(pattern, target string) bool {
	return matchGlobFrom(pattern, target)
}

func matchGlobFrom(p, t string) bool {
	for len(p) > 0 {
		switch {
		case strings.HasPrefix(p, "**"):
			rest := strings.TrimPrefix(p, "**")
			rest = strings.TrimPrefix(rest, "/")
			if rest == "" {
				return true
			}
			for i := 0; i <= len(t); i++ {
				if (i == 0 || t[i-1] == '/') && matchGlobFrom(rest, t[i:]) {
					return true
				}
			}
			return false
		case p[0] == '*':
			for i := 0; i <= len(t); i++ {
				if matchGlobFrom(p[1:], t[i:]) {
					return true
				}
				if i < len(t) && t[i] == '/' {
					break
				}
			}
			return false
		case p[0] == '?':
			if len(t) == 0 || t[0] == '/' {
				return false
			}
			p, t = p[1:], t[1:]
		default:
			if len(t) == 0 || p[0] != t[0] {
				return false
			}
			p, t = p[1:], t[1:]
		}
	}
	return len(t) == 0
}
