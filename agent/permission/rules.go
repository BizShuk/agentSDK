package permission

import (
	"fmt"

	"github.com/bizshuk/agentsdk/core"
)

// RulesApprovalPolicy evaluates specifier rules against a tool call:
// deny > ask > allow precedence, mirroring claude-code rule precedence.
type RulesApprovalPolicy struct {
	Rules   []Rule
	Targets map[string]TargetFunc
}

// DecideMatch evaluates rules against a tool call. Returns the verdict and whether a rule matched.
func (p RulesApprovalPolicy) DecideMatch(call core.ToolCall) (core.ApprovalAction, bool) {
	target := extractTarget(p.Targets, call)
	for _, b := range []Behavior{BEHAVIOR_DENY, BEHAVIOR_ASK, BEHAVIOR_ALLOW} {
		for _, r := range p.Rules {
			if r.Behavior != b {
				continue
			}
			if MatchSpec(r.Spec, call.Name, target) {
				switch b {
				case BEHAVIOR_DENY:
					return core.APPROVAL_ACTION_DENY, true
				case BEHAVIOR_ASK:
					return core.APPROVAL_ACTION_ASK, true
				case BEHAVIOR_ALLOW:
					return core.APPROVAL_ACTION_ALLOW, true
				}
			}
		}
	}
	return 0, false
}

func extractTarget(targets map[string]TargetFunc, call core.ToolCall) string {
	if fn, ok := targets[call.Name]; ok && fn != nil {
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
