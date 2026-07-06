package core

// EffectKind discriminates what side-effect a runtime dispatcher must execute.
type EffectKind string

const (
	// EFFECT_CALL_MODEL — invoke the model with the carried request.
	EFFECT_CALL_MODEL EffectKind = "call_model"
	// EFFECT_CALL_TOOL — invoke a registered tool.
	EFFECT_CALL_TOOL EffectKind = "call_tool"
	// EFFECT_REQUEST_APPROVAL — append a PendingApproval and pause the run.
	EFFECT_REQUEST_APPROVAL EffectKind = "request_approval"
	// EFFECT_NOTIFY — emit a notification via the bound Notifier.
	EFFECT_NOTIFY EffectKind = "notify"
	// EFFECT_CHECKPOINT — persist current State (called automatically before dispatch).
	EFFECT_CHECKPOINT EffectKind = "checkpoint"
	// EFFECT_EMIT — push an external Envelope (CLI JSONL, websocket, etc.).
	EFFECT_EMIT EffectKind = "emit"
	// EFFECT_DONE — terminal: no further work; run completes.
	EFFECT_DONE EffectKind = "done"
)

// CallModelEffect is dispatched to ModelProvider.Generate / Stream.
type CallModelEffect struct {
	RequestID string  `json:"request_id"`
	Messages  []Message `json:"messages"`
	Tools     []ToolSchema `json:"tools,omitempty"`
	MaxTokens int     `json:"max_tokens,omitempty"`
}

// CallToolEffect is dispatched to the tool Registry.
type CallToolEffect struct {
	Call ToolCall `json:"call"`
}

// RequestApprovalEffect carries the context a human approver needs.
type RequestApprovalEffect struct {
	ApprovalID string   `json:"approval_id"`
	Reason     string   `json:"reason"`
	Risk       RiskLevel `json:"risk"`
	Summary    string   `json:"summary"`
	ToolCall   *ToolCall `json:"tool_call,omitempty"`
}

// NotifyEffect is dispatched to Notifier.Notify.
type NotifyEffect struct {
	Level   string `json:"level"` // info | warn | error
	Message string `json:"message"`
}

// CheckpointEffect is dispatched to StateStore.Save.
type CheckpointEffect struct {
	Reason string `json:"reason"`
}

// EmitEffect is dispatched to runtime.Emit so callers (CLI / websocket) see it.
type EmitEffect struct {
	Envelope any `json:"envelope"`
}

// Effect is a tagged union — exactly one pointer is non-nil for any Kind.
//
// Decoders in JSON-land can rehydrate using the Kind field as a discriminator.
// In-process dispatch is a type switch on the active pointer.
type Effect struct {
	Kind            EffectKind          `json:"kind"`
	CallModel       *CallModelEffect    `json:"call_model,omitempty"`
	CallTool        *CallToolEffect     `json:"call_tool,omitempty"`
	RequestApproval *RequestApprovalEffect `json:"request_approval,omitempty"`
	Notify          *NotifyEffect       `json:"notify,omitempty"`
	Checkpoint      *CheckpointEffect   `json:"checkpoint,omitempty"`
	Emit            *EmitEffect         `json:"emit,omitempty"`
}
