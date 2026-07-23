package cmd

import (
	"context"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/hook"
)

// SAFETY_DENY and SAFETY_ASK are this application's permission rules —
// data, so they could equally live in a config file. They stay in code
// here only because code-agent has no config file of its own.
var (
	SAFETY_DENY = []string{"bash(sudo:*)"}
	SAFETY_ASK  = []string{"bash(git push:*)"}
)

// blockDestructiveBash is the counter-example to the rules above: a gate
// that inspects the actual argument, which no specifier pattern can do.
// That is what hooks are for, and why they arrive by injection rather
// than through the config.
//
// A blocked call folds into a failed ToolResult, so the model sees the
// refusal and can choose differently instead of the call silently
// disappearing.
func blockDestructiveBash() hook.Rule {
	return hook.Rule{
		Event: core.HOOK_PRE_TOOL_USE,
		Match: "bash",
		Handlers: []hook.Handler{hook.Func(func(_ context.Context, ev core.HookEvent) (core.HookDecision, error) {
			cmdStr, _ := ev.ToolCall.Args["command"].(string)
			if strings.Contains(cmdStr, "rm -rf /") {
				return core.HookDecision{Block: true, Reason: "refusing rm -rf on root-ish path"}, nil
			}
			return core.HookDecision{}, nil
		})},
	}
}
