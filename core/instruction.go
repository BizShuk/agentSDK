package core

// InstructionKind discriminates which field of Instruction carries the
// payload. (Plotkin & Power 2003, "Algebraic Operations and Generic
// Effects"; the agent-loop community overloads "effect" with provider
// side-effect, so we rename to Instruction.)
type InstructionKind string

const (
	// INSTRUCTION_CALL_MODEL — invoke the model with the carried request.
	INSTRUCTION_CALL_MODEL InstructionKind = "call_model"
	// INSTRUCTION_CALL_TOOL — invoke a registered tool.
	INSTRUCTION_CALL_TOOL InstructionKind = "call_tool"
	// INSTRUCTION_REQUEST_APPROVAL — append a PendingApproval and pause the run.
	INSTRUCTION_REQUEST_APPROVAL InstructionKind = "request_approval"
	// INSTRUCTION_NOTIFY — emit a notification via the bound Notifier.
	INSTRUCTION_NOTIFY InstructionKind = "notify"
	// INSTRUCTION_CHECKPOINT — persist current State (called automatically before dispatch).
	INSTRUCTION_CHECKPOINT InstructionKind = "checkpoint"
	// INSTRUCTION_EMIT — push an external Envelope (CLI JSONL, websocket, etc.).
	INSTRUCTION_EMIT InstructionKind = "emit"
	// INSTRUCTION_DONE — terminal: no further work; run completes.
	INSTRUCTION_DONE InstructionKind = "done"
)

// CallToolInstruction is dispatched to the tool Registry.
type CallToolInstruction struct {
	Call ToolCall `json:"call"`
}

// RequestApprovalInstruction carries the context a human approver needs.
type RequestApprovalInstruction struct {
	ApprovalID string    `json:"approval_id"`
	Reason     string    `json:"reason"`
	Risk       RiskLevel `json:"risk"`
	Summary    string    `json:"summary"`
	ToolCall   *ToolCall `json:"tool_call,omitempty"`
}

// NotifyInstruction is dispatched to Notifier.Notify.
type NotifyInstruction struct {
	Level   string `json:"level"` // info | warn | error
	Message string `json:"message"`
}

// CheckpointInstruction is dispatched to StateStore.Save.
type CheckpointInstruction struct {
	Reason string `json:"reason"`
}

// EmitInstruction is dispatched to runtime.Emitter so callers (CLI / websocket) see it.
type EmitInstruction struct {
	Envelope any `json:"envelope"`
}

// Instruction is a tagged union — exactly one pointer is non-nil for any Kind.
//
// Decoders in JSON-land can rehydrate using the Kind field as a discriminator.
// In-process dispatch is a type switch on the active pointer.
type Instruction struct {
	Kind            InstructionKind             `json:"kind"`
	CallModel       *ModelRequest               `json:"call_model,omitempty"`
	CallTool        *CallToolInstruction        `json:"call_tool,omitempty"`
	RequestApproval *RequestApprovalInstruction `json:"request_approval,omitempty"`
	Notify          *NotifyInstruction          `json:"notify,omitempty"`
	Checkpoint      *CheckpointInstruction      `json:"checkpoint,omitempty"`
	Emit            *EmitInstruction            `json:"emit,omitempty"`
}
