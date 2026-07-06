package action

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Sandbox is the policy side of tool safety: it decides whether a given
// ToolCall is allowed, denied, or needs explicit approval.
//
// The default implementation is policy-driven (Allow/Deny tables) and
// lives in this file. Production-grade sandboxes would back the policy
// with seccomp / AppArmor / eBPF — M3 lays the policy interface; M4+
// can layer OS-level enforcement.
type Sandbox interface {
	// Check returns the verdict for a (tool name, args) pair.
	// The default Check verifies path / command allowlists.
	Check(toolName string, args map[string]any) Verdict
}

// Verdict is the outcome of a sandbox check.
type Verdict int

const (
	// VERDICT_ALLOW — pass through to the tool.
	VERDICT_ALLOW Verdict = iota
	// VERDICT_DENY — refuse; surface a NOTIFY explaining why.
	VERDICT_DENY
)

// Policy is the default allow/deny sandbox. It supports:
//
//   - Path allowlist: any tool whose args contain a "path" key (or any
//     key listed in PathKeys) must point to a path under one of the
//     allowed prefixes. Absolute paths only — relative paths are denied.
//   - Command denylist: any tool whose args contain a "command" key
//     (or any key listed in CommandKeys) is checked against the deny
//     list of dangerous substrings.
type Policy struct {
	// AllowedPathPrefixes is the allowlist for path-bearing args.
	AllowedPathPrefixes []string
	// PathKeys is the list of arg keys that hold a filesystem path.
	// Default: ["path"].
	PathKeys []string
	// DeniedCommandSubstrings is the denylist for command-bearing args.
	// Default: ["rm -rf /", ":(){:|:&};:", "dd if=", "mkfs.", "shutdown"]
	DeniedCommandSubstrings []string
	// CommandKeys is the list of arg keys that hold a shell command.
	// Default: ["command", "cmd"].
	CommandKeys []string
}

// DefaultPolicy returns a Policy with sane denylists but no allowed
// prefixes — meaning all paths are denied until explicitly allowed.
func DefaultPolicy() *Policy {
	return &Policy{
		AllowedPathPrefixes: []string{
			"/tmp",
		},
		PathKeys: []string{"path"},
		DeniedCommandSubstrings: []string{
			"rm -rf /",
			":(){:|:&};:", // fork bomb
			"dd if=",
			"mkfs.",
			"shutdown",
			"reboot",
			"halt",
			"poweroff",
		},
		CommandKeys: []string{"command", "cmd"},
	}
}

// Check implements Sandbox.
func (p *Policy) Check(toolName string, args map[string]any) Verdict {
	for _, key := range p.PathKeys {
		if v, ok := args[key]; ok {
			s, ok := v.(string)
			if !ok {
				return VERDICT_DENY
			}
			if !p.pathAllowed(s) {
				return VERDICT_DENY
			}
		}
	}
	for _, key := range p.CommandKeys {
		if v, ok := args[key]; ok {
			s, ok := v.(string)
			if !ok {
				return VERDICT_DENY
			}
			if p.commandDenied(s) {
				return VERDICT_DENY
			}
		}
	}
	return VERDICT_ALLOW
}

// pathAllowed returns true if path is absolute and lands under one of
// the allowed prefixes. Symlinks are NOT resolved here — the tool that
// consumes the path is responsible for following them safely.
func (p *Policy) pathAllowed(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	for _, prefix := range p.AllowedPathPrefixes {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

// commandDenied returns true if cmd contains any denied substring.
func (p *Policy) commandDenied(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, bad := range p.DeniedCommandSubstrings {
		if strings.Contains(lower, bad) {
			return true
		}
	}
	return false
}

// String renders the verdict for tests / logs.
func (v Verdict) String() string {
	switch v {
	case VERDICT_ALLOW:
		return "ALLOW"
	case VERDICT_DENY:
		return "DENY"
	}
	return fmt.Sprintf("Verdict(%d)", int(v))
}