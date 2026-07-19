package core

import "context"

// HookEventName identifies one lifecycle point the runtime fires hooks at.
// Names follow the claude-code hook vocabulary so external hook scripts can
// be reused across harnesses.
type HookEventName string

const (
	HOOK_PRE_TOOL_USE       HookEventName = "PreToolUse"
	HOOK_POST_TOOL_USE      HookEventName = "PostToolUse"
	HOOK_USER_PROMPT_SUBMIT HookEventName = "UserPromptSubmit"
	HOOK_STOP               HookEventName = "Stop"
	HOOK_SESSION_START      HookEventName = "SessionStart"
	HOOK_SESSION_END        HookEventName = "SessionEnd"
	HOOK_PRE_COMPACT        HookEventName = "PreCompact"
	HOOK_NOTIFICATION       HookEventName = "Notification"
)

// HookEvent is the payload delivered to hook handlers. Exactly the fields
// relevant to the event name are set: tool events carry ToolName/ToolCall
// (and ToolResult for PostToolUse); prompt events carry Prompt.
type HookEvent struct {
	Name       HookEventName  `json:"name"`
	RunID      string         `json:"run_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	ToolCall   *ToolCall      `json:"tool_call,omitempty"`
	ToolResult *ToolResult    `json:"tool_result,omitempty"`
	Prompt     string         `json:"prompt,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

// HookDecision is the merged verdict of every handler that matched an event.
// Zero value = proceed unchanged. Block is only honored on gate events
// (PreToolUse, UserPromptSubmit); SystemNote is surfaced back into the run
// as context the model can see.
type HookDecision struct {
	Block       bool           `json:"block,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	SystemNote  string         `json:"system_note,omitempty"`
	ReplaceArgs map[string]any `json:"replace_args,omitempty"`
}

// Hooks is the lifecycle-hook port consumed by runtime.Engine. nil = no
// hooks. The default implementation lives in hook.Runner; the engine treats
// a Fire error as a hook infrastructure failure, not a block.
type Hooks interface {
	Fire(ctx context.Context, ev HookEvent) (HookDecision, error)
}
