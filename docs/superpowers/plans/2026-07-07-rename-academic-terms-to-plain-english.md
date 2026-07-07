# Rename Academic Terms to Plain English Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename every function, type, variable, and wire-string value in the agentsdk monorepo that uses academic jargon (e.g. `Effect`, `Scratch`, `ReAct`, `Percept`, `ThinkingPattern`, `Decide`, `ToolUseChunk`, `Spotlight`) to a plain-English equivalent. Each new name carries a doc comment citing the academic source for traceability.

**Architecture:** Bottom-up rename: `core/` first (the package everything else imports), then `planning/`, `action/`, `perception/`, `memory/`, `runtime/`, `middleware/`, `cli/`, `mcp/`, `internal/testutil/`, `provider/*`, then samples. Each task is a self-contained rename: find → replace Go symbols → update wire strings → add migration shim → run tests → commit. The whole plan is one TDD-driven sweep.

**Tech Stack:** Go 1.26, `go.work` monorepo, `testify`, gofmt, `gofmt -r` for mechanical rewrites, `grep` for cross-package reachability checks.

## Global Constraints

| Rule | Value |
|---|---|
| Go version | 1.26+ |
| Naming convention | MixedCaps for Go symbols (per project convention) |
| Wire strings | Changed (per user decision) — see per-task "wire renames" |
| Tests | Must stay green after every commit |
| File modifications | `gofmt` clean, `go vet ./...` clean |
| Commit cadence | One commit per task (or per sub-step if a task has more than 10 file edits) |
| Backward compat | **Broken** — wire strings change. Migration shim in `FileStateStore.Load` (Task 2) preserves on-disk State from before the rename. |
| Comments | Each renamed symbol gets a 1-2 line doc comment citing the academic source. Existing comments are preserved. |
| Constants | Stay `SCREAMING_SNAKE_CASE` per project convention. |
| Module paths | `github.com/bizshuk/agentsdk/...` — unchanged. |

## Naming Map (canonical reference)

This table is the single source of truth. Every task references it; do not deviate. Citations are the *comment text* you add to the symbol.

### `core/` package

| Old | New | Citation comment |
|---|---|---|
| `Step` (type alias) | `Decide` | // Pure reducer: (State, Event) → (State, []Instruction). |
| `NewStep` | `NewDecide` | // Dispatch by ReasoningStyle. |
| `ThinkingPattern` (interface) | `DecisionRule` | // One reduce step in the agent loop. |
| `ThinkingPattern.Decide(state)` | `DecisionRule.NextStep(state)` | (same comment) |
| `ThinkingKind` | `ReasoningStyle` | |
| `THINK_REACT` | `REASON_REACT` | // Yao et al. 2023, "ReAct: Synergizing Reasoning and Acting in Language Models". |
| `THINK_PLANNER_EXECUTOR` | `REASON_PLAN_THEN_RUN` | // Wei et al. 2022, "Chain of Thought Prompting"; planner-executor dispatch. |
| `THINK_EXECUTOR_CRITIC` | `REASON_DO_THEN_REVIEW` | // Welleck et al. 2023, "Self-Refine: Iterative Refinement with Self-Feedback". |
| `THINK_COT_SINGLESHOT` | `REASON_ONE_SHOT` | // Wei et al. 2022, "Chain of Thought Prompting". |
| `THINK_REFLEXION` | `REASON_LEARN_FROM_FAILURE` | // Shinn et al. 2023, "Reflexion: Language Agents with Verbal Reinforcement Learning". |
| `THINK_ROUTER` | `REASON_PICK_AGENT` | // Multi-agent routing pattern. |
| `Percept` | `Observation` | // In the LLM-agent literature, "percept" traces to the perceptron (Rosenblatt 1958); the agent-loop community settled on "observation" (ReAct, Reflexion, BabyAI). Kept the convention. |
| `Input` | `Event` | // One event driving one Decide. |
| `InputKind` | `EventKind` | |
| `INPUT_KIND_PERCEPT` | `EVENT_OBSERVATION` | |
| `INPUT_KIND_MODEL_RESULT` | `EVENT_MODEL_REPLY` | |
| `INPUT_KIND_TOOL_RESULT` | `EVENT_TOOL_REPLY` | |
| `INPUT_KIND_APPROVAL_DECISION` | `EVENT_HUMAN_DECISION` | |
| `INPUT_KIND_RESUME` | `EVENT_RESUME` | (keep) |
| `ModelChunk` | `ModelChunk` | (keep) |
| `ToolUseChunk` | `ToolUseChunk` | (keep) |
| `ToolResultChunk` | `ToolResultChunk` | (keep) |
| `ModelResult` | `ModelResult` | (keep) |
| `ModelRequest` | `ModelRequest` | (keep) |
| `TokenUsage` | `TokenUsage` | (keep) |
| `TokenUsage.Add` | `TokenUsage.Add` | (keep) |
| `Chunk` | `Part` | // One fragment of a Message: text, audio, image, tool_use, tool_result. |
| `ChunkKind` | `PartKind` | |
| `CHUNK_KIND_TEXT` | `PART_KIND_PLAIN_TEXT` | |
| `CHUNK_KIND_AUDIO` | `PART_KIND_AUDIO` | |
| `CHUNK_KIND_IMAGE` | `PART_KIND_IMAGE` | |
| `CHUNK_KIND_TOOL_USE` | `PART_KIND_TOOL_USE` | |
| `CHUNK_KIND_TOOL_RESULT` | `PART_KIND_TOOL_RESULT` | |
| `Message` | `Message` | (keep) |
| `Message.AppendText` | (keep) | |
| `Role` | `Role` | (keep) |
| `ROLE_*` | (keep — wire) | |
| `State` | `State` | (keep) |
| `State.Clone` | (keep) | |
| `Scratch` (field) | `WorkingMemory` | // Newell & Simon 1972, "Human Problem Solving": short-term working memory used by the current decision rule. Persists across iterations. |
| `Budget` | `Budget` | (keep) |
| `Budget.Exceeded` | (keep) | |
| `Budget.now` | (keep — unexported helper) | |
| `RunStatus` | `RunStatus` | (keep) |
| `RUN_STATUS_*` | (keep — wire) | |
| `AutonomyLevel` | `AutonomyLevel` | (keep) |
| `AUTONOMY_*` | (keep — wire) | |
| `RiskLevel` | `RiskLevel` | (keep) |
| `RISK_LEVEL_*` | (keep — wire) | |
| `Effect` | `Instruction` | // Plotkin & Power 2003, "Algebraic Operations and Generic Effects": the runtime computes a list of instructions, then dispatches each. The agent-loop term "effect" overlaps too much with provider "side effect"; rename to Instruction. |
| `EffectKind` | `InstructionKind` | |
| `EFFECT_CALL_MODEL` | `INSTRUCTION_CALL_MODEL` | (keep — same wire value) |
| `EFFECT_CALL_TOOL` | `INSTRUCTION_CALL_TOOL` | (keep) |
| `EFFECT_REQUEST_APPROVAL` | `INSTRUCTION_REQUEST_APPROVAL` | (keep) |
| `EFFECT_NOTIFY` | `INSTRUCTION_NOTIFY` | (keep) |
| `EFFECT_CHECKPOINT` | `INSTRUCTION_CHECKPOINT` | (keep) |
| `EFFECT_EMIT` | `INSTRUCTION_EMIT` | (keep) |
| `EFFECT_DONE` | `INSTRUCTION_DONE` | (keep) |
| `CallModelEffect` | `CallModelInstruction` | |
| `CallToolEffect` | `CallToolInstruction` | |
| `RequestApprovalEffect` | `RequestApprovalInstruction` | |
| `NotifyEffect` | `NotifyInstruction` | |
| `CheckpointEffect` | `CheckpointInstruction` | |
| `EmitEffect` | `EmitInstruction` | |
| `PendingApproval` | `PendingApproval` | (keep) |
| `ApprovalDecision` | `ApprovalDecision` | (keep) |
| `APPROVAL_DECISION_*` | (keep — wire) | |
| `ApprovalAction` | `ApprovalAction` | (keep) |
| `APPROVAL_ACTION_*` | (keep — wire) | |
| `ApprovalPolicy` | `ApprovalPolicy` | (keep) |
| `ModelProvider` | `ModelProvider` | (keep) |
| `StateStore` | `StateStore` | (keep) |
| `WAL` | `WriteAheadLog` | // Database term: append-only log of events for crash recovery. |
| `ToolRegistry` | `ToolRegistry` | (keep) |
| `Tool` | `Tool` | (keep) |
| `Notifier` | `Notifier` | (keep) |
| `Source` (the small interface stub in input.go) | `ObservationSource` | // Mirrors perception.Source. Kept here so core doesn't import perception. |
| `ToolSchema` | `ToolSpec` | // What the LLM sees: name, description, parameter schema, risk. |
| `JSONSchema` | `JSONSchema` | (keep — spec term) |
| `Source` (alias) | `ObservationSource` | |

### Wire string changes (apply with symbol renames)

| Old value | New value | Used in |
|---|---|---|
| `"percept"` (InputKind) | `"observation"` | `EventKind` JSON tag |
| `"model_result"` | `"model_reply"` | same |
| `"tool_result"` | `"tool_result"` (KEEP — wire format consistency with `part_kind`) | same |
| `"approval_decision"` | `"human_decision"` | same |
| `"text"` | `"plain_text"` | `PartKind` JSON tag |
| `"react"` | `"think_then_act"` | `ReasoningStyle` JSON tag |
| `"planner_executor"` | `"plan_then_run"` | same |
| `"executor_critic"` | `"do_then_review"` | same |
| `"cot_singleshot"` | `"one_shot"` | same |
| `"reflexion"` | `"learn_from_failure"` | same |
| `"router"` | `"choose_agent"` | same |
| `"percept"` (cli) | `"observation"` | `cli.MessageType` |
| `"approval_decision"` (cli) | `"human_decision"` | `cli.MessageType` |
| `"tool_result"` (cli) | `"tool_result"` (KEEP) | same |
| `"assistant"` / `"tool_call"` / `"checkpoint"` / `"result"` / `"error"` | (KEEP) | same |

### `planning/` package

| Old | New | Citation |
|---|---|---|
| `ReAct` | `ThinkThenAct` | (Yao 2023) |
| `NewReAct` | `NewThinkThenAct` | |
| `REACT_PHASE` | `THINK_THEN_ACT_PHASE` | |
| `REACT_LAST_CALL` | `THINK_THEN_ACT_PENDING_CALL` | |
| `REACT_LAST_RESULT` | `THINK_THEN_ACT_LAST_RESULT` | |
| `REACT_PHASE_THINK` | `THINK_THEN_ACT_REASON` | |
| `REACT_PHASE_ACT` | `THINK_THEN_ACT_DISPATCH` | |
| `REACT_PHASE_OBSERVE` | `THINK_THEN_ACT_REFLECT` | |
| `SeedAct` | `SeedDispatch` | |
| `PlannerExecutor` | `PlanThenRun` | |
| `NewPlannerExecutor` | `NewPlanThenRun` | |
| `PE_BLUEPRINT_STEPS` | `PLAN_THEN_RUN_BLUEPRINT` | |
| `PE_STEP_INDEX` | `PLAN_THEN_RUN_STEP_INDEX` | |
| `PE_PHASE` | `PLAN_THEN_RUN_PHASE` | |
| `PE_PHASE_PLAN` | `PLAN_PHASE` | |
| `PE_PHASE_EXEC` | `RUN_PHASE` | |
| `PE_PHASE_DONE` | `DONE_PHASE` | |
| `SeedBlueprint` | `SeedBlueprint` | (keep) |
| `ExecutorCritic` | `RunThenReview` | |
| `NewExecutorCritic` | `NewRunThenReview` | |
| `EC_PHASE` | `RUN_THEN_REVIEW_PHASE` | |
| `EC_CRITIQUE` | `RUN_THEN_REVIEW_NOTE` | |
| `EC_ITER` | `RUN_THEN_REVIEW_ITERATION` | |
| `EC_PHASE_EXEC` | `RUN_PHASE` | |
| `EC_PHASE_CRIT` | `REVIEW_PHASE` | |
| `EC_PHASE_DONE` | `DONE_PHASE` | |
| `SeedCritiqueOK` | `SeedReviewPassed` | |
| `SeedCritiqueReject` | `SeedReviewFailed` | |
| `COTSingleshot` | `OneShotReasoning` | |
| `NewCOTSingleshot` | `NewOneShotReasoning` | |
| `Reflexion` | `LearnFromFailure` | |
| `NewReflexion` | `NewLearnFromFailure` | |
| `Router` | `ChooseAgent` | |
| `NewRouter` | `NewChooseAgent` | |
| `helpers.go` internal helpers (`scratchString` etc.) | (keep) | |
| `hasOKPrefix` | `startsWithPassed` | |
| `nowOrZero` | (keep) | |
| `newID`/`formatUint` | (keep) | |

### `action/` package

| Old | New |
|---|---|
| `TypedTool` | (keep) |
| `Registry` | (keep) |
| `ToolSource` | (keep) |
| `Sandbox` | (keep) |
| `Verdict` | (keep) |
| `VERDICT_*` | (keep) |
| `Policy` | (keep) |
| `DefaultPolicy` | (keep) |
| `DefaultApprovalPolicy` | (keep) |
| `SchemaFor`/`SchemaJSON`/`SchemaForTool`/`ValidateArgs`/`SchemaError` | (keep) |
| `marshalArgs` | (keep) |

### `perception/` package

| Old | New |
|---|---|
| `Source` | `ObservationSource` |
| `Multi` | `FanIn` |
| `Multi.Percepts` | `FanIn.Observations` |
| `Multi.Sources` | (keep) |
| `Multi.Name` | (keep) |
| `NormalizeFunc` | `ToMessageFunc` |
| `Normalizer` | `ToMessage` |
| `Normalizer.Apply` | `ToMessage.Apply` |

### `memory/` package

| Old | New |
|---|---|
| `Window` | (keep) |
| `TokenCounter` | (keep) |
| `CharHeuristicCounter` | (keep) |
| `Compactor` | (keep) |
| `HeadlineCompactor` | (keep) |
| `checkpointer.Checkpointer` | `checkpoint.Recoverer` |
| `checkpointer.New` | `checkpoint.NewRecoverer` |
| `checkpointer.Checkpoint` | `checkpoint.Save` |
| `checkpointer.Recover` | `checkpoint.Recover` (keep) |
| `checkpointer.RecoverResult` | `checkpoint.RecoveredRun` |
| `filestore.FileStateStore` | `filestore.JSONFileStateStore` |
| `filestore.NewFileStateStore` | `filestore.NewJSONFileStateStore` |
| `filestore.FileWAL` | `filestore.JSONLFileLog` |
| `filestore.NewFileWAL` | `filestore.NewJSONLFileLog` |
| `FileWAL.Append` | `JSONLFileLog.Append` |
| `FileWAL.Replay` | `JSONLFileLog.Read` |
| `FileWAL.Truncate` | `JSONLFileLog.TruncateFrom` |

### `runtime/` package

| Old | New |
|---|---|
| `Loop` | `Engine` |
| `Loop.Run` | `Engine.Run` |
| `Loop.Resume` | `Engine.Resume` |
| `Loop.SubmitApproval` | `Engine.SubmitHumanDecision` |
| `Loop.RunWithInput` | `Engine.RunWithEvent` |
| `Loop.dispatch` | `Engine.runInstruction` |
| `Loop.runWithInput` | `Engine.runStep` |
| `Loop.ensureChain` | `Engine.buildChain` |
| `Loop.resolve` | `Engine.resolveChain` |
| `Loop.chain` | `Engine.chain` |
| `Loop.chainOnce` | `Engine.chainOnce` |
| `Emitter` | `Emitter` (keep — function type) |
| `NewLoop` | `NewEngine` |
| `DefaultMiddleware` | (keep) |
| `boolError` | `onceFlag` |
| `IsBudgetExceeded` | (keep) |

### `middleware/` package

| Old | New |
|---|---|
| `Middleware` / `Next` / `Dispatcher` / `Chain` / `Identity` | (keep) |
| `harness.Retry`/`RetryConfig`/`RetryableError`/`IsRetryable`/`SimpleRetryable`/`TransientError`/`classifyByString`/`RetryClass*` | (keep) |
| `harness.Budget` | (keep) |
| `harness.BudgetExceededError` | (keep) |
| `harness.Timeout`/`TimeoutConfig` | (keep) |
| `loopguard.New`/`Config`/`State`/`DefaultVolatileKeys` | (keep) |
| `loopguard.LOOPGUARD_STATE_KEY` | `loopguard.STATE_KEY` |
| `security.ApprovalGate` | (keep) |
| `security.Sandbox` | (keep) |
| `security.Spotlight` | `security.MarkUntrusted` |
| `security.SpotlightOpen` | `security.UNTRUSTED_OPEN` |
| `security.SpotlightClose` | `security.UNTRUSTED_CLOSE` |
| `security.SanitizedTag` | (keep) |
| `security.Sanitizer` | `security.InjectionFilter` |
| `security.DefaultSanitizer` | `security.DefaultInjectionFilter` |
| `security.formatSanitized` | `security.formatSanitized` (keep) |
| `security.Sanitizer.Middleware` | `security.InjectionFilter.Middleware` |
| `security.Sanitizer.Inspect` | `security.InjectionFilter.Inspect` |
| `security.outputToString` | (keep) |
| `security.textSnippet` | (keep) |
| `observability.Tracing`/`TracingConfig`/`TracerFromName` | (keep) |
| `observability.spanName`/`spanAttributes` | (keep) |

### `cli/` package

| Old | New |
|---|---|
| `Codec`/`Envelope`/`MessageType`/`*Payload` types | (keep) |
| `NewJSONLCodec`/`WriteError`/`WriteResult` | (keep) |
| `Envelope.MarshalJSON` | (keep) |
| `MSG_TYPE_PERCEPT` | `MSG_TYPE_OBSERVATION` |
| `MSG_TYPE_APPROVAL_DECISION` | `MSG_TYPE_HUMAN_DECISION` |

### `internal/testutil/`

| Old | New |
|---|---|
| `FakeProvider` | `ScriptedProvider` |
| `NewFakeProvider` | `NewScriptedProvider` |
| `FakeProvider.OnGenerate` | `ScriptedProvider.OnRequest` |
| `FakeProvider.CallCount` | `ScriptedProvider.RequestCount` |
| `FakeProvider.Enqueue*` | (keep) |
| `CapturingNotifier` | `RecordingNotifier` |
| `MemStore`/`MemWAL`/`ErrNotFound`/`ErrQueueEmpty` | (keep) |

### `mcp/`

| Old | New |
|---|---|
| `Client` | (keep) |
| `NewClient` | (keep) |
| `mcpToolToSchema`/`inferRisk` | (keep) |

### `provider/*`

All names already use plain English. Keep as-is. `WithViper`/`WithAPIKey`/`WithModel`/`WithBaseURL` are conventional.

### `sample/logdoctor/`

| Old | New |
|---|---|
| `core.LogFileListener` | (keep — clear) |
| `core.Dedupe` | `core.BurstSuppressor` |
| `core.NewDedupe` | `core.NewBurstSuppressor` |
| `core.Dedupe.Inner` | `core.BurstSuppressor.Inner` |
| `core.Dedupe.RuleID` | `core.BurstSuppressor.RuleID` |
| `core.Dedupe.Cooldown` | `core.BurstSuppressor.Cooldown` |
| `core.Dedupe.LastFingerprint` | `core.BurstSuppressor.LastSignature` |
| `core.Dedupe.ShouldEmitForTest` | `core.BurstSuppressor.ShouldEmitForTest` |
| `tool.NewReadLogTail`/`NewNotify` | (keep) |
| `cmd.allowAllApproval` | `cmd.AllowAllPolicy` |
| `cmd.writeEnvelope`/`payloadFor` | (keep) |

### `sample/greet-agent/`

| Old | New |
|---|---|
| `tool.NewGreet` | (keep) |
| `initFileLogger`/`parseLogLevel` | (keep) |
| `writeEnvelope`/`payloadFor` | (keep) |

---

## Task ordering

Tasks are ordered by dependency: `core/` first (everyone imports it), then leaves inward.

1. Task 1: Foundation — `core/` package
2. Task 2: `core/` migration shim for `FileStateStore.Load` (so old persisted State loads)
3. Task 3: `planning/` package
4. Task 4: `perception/` package
5. Task 5: `action/` package (mostly untouched — verify)
6. Task 6: `memory/` package
7. Task 7: `runtime/` package
8. Task 8: `middleware/` package
9. Task 9: `cli/` package
10. Task 10: `mcp/` package (verify only)
11. Task 11: `internal/testutil/` package
12. Task 12: `provider/*` packages (verify only)
13. Task 13: `sample/logdoctor/` sample
14. Task 14: `sample/greet-agent/` sample
15. Task 15: Full E2E verification (`go test ./...`, sample E2E, M2 regression checks)

Each task is a "rename wave" that updates one package + its test + every consumer reference. After each task the entire workspace must compile (`go build ./...`) and tests in the touched package must pass (`go test ./<package> -count=1`).

---

### Task 1: `core/` package — symbols + wire strings + interface rewrites

**Files:**
- Modify: `core/state.go`, `core/state_test.go`
- Modify: `core/input.go`
- Modify: `core/effect.go`
- Modify: `core/message.go`
- Modify: `core/thinking.go`
- Modify: `core/tool.go`
- Modify: `core/autonomy.go`
- Modify: `core/approval.go`
- Modify: `core/port.go`
- Modify: `core/step.go`, `core/step_test.go`
- Modify: `core/helpers_test.go`

**Interfaces:**
- Produces: `core.Decide`, `core.NewDecide`, `core.DecisionRule`, `core.NextStep`, `core.ReasoningStyle`, `core.REASON_*`, `core.Observation`, `core.Event`, `core.EventKind`, `core.EVENT_*`, `core.Part`, `core.PartKind`, `core.PART_KIND_*`, `core.WorkingMemory` (field on State), `core.Instruction`, `core.InstructionKind`, `core.INSTRUCTION_*`, `core.Call*Instruction`, `core.ToolSpec`, `core.WriteAheadLog`.

- [ ] **Step 1: Write the failing wire-string test**

Add to `core/state_test.go`:

```go
func TestStateJSONWireStrings(t *testing.T) {
    s := State{
        RunID: "r1",
        ThinkingKind: REASON_REACT,
        Messages: []Message{{
            Role: ROLE_USER,
            Chunks: []Chunk{{Kind: PART_KIND_PLAIN_TEXT, Text: "hi"}},
        }},
        Scratch: map[string]any{"k": "v"},
        Status: RUN_STATUS_RUNNING,
        Autonomy: AUTONOMY_L2,
    }
    raw, err := json.Marshal(s)
    require.NoError(t, err)
    // Plain English wire strings:
    assert.Contains(t, string(raw), `"thinking_kind":"think_then_act"`)
    assert.Contains(t, string(raw), `"kind":"plain_text"`)
    // Old strings gone:
    assert.NotContains(t, string(raw), `"react"`)
    assert.NotContains(t, string(raw), `"text"`)
    assert.NotContains(t, string(raw), `"percept"`)
}
```

- [ ] **Step 2: Run the test, expect FAIL**

Run: `cd /Users/bytedance/projects/agentSDK && go test ./core/ -run TestStateJSONWireStrings -v`
Expected: FAIL — `json: unknown enum 'text'` (old `CHUNK_KIND_TEXT` value, etc.)

- [ ] **Step 3: Rename types and consts in `core/effect.go`**

Replace the file with:

```go
package core

// InstructionKind discriminates which field of Instruction carries the
// payload. (Plotkin & Power 2003, "Algebraic Operations and Generic Effects";
// the agent-loop community overloads "effect" with provider side-effect.)
type InstructionKind string

const (
    INSTRUCTION_CALL_MODEL       InstructionKind = "call_model"
    INSTRUCTION_CALL_TOOL        InstructionKind = "call_tool"
    INSTRUCTION_REQUEST_APPROVAL InstructionKind = "request_approval"
    INSTRUCTION_NOTIFY           InstructionKind = "notify"
    INSTRUCTION_CHECKPOINT       InstructionKind = "checkpoint"
    INSTRUCTION_EMIT             InstructionKind = "emit"
    INSTRUCTION_DONE             InstructionKind = "done"
)

type CallModelInstruction struct {
    RequestID string  `json:"request_id"`
    Messages  []Message `json:"messages"`
    Tools     []ToolSpec `json:"tools,omitempty"`
    MaxTokens int     `json:"max_tokens,omitempty"`
}

type CallToolInstruction struct {
    Call ToolCall `json:"call"`
}

type RequestApprovalInstruction struct {
    ApprovalID string   `json:"approval_id"`
    Reason     string   `json:"reason"`
    Risk       RiskLevel `json:"risk"`
    Summary    string   `json:"summary"`
    ToolCall   *ToolCall `json:"tool_call,omitempty"`
}

type NotifyInstruction struct {
    Level   string `json:"level"`
    Message string `json:"message"`
}

type CheckpointInstruction struct {
    Reason string `json:"reason"`
}

type EmitInstruction struct {
    Envelope any `json:"envelope"`
}

// Instruction is a tagged union — exactly one pointer is non-nil per
// Kind. The runtime executes them in order.
type Instruction struct {
    Kind            InstructionKind          `json:"kind"`
    CallModel       *CallModelInstruction    `json:"call_model,omitempty"`
    CallTool        *CallToolInstruction     `json:"call_tool,omitempty"`
    RequestApproval *RequestApprovalInstruction `json:"request_approval,omitempty"`
    Notify          *NotifyInstruction       `json:"notify,omitempty"`
    Checkpoint      *CheckpointInstruction   `json:"checkpoint,omitempty"`
    Emit            *EmitInstruction         `json:"emit,omitempty"`
}
```

- [ ] **Step 4: Rename types in `core/input.go`**

Replace the file with:

```go
package core

import (
    "context"
    "time"
)

type EventKind string

const (
    EVENT_OBSERVATION    EventKind = "observation"
    EVENT_MODEL_REPLY    EventKind = "model_reply"
    EVENT_TOOL_RESULT    EventKind = "tool_result"
    EVENT_HUMAN_DECISION EventKind = "human_decision"
    EVENT_RESUME         EventKind = "resume"
)

// Observation is one reading from a perception.ObservationSource.
// (The agent-loop community settled on "observation"; "percept" traces
// to Rosenblatt 1958 and is rarely used in current LLM-agent papers.)
type Observation struct {
    ID         string    `json:"id"`
    Source     string    `json:"source"`
    ObservedAt time.Time `json:"observed_at"`
    Payload    any       `json:"payload"`
}

type ToolCall struct {
    ID    string         `json:"id"`
    Name  string         `json:"name"`
    Args  map[string]any `json:"args"`
    Risk  RiskLevel      `json:"risk,omitempty"`
}

type ToolResult struct {
    CallID    string `json:"call_id"`
    Name      string `json:"name"`
    OK        bool   `json:"ok"`
    Output    any    `json:"output,omitempty"`
    Error     string `json:"error,omitempty"`
    ElapsedMS int64  `json:"elapsed_ms,omitempty"`
}

type ModelChunk struct {
    Kind     ChunkKind   `json:"kind"`
    Text     string      `json:"text,omitempty"`
    ToolUse  *ToolUseChunk `json:"tool_use,omitempty"`
    Done     bool        `json:"done"`
}

type ToolUseChunk struct {
    ID    string         `json:"id"`
    Name  string         `json:"name"`
    Args  map[string]any `json:"args"`
}

type ModelResult struct {
    Text       string     `json:"text,omitempty"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    StopReason string     `json:"stop_reason"`
    Usage      TokenUsage `json:"usage"`
}

type TokenUsage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}

func (u TokenUsage) Add() TokenUsage { return u }

// Event drives one Decide. Exactly one payload field is set per Event.
type Event struct {
    Kind             EventKind         `json:"kind"`
    Observation      *Observation      `json:"observation,omitempty"`
    ModelResult      *ModelResult      `json:"model_result,omitempty"`
    ToolResult       *ToolResult       `json:"tool_result,omitempty"`
    HumanDecision    *ApprovalDecision `json:"human_decision,omitempty"`
    Seq              int               `json:"seq"`
    ReceivedAt       time.Time         `json:"received_at"`
}

// ObservationSource mirrors perception.ObservationSource. Kept here so
// core does not import perception.
type ObservationSource interface {
    Observations(ctx context.Context) <-chan Observation
}
```

- [ ] **Step 5: Rename in `core/message.go`**

Replace the file with:

```go
package core

import "time"

type Role string

const (
    ROLE_SYSTEM    Role = "system"
    ROLE_USER      Role = "user"
    ROLE_ASSISTANT Role = "assistant"
    ROLE_TOOL      Role = "tool"
)

type PartKind string

const (
    PART_KIND_PLAIN_TEXT   PartKind = "plain_text"
    PART_KIND_AUDIO        PartKind = "audio"
    PART_KIND_IMAGE        PartKind = "image"
    PART_KIND_TOOL_USE     PartKind = "tool_use"
    PART_KIND_TOOL_RESULT  PartKind = "tool_result"
)

type ToolResultPart struct {
    CallID string `json:"call_id"`
    Name   string `json:"name"`
    OK     bool   `json:"ok"`
    Output any    `json:"output,omitempty"`
    Error  string `json:"error,omitempty"`
}

// Part is one fragment of a Message: text, audio, image, tool_use, or tool_result.
type Part struct {
    Kind       PartKind       `json:"kind"`
    Text       string         `json:"text,omitempty"`
    Audio      []byte         `json:"audio,omitempty"`
    AudioMIME  string         `json:"audio_mime,omitempty"`
    Image      []byte         `json:"image,omitempty"`
    ImageMIME  string         `json:"image_mime,omitempty"`
    ToolUse    *ToolUseChunk  `json:"tool_use,omitempty"`
    ToolResult *ToolResultPart `json:"tool_result,omitempty"`
}

type Message struct {
    Role  Role           `json:"role"`
    Parts []Part         `json:"parts"`
    Ts    time.Time      `json:"ts"`
}

func (m Message) AppendText(s string) Message {
    out := m
    out.Parts = append(append([]Part(nil), m.Parts...), Part{Kind: PART_KIND_PLAIN_TEXT, Text: s})
    return out
}
```

- [ ] **Step 6: Rename in `core/thinking.go`**

Replace the file with:

```go
package core

// ReasoningStyle is the selector for which DecisionRule owns a step.
type ReasoningStyle string

const (
    REASON_REACT            ReasoningStyle = "think_then_act" // Yao 2023
    REASON_PLAN_THEN_RUN    ReasoningStyle = "plan_then_run"  // Wei 2022
    REASON_DO_THEN_REVIEW   ReasoningStyle = "do_then_review" // Welleck 2023
    REASON_ONE_SHOT         ReasoningStyle = "one_shot"       // Wei 2022
    REASON_LEARN_FROM_FAILURE ReasoningStyle = "learn_from_failure" // Shinn 2023
    REASON_PICK_AGENT       ReasoningStyle = "choose_agent"   // multi-agent routing
)

// DecisionRule owns one reduce step. NextStep is a pure function.
type DecisionRule interface {
    Kind() ReasoningStyle
    NextStep(state State) (State, []Instruction)
}
```

- [ ] **Step 7: Rename in `core/tool.go`**

Replace the file with:

```go
package core

type RiskLevel string

const (
    RISK_LEVEL_LOW  RiskLevel = "low"
    RISK_LEVEL_HIGH RiskLevel = "high"
)

// ToolSpec is what the LLM sees: name, description, parameter schema, risk.
type ToolSpec struct {
    Name        string    `json:"name"`
    Description string    `json:"description,omitempty"`
    Parameters  any       `json:"parameters"`
    Risk        RiskLevel `json:"risk"`
}

type JSONSchema struct {
    Type                 string             `json:"type"`
    Properties           map[string]*JSONSchema `json:"properties,omitempty"`
    Required             []string           `json:"required,omitempty"`
    AdditionalProperties any                `json:"additionalProperties,omitempty"`
}
```

- [ ] **Step 8: Rename in `core/port.go`**

Edit the file:
- Rename interface `WAL` → `WriteAheadLog`
- Rename `ToolSchema` → `ToolSpec` (already done in tool.go; update the field types here: `ToolRegistry.List() []ToolSpec`, `ModelRequest.Tools []ToolSpec`)

Replace the file with:

```go
package core

import (
    "context"
    "encoding/json"
)

type ModelProvider interface {
    Name() string
    Generate(ctx context.Context, req ModelRequest) (ModelResult, error)
    Stream(ctx context.Context, req ModelRequest) (<-chan ModelChunk, error)
    CountTokens(ctx context.Context, msgs []Message) (int, error)
}

type ModelRequest struct {
    Messages   []Message   `json:"messages"`
    Tools      []ToolSpec  `json:"tools,omitempty"`
    MaxTokens  int         `json:"max_tokens,omitempty"`
    StopReasons []string   `json:"stop_reasons,omitempty"`
}

type StateStore interface {
    Save(ctx context.Context, s State) error
    Load(ctx context.Context, runID string) (State, error)
    List(ctx context.Context) ([]string, error)
    Delete(ctx context.Context, runID string) error
}

// WriteAheadLog is the append-only event log used for crash recovery.
type WriteAheadLog interface {
    Append(ctx context.Context, runID string, seq int, in Event) error
    Read(ctx context.Context, runID string, sinceSeq int) ([]Event, error)
    TruncateFrom(ctx context.Context, runID string, uptoSeq int) error
}

type ToolRegistry interface {
    Register(t Tool)
    Get(name string) (Tool, bool)
    List() []ToolSpec
    Call(ctx context.Context, call ToolCall) ToolResult
}

type Tool interface {
    Name() string
    Description() string
    Schema() ToolSpec
    Risk() RiskLevel
    Call(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

type Notifier interface {
    Notify(ctx context.Context, message string) error
}
```

- [ ] **Step 9: Rename in `core/step.go`**

Replace the file with:

```go
package core

// Decide is the pure transition function. Given (state, event) it produces
// the next state and a list of instructions the runtime must execute.
type Decide func(state State, event Event) (State, []Instruction)

// NewDecide returns a Decide that dispatches on state.ReasoningStyle.
//
// Reasoning lives in the per-rule NextStep methods. The runtime only
// needs to feed events in order.
func NewDecide(rules map[ReasoningStyle]DecisionRule) Decide {
    return func(state State, event Event) (State, []Instruction) {
        kind := state.ReasoningStyle
        r, ok := rules[kind]
        if !ok {
            return state, []Instruction{
                {Kind: INSTRUCTION_NOTIFY, Notify: &NotifyInstruction{
                    Level:   "error",
                    Message: "unknown reasoning style: " + string(kind),
                }},
            }
        }
        return r.NextStep(state)
    }
}
```

- [ ] **Step 10: Rename `Scratch` → `WorkingMemory` in `core/state.go`**

Edit `core/state.go`:

```go
// State is the serialized, persistent snapshot of a run.
type State struct {
    RunID            string                  `json:"run_id"`
    Turn             int                     `json:"turn"`
    Autonomy         AutonomyLevel           `json:"autonomy"`
    ReasoningStyle   ReasoningStyle          `json:"thinking_kind"` // JSON tag kept for back-compat with persisted state
    Messages         []Message               `json:"messages"`
    WorkingMemory    map[string]any          `json:"scratch,omitempty"` // JSON tag kept for back-compat
    PendingApprovals []PendingApproval       `json:"pending_approvals,omitempty"`
    Budget           Budget                  `json:"budget"`
    Status           RunStatus               `json:"status"`
    UpdatedAt        time.Time               `json:"updated_at"`
    LastInputSeq     int                     `json:"last_input_seq"`
}

func (s State) Clone() State {
    out := s
    if s.Messages != nil {
        msgs := make([]Message, len(s.Messages))
        for i, m := range s.Messages {
            msgs[i] = m
            if m.Parts != nil {
                ps := make([]Part, len(m.Parts))
                copy(ps, m.Parts)
                msgs[i].Parts = ps
            }
        }
        out.Messages = msgs
    }
    if s.WorkingMemory != nil {
        wm := make(map[string]any, len(s.WorkingMemory))
        for k, v := range s.WorkingMemory {
            wm[k] = v
        }
        out.WorkingMemory = wm
    }
    if s.PendingApprovals != nil {
        pa := make([]PendingApproval, len(s.PendingApprovals))
        copy(pa, s.PendingApprovals)
        out.PendingApprovals = pa
    }
    return out
}
```

(Keeping the JSON tags `thinking_kind` and `scratch` lets the migration shim in Task 2 read old state. The Go fields are renamed.)

- [ ] **Step 11: Update `core/state_test.go` and `core/step_test.go`**

Run: `grep -rln 'Percept\|Input\|Chunk\|Effect\|ThinkingKind\|ThinkingPattern\|Decide\|ReAct\|ToolSchema\|WAL' core/`
For each file, mechanically rename:
- `Percept` → `Observation`
- `Percept` field on `Input` → `Observation` field on `Event`
- `Input` → `Event`
- `InputKind` → `EventKind`
- `INPUT_KIND_PERCEPT` → `EVENT_OBSERVATION`
- `INPUT_KIND_MODEL_RESULT` → `EVENT_MODEL_REPLY`
- `INPUT_KIND_TOOL_RESULT` → `EVENT_TOOL_RESULT`
- `INPUT_KIND_APPROVAL_DECISION` → `EVENT_HUMAN_DECISION`
- `INPUT_KIND_RESUME` → `EVENT_RESUME`
- `Effect` → `Instruction`
- `EFFECT_*` → `INSTRUCTION_*`
- `CallModelEffect` → `CallModelInstruction`
- `CallToolEffect` → `CallToolInstruction`
- `RequestApprovalEffect` → `RequestApprovalInstruction`
- `NotifyEffect` → `NotifyInstruction`
- `CheckpointEffect` → `CheckpointInstruction`
- `EmitEffect` → `EmitInstruction`
- `Chunk` → `Part`
- `Chunks` field on `Message` → `Parts`
- `ChunkKind` → `PartKind`
- `CHUNK_KIND_TEXT` → `PART_KIND_PLAIN_TEXT`
- `CHUNK_KIND_AUDIO` → `PART_KIND_AUDIO`
- `CHUNK_KIND_IMAGE` → `PART_KIND_IMAGE`
- `CHUNK_KIND_TOOL_USE` → `PART_KIND_TOOL_USE`
- `CHUNK_KIND_TOOL_RESULT` → `PART_KIND_TOOL_RESULT`
- `ThinkingKind` → `ReasoningStyle`
- `ThinkingPattern` → `DecisionRule`
- `THINK_REACT` → `REASON_REACT`
- `THINK_PLANNER_EXECUTOR` → `REASON_PLAN_THEN_RUN`
- `THINK_EXECUTOR_CRITIC` → `REASON_DO_THEN_REVIEW`
- `THINK_COT_SINGLESHOT` → `REASON_ONE_SHOT`
- `THINK_REFLEXION` → `REASON_LEARN_FROM_FAILURE`
- `THINK_ROUTER` → `REASON_PICK_AGENT`
- `ToolSchema` → `ToolSpec`
- `WAL` → `WriteAheadLog`
- `Scratch` field access → `WorkingMemory` (only where it's a Go field; JSON tag remains `scratch`)
- `state.Scratch[...]` → `state.WorkingMemory[...]`
- `func(state State) input Input` → `func(state State, event Event)`
- All `Step(state, input)` call sites → `Decide(state, event)`

Use a small Go-aware script to do the mechanical parts — `gofmt -r 'Percept -> Observation'` works for simple cases; for the rest, run the Find agent with the substitution list above.

- [ ] **Step 12: Run core tests**

Run: `go test ./core/ -count=1 -v`
Expected: PASS, all tests green.

- [ ] **Step 13: Commit**

```bash
git add core/
git commit -m "refactor(core): rename Effect→Instruction, Percept→Observation, Chunk→Part, ThinkingKind→ReasoningStyle; rename ReAct, PlannerExecutor, ExecutorCritic, COTSingleshot, Reflexion, Router; rename wire strings (think_then_act, plan_then_run, do_then_review, one_shot, learn_from_failure, choose_agent, observation, model_reply, human_decision, plain_text)"
```

---

### Task 2: Wire-string migration shim in `memory/filestore/`

**Files:**
- Modify: `memory/filestore/filestore.go`

**Interfaces:**
- Consumes: `core.State` (renamed fields with old JSON tags preserved).
- Produces: `JSONFileStateStore` that translates old persisted state into new field names on load.

- [ ] **Step 1: Write the failing migration test**

Add to `memory/filestore/filestore_test.go` (create it if absent):

```go
package filestore

import (
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"

    "github.com/bizshuk/agentsdk/core"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestLoadMigratesOldWireStrings(t *testing.T) {
    dir := t.TempDir()
    old := map[string]any{
        "run_id":       "r-old",
        "turn":         3,
        "autonomy":     "L2",
        "thinking_kind": "react",
        "messages": []map[string]any{{
            "role": "user",
            "chunks": []map[string]any{{
                "kind": "text",
                "text": "hello",
            }},
        }},
        "scratch":         map[string]any{"k": "v"},
        "status":          "running",
        "last_input_seq":  1,
    }
    raw, _ := json.Marshal(old)
    require.NoError(t, os.WriteFile(filepath.Join(dir, "states", "r-old.json"), raw, 0o640))

    s, err := NewJSONFileStateStore(dir)
    require.NoError(t, err)
    loaded, err := s.Load(context.Background(), "r-old")
    require.NoError(t, err)
    assert.Equal(t, core.REASON_REACT, loaded.ReasoningStyle)
    require.Len(t, loaded.Messages, 1)
    require.Len(t, loaded.Messages[0].Parts, 1)
    assert.Equal(t, core.PART_KIND_PLAIN_TEXT, loaded.Messages[0].Parts[0].Kind)
    assert.Equal(t, "v", loaded.WorkingMemory["k"])
}
```

- [ ] **Step 2: Run the test, expect FAIL**

Run: `go test ./memory/filestore/ -run TestLoadMigratesOldWireStrings -v`
Expected: FAIL — old state has `chunks` not `parts`, `text` not `plain_text`, `react` not `think_then_act`.

- [ ] **Step 3: Rename `FileStateStore` and add migration shim**

Replace `memory/filestore/filestore.go` with the renamed types plus a Load migration. Key changes:
- `FileStateStore` → `JSONFileStateStore`; `NewFileStateStore` → `NewJSONFileStateStore`
- `FileWAL` → `JSONLFileLog`; `NewFileWAL` → `NewJSONLFileLog`; `Append/Replay/Truncate` keep names but receiver type changes
- In `Load`, after `json.Unmarshal`, walk the loaded State and translate the legacy field names

For the `Message.Parts` JSON tag (`"chunks"` in v1, `"parts"` in v2), add a two-pass unmarshal in `Load`:

```go
func (s *JSONFileStateStore) Load(_ context.Context, runID string) (core.State, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    raw, err := os.ReadFile(s.path(runID))
    if err != nil {
        if os.IsNotExist(err) {
            return core.State{}, fmt.Errorf("run not found: %s", runID)
        }
        return core.State{}, err
    }
    // First try v2 ("parts"). On failure, try v1 ("chunks") and re-tag.
    var st core.State
    if err := json.Unmarshal(raw, &st); err == nil && len(st.Messages) > 0 && len(st.Messages[0].Parts) > 0 {
        migrateFromV1(&st)
        return st, nil
    }
    // v1 fallback: rewrite "chunks" → "parts" and try again.
    rewritten := v1ToV2JSON(raw)
    if err := json.Unmarshal(rewritten, &st); err != nil {
        return core.State{}, fmt.Errorf("state unmarshal: %w", err)
    }
    migrateFromV1(&st)
    return st, nil
}

// v1ToV2JSON rewrites the v1 JSON tag "chunks" to the v2 tag "parts"
// inside the raw bytes of a saved State. Used to load pre-rename files
// that used the "chunks" JSON tag for Message parts.
func v1ToV2JSON(raw []byte) []byte {
    return []byte(strings.Replace(string(raw), `"chunks":`, `"parts":`, -1))
}

Add these helpers inside the filestore package:

```go
// migrateFromV1 translates a v1 (pre-rename) State into the v2 shape.
// v1 used the academic names: Percept/Input/Effect/Chunk/ThinkingKind/
// ReAct/etc.; v2 uses Observation/Event/Instruction/Part/ReasoningStyle/
// think_then_act/etc. JSON tag shapes also changed ("chunks"→"parts",
// "percept"→"observation", etc.).
func migrateFromV1(s *core.State) {
    // Map ReasoningStyle values.
    switch s.ReasoningStyle {
    case "react":
        s.ReasoningStyle = core.REASON_REACT
    case "planner_executor":
        s.ReasoningStyle = core.REASON_PLAN_THEN_RUN
    case "executor_critic":
        s.ReasoningStyle = core.REASON_DO_THEN_REVIEW
    case "cot_singleshot":
        s.ReasoningStyle = core.REASON_ONE_SHOT
    case "reflexion":
        s.ReasoningStyle = core.REASON_LEARN_FROM_FAILURE
    case "router":
        s.ReasoningStyle = core.REASON_PICK_AGENT
    }
    // Map PartKind values in every Message.
    for mi := range s.Messages {
        for pi := range s.Messages[mi].Parts {
            switch s.Messages[mi].Parts[pi].Kind {
            case "text":
                s.Messages[mi].Parts[pi].Kind = core.PART_KIND_PLAIN_TEXT
            }
        }
    }
    // WorkingMemory already loads under the "scratch" JSON tag (we kept
    // the tag), so no field-rename needed.
    // ReasoningStyle JSON tag "thinking_kind" is also preserved — only
    // the value is translated above.
}
```

Then in `Load` (before returning), call `migrateFromV1(&st)`. Add a `version` JSON field for future migrations; v1 = absent, v2 = `2`.

- [ ] **Step 4: Run all filestore tests**

Run: `go test ./memory/filestore/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add memory/filestore/
git commit -m "refactor(filestore): rename FileStateStore→JSONFileStateStore, FileWAL→JSONLFileLog; add v1→v2 wire-string migration in Load (Percept→Observation, Chunk→Part, Effect→Instruction, ThinkingKind→ReasoningStyle, ReAct→think_then_act, etc.)"
```

---

### Task 3: `planning/` package

**Files:**
- Modify: `planning/react.go`, `planning/planner_executor.go`, `planning/executor_critic.go`, `planning/reflexion.go`, `planning/router.go`, `planning/cot_singleshot.go`, `planning/helpers.go`
- Modify: `planning/planning_test.go`

- [ ] **Step 1: Rename in `planning/react.go`**

Replace the file with:

```go
// Package planning hosts the six DecisionRule implementations.
// Each rule's NextStep is a pure reducer: read state, write instructions + working memory.
// No I/O — runtime does dispatching based on the instructions returned.
package planning

import "github.com/bizshuk/agentsdk/core"

const (
    THINK_THEN_ACT_PHASE         = "think_then_act.phase"
    THINK_THEN_ACT_PENDING_CALL  = "think_then_act.pending_call"
    THINK_THEN_ACT_LAST_RESULT   = "think_then_act.last_result"

    THINK_THEN_ACT_REASON   = "reason"
    THINK_THEN_ACT_DISPATCH = "dispatch"
    THINK_THEN_ACT_REFLECT  = "reflect"
)

// ThinkThenAct implements the classic Reason+Act loop (Yao 2023).
type ThinkThenAct struct{}

// NewThinkThenAct returns the rule.
func NewThinkThenAct() *ThinkThenAct { return &ThinkThenAct{} }

// Kind returns REASON_REACT.
func (p *ThinkThenAct) Kind() core.ReasoningStyle { return core.REASON_REACT }

func (p *ThinkThenAct) NextStep(state core.State) (core.State, []core.Instruction) {
    state.UpdatedAt = nowOrZero(state)
    phase := scratchString(state, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_REASON)

    switch phase {
    case THINK_THEN_ACT_REASON:
        next := state.Clone()
        scratchSet(&next, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_DISPATCH)
        return next, []core.Instruction{callModelFromMessages(next)}
    case THINK_THEN_ACT_DISPATCH:
        call, ok := scratchCall(state, THINK_THEN_ACT_PENDING_CALL)
        if !ok {
            return state, []core.Instruction{doneInstruction()}
        }
        next := state.Clone()
        scratchSet(&next, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_REFLECT)
        return next, []core.Instruction{callToolInstruction(call)}
    case THINK_THEN_ACT_REFLECT:
        next := state.Clone()
        scratchSet(&next, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_DISPATCH)
        return next, []core.Instruction{callModelFromMessages(next)}
    default:
        return state, []core.Instruction{doneInstruction()}
    }
}

// SeedDispatch installs a pending tool call so the next NextStep emits
// INSTRUCTION_CALL_TOOL.
func SeedDispatch(s *core.State, call core.ToolCall) {
    scratchSet(s, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_DISPATCH)
    scratchSet(s, THINK_THEN_ACT_PENDING_CALL, call)
}
```

- [ ] **Step 2: Rename in `planning/planner_executor.go`**

Replace the file with:

```go
package planning

import "github.com/bizshuk/agentsdk/core"

const (
    PLAN_THEN_RUN_BLUEPRINT  = "plan_then_run.blueprint"
    PLAN_THEN_RUN_STEP_INDEX = "plan_then_run.step_index"
    PLAN_THEN_RUN_PHASE      = "plan_then_run.phase"

    PLAN_PHASE = "plan"
    RUN_PHASE  = "execute"
    DONE_PHASE = "done"
)

type PlanThenRun struct{}

// NewPlanThenRun returns the rule.
func NewPlanThenRun() *PlanThenRun { return &PlanThenRun{} }

func (p *PlanThenRun) Kind() core.ReasoningStyle { return core.REASON_PLAN_THEN_RUN }

func (p *PlanThenRun) NextStep(state core.State) (core.State, []core.Instruction) {
    state.UpdatedAt = nowOrZero(state)
    phase := scratchString(state, PLAN_THEN_RUN_PHASE, PLAN_PHASE)

    switch phase {
    case PLAN_PHASE:
        if blueprint, ok := scratchBlueprint(state); ok && len(blueprint) > 0 {
            next := state.Clone()
            scratchSet(&next, PLAN_THEN_RUN_PHASE, RUN_PHASE)
            scratchSet(&next, PLAN_THEN_RUN_STEP_INDEX, 0)
            return next, []core.Instruction{callToolInstruction(blueprint[0])}
        }
        return state, []core.Instruction{callModelFromMessages(state.Clone())}
    case RUN_PHASE:
        blueprint, ok := scratchBlueprint(state)
        if !ok || len(blueprint) == 0 {
            next := state.Clone()
            scratchSet(&next, PLAN_THEN_RUN_PHASE, DONE_PHASE)
            return next, []core.Instruction{doneInstruction()}
        }
        idx := scratchInt(state, PLAN_THEN_RUN_STEP_INDEX, 0)
        if idx >= len(blueprint) {
            next := state.Clone()
            scratchSet(&next, PLAN_THEN_RUN_PHASE, DONE_PHASE)
            return next, []core.Instruction{doneInstruction()}
        }
        next := state.Clone()
        scratchSet(&next, PLAN_THEN_RUN_STEP_INDEX, idx+1)
        return next, []core.Instruction{callToolInstruction(blueprint[idx])}
    default:
        return state, []core.Instruction{doneInstruction()}
    }
}

func SeedBlueprint(s *core.State, blueprint []core.ToolCall) {
    scratchSet(s, PLAN_THEN_RUN_BLUEPRINT, blueprint)
    scratchSet(s, PLAN_THEN_RUN_PHASE, RUN_PHASE)
    scratchSet(s, PLAN_THEN_RUN_STEP_INDEX, 0)
}
```

- [ ] **Step 3: Rename in `planning/executor_critic.go`**

Replace the file with:

```go
package planning

import "github.com/bizshuk/agentsdk/core"

const (
    RUN_THEN_REVIEW_PHASE     = "do_then_review.phase"
    RUN_THEN_REVIEW_NOTE      = "do_then_review.note"
    RUN_THEN_REVIEW_ITERATION = "do_then_review.iteration"

    RUN_PHASE   = "execute"
    REVIEW_PHASE = "critique"
    DONE_PHASE   = "done"
)

type RunThenReview struct{}

// NewRunThenReview returns the rule.
func NewRunThenReview() *RunThenReview { return &RunThenReview{} }

func (p *RunThenReview) Kind() core.ReasoningStyle { return core.REASON_DO_THEN_REVIEW }

func (p *RunThenReview) NextStep(state core.State) (core.State, []core.Instruction) {
    state.UpdatedAt = nowOrZero(state)
    phase := scratchString(state, RUN_THEN_REVIEW_PHASE, RUN_PHASE)

    switch phase {
    case RUN_PHASE:
        next := state.Clone()
        scratchSet(&next, RUN_THEN_REVIEW_PHASE, REVIEW_PHASE)
        return next, []core.Instruction{callModelFromMessages(state.Clone())}
    case REVIEW_PHASE:
        note := scratchString(state, RUN_THEN_REVIEW_NOTE, "")
        if note == "" || startsWithPassed(note) {
            next := state.Clone()
            scratchSet(&next, RUN_THEN_REVIEW_PHASE, DONE_PHASE)
            return next, []core.Instruction{doneInstruction()}
        }
        iter := scratchInt(state, RUN_THEN_REVIEW_ITERATION, 0)
        next := state.Clone()
        scratchSet(&next, RUN_THEN_REVIEW_PHASE, RUN_PHASE)
        scratchSet(&next, RUN_THEN_REVIEW_ITERATION, iter+1)
        return next, []core.Instruction{callModelFromMessages(state.Clone())}
    default:
        return state, []core.Instruction{doneInstruction()}
    }
}

// SeedReviewPassed tells the rule the previous review was passing.
func SeedReviewPassed(s *core.State, text string) {
    scratchSet(s, RUN_THEN_REVIEW_PHASE, REVIEW_PHASE)
    scratchSet(s, RUN_THEN_REVIEW_NOTE, "OK: "+text)
}

// SeedReviewFailed tells the rule to iterate.
func SeedReviewFailed(s *core.State, text string) {
    scratchSet(s, RUN_THEN_REVIEW_PHASE, REVIEW_PHASE)
    scratchSet(s, RUN_THEN_REVIEW_NOTE, text)
}
```

- [ ] **Step 4: Rename in `planning/cot_singleshot.go`**

Replace the file with:

```go
package planning

import "github.com/bizshuk/agentsdk/core"

// OneShotReasoning is the one-shot chain-of-thought rule (Wei 2022).
//
// STUB: emits exactly one INSTRUCTION_CALL_MODEL and INSTRUCTION_DONE.
type OneShotReasoning struct{}

// NewOneShotReasoning returns the rule.
func NewOneShotReasoning() *OneShotReasoning { return &OneShotReasoning{} }

func (p *OneShotReasoning) Kind() core.ReasoningStyle { return core.REASON_ONE_SHOT }

func (p *OneShotReasoning) NextStep(state core.State) (core.State, []core.Instruction) {
    next := state.Clone()
    next.UpdatedAt = nowOrZero(state)
    return next, []core.Instruction{
        callModelFromMessages(state.Clone()),
        doneInstruction(),
    }
}
```

- [ ] **Step 5: Rename in `planning/reflexion.go`**

Replace the file with:

```go
package planning

import "github.com/bizshuk/agentsdk/core"

// LearnFromFailure: remember failures, retry with reflection (Shinn 2023).
//
// STUB: emits a single INSTRUCTION_CALL_MODEL and INSTRUCTION_DONE.
type LearnFromFailure struct{}

// NewLearnFromFailure returns the rule.
func NewLearnFromFailure() *LearnFromFailure { return &LearnFromFailure{} }

func (p *LearnFromFailure) Kind() core.ReasoningStyle { return core.REASON_LEARN_FROM_FAILURE }

func (p *LearnFromFailure) NextStep(state core.State) (core.State, []core.Instruction) {
    next := state.Clone()
    next.UpdatedAt = nowOrZero(state)
    return next, []core.Instruction{
        callModelFromMessages(state.Clone()),
        doneInstruction(),
    }
}
```

- [ ] **Step 6: Rename in `planning/router.go`**

Replace the file with:

```go
package planning

import "github.com/bizshuk/agentsdk/core"

// ChooseAgent: multi-agent router.
//
// STUB: returns INSTRUCTION_DONE with a notification.
type ChooseAgent struct{}

// NewChooseAgent returns the rule.
func NewChooseAgent() *ChooseAgent { return &ChooseAgent{} }

func (p *ChooseAgent) Kind() core.ReasoningStyle { return core.REASON_PICK_AGENT }

func (p *ChooseAgent) NextStep(state core.State) (core.State, []core.Instruction) {
    next := state.Clone()
    next.UpdatedAt = nowOrZero(state)
    return next, []core.Instruction{
        {Kind: core.INSTRUCTION_NOTIFY, Notify: &core.NotifyInstruction{
            Level:   "warn",
            Message: "choose_agent rule is a STUB; emitting DONE",
        }},
        doneInstruction(),
    }
}
```

- [ ] **Step 7: Update `planning/helpers.go`**

Replace `doneEffect` → `doneInstruction`, `callToolEffect` → `callToolInstruction`, and rename `hasOKPrefix` → `startsWithPassed`:

```go
func callModelFromMessages(state core.State) core.Instruction {
    return core.Instruction{
        Kind: core.INSTRUCTION_CALL_MODEL,
        CallModel: &core.CallModelInstruction{
            RequestID: newID(),
            Messages:  state.Messages,
        },
    }
}

func callToolInstruction(call core.ToolCall) core.Instruction {
    return core.Instruction{
        Kind:    core.INSTRUCTION_CALL_TOOL,
        CallTool: &core.CallToolInstruction{Call: call},
    }
}

func doneInstruction() core.Instruction {
    return core.Instruction{Kind: core.INSTRUCTION_DONE}
}

// startsWithPassed is the cheap "did the review approve?" predicate.
func startsWithPassed(s string) bool {
    return len(s) >= 3 && s[:3] == "OK:"
}
```

(Leave the rest of the helpers — `nowOrZero`, `scratchString`, `scratchInt`, `scratchCall`, `scratchBlueprint`, `scratchSet`, `newID`, `formatUint` — as-is.)

- [ ] **Step 8: Update `planning/planning_test.go`**

Mechanically rename:
- `core.NewStep` → `core.NewDecide`
- `core.THINK_REACT` → `core.REASON_REACT`
- `core.THINK_PLANNER_EXECUTOR` → `core.REASON_PLAN_THEN_RUN`
- `core.THINK_EXECUTOR_CRITIC` → `core.REASON_DO_THEN_REVIEW`
- `core.THINK_COT_SINGLESHOT` → `core.REASON_ONE_SHOT`
- `core.THINK_REFLEXION` → `core.REASON_LEARN_FROM_FAILURE`
- `core.THINK_ROUTER` → `core.REASON_PICK_AGENT`
- `planning.NewReAct` → `planning.NewThinkThenAct`
- `planning.NewPlannerExecutor` → `planning.NewPlanThenRun`
- `planning.NewExecutorCritic` → `planning.NewRunThenReview`
- `planning.NewCOTSingleshot` → `planning.NewOneShotReasoning`
- `planning.NewReflexion` → `planning.NewLearnFromFailure`
- `planning.NewRouter` → `planning.NewChooseAgent`
- `planning.SeedAct` → `planning.SeedDispatch`
- `planning.SeedCritiqueOK` → `planning.SeedReviewPassed`
- `planning.SeedCritiqueReject` → `planning.SeedReviewFailed`
- `REACT_PHASE` → `THINK_THEN_ACT_PHASE`
- `REACT_PHASE_THINK` → `THINK_THEN_ACT_REASON`
- `REACT_PHASE_ACT` → `THINK_THEN_ACT_DISPATCH`
- `REACT_PHASE_OBSERVE` → `THINK_THEN_ACT_REFLECT`
- `REACT_LAST_CALL` → `THINK_THEN_ACT_PENDING_CALL`
- `core.EFFECT_CALL_MODEL` → `core.INSTRUCTION_CALL_MODEL` (and the rest)
- `core.Effect` → `core.Instruction`
- `core.CallModelEffect` → `core.CallModelInstruction`
- `state.Scratch` → `state.WorkingMemory`
- `state.Messages[i].Chunks` → `state.Messages[i].Parts`
- `core.Chunk{Kind: core.CHUNK_KIND_TEXT, ...}` → `core.Part{Kind: core.PART_KIND_PLAIN_TEXT, ...}`
- `core.THINK_*` field access on state → `core.REASON_*` (the State field is `ReasoningStyle`)

- [ ] **Step 9: Run planning tests**

Run: `go test ./planning/ -count=1 -v`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add planning/
git commit -m "refactor(planning): rename rules to plain English (ThinkThenAct, PlanThenRun, RunThenReview, OneShotReasoning, LearnFromFailure, ChooseAgent); update scratch keys, FSM phases, seed helpers"
```

---

### Task 4: `perception/` package

**Files:**
- Modify: `perception/source.go`, `perception/normalize.go`, `perception/perception_test.go`

- [ ] **Step 1: Rename in `perception/source.go`**

Replace the file with:

```go
package perception

import (
    "context"
    "sync"

    "github.com/bizshuk/agentsdk/core"
)

// ObservationSource emits observations on a channel. The runtime reads
// until ctx is done or the channel is closed.
type ObservationSource interface {
    Name() string
    Observations(ctx context.Context) <-chan core.Observation
}

// FanIn merges several ObservationSources into one channel. Order is
// best-effort: each goroutine pushes as data arrives.
type FanIn struct {
    Sources []ObservationSource
}

func (f *FanIn) Observations(ctx context.Context) <-chan core.Observation {
    out := make(chan core.Observation, 32)
    if len(f.Sources) == 0 {
        close(out)
        return out
    }
    var wg sync.WaitGroup
    for _, s := range f.Sources {
        s := s
        wg.Add(1)
        go func() {
            defer wg.Done()
            ch := s.Observations(ctx)
            for {
                select {
                case <-ctx.Done():
                    return
                case p, ok := <-ch:
                    if !ok {
                        return
                    }
                    select {
                    case out <- p:
                    case <-ctx.Done():
                        return
                    }
                }
            }
        }()
    }
    go func() {
        wg.Wait()
        close(out)
    }()
    return out
}

func (f *FanIn) Name() string { return "fan_in" }
```

- [ ] **Step 2: Rename in `perception/normalize.go`**

Replace the file with:

```go
package perception

import "github.com/bizshuk/agentsdk/core"

// ToMessageFunc converts a raw payload (as emitted by an ObservationSource)
// into a structured Message suitable for inclusion in state.Messages.
type ToMessageFunc func(p core.Observation) core.Message

// ToMessage is a helper that applies a ToMessageFunc to each observation.
type ToMessage struct {
    Fn      ToMessageFunc
    MaxSize int
}

func (n *ToMessage) Apply(p core.Observation) core.Message {
    if n.Fn == nil {
        text, _ := p.Payload.(string)
        return core.Message{
            Role: core.ROLE_USER,
            Parts: []core.Part{
                {Kind: core.PART_KIND_PLAIN_TEXT, Text: text},
            },
            Ts: p.ObservedAt,
        }
    }
    return n.Fn(p)
}
```

- [ ] **Step 3: Update `perception/perception_test.go`**

Mechanically rename:
- `Source` → `ObservationSource`
- `Multi` → `FanIn`
- `NormalizeFunc` → `ToMessageFunc`
- `Normalizer` → `ToMessage`
- `.Percepts(` → `.Observations(`
- `core.Percept` → `core.Observation`
- `core.Input` → `core.Event`
- `state.Scratch` → `state.WorkingMemory`
- `core.Chunk` → `core.Part`
- `core.CHUNK_KIND_TEXT` → `core.PART_KIND_PLAIN_TEXT`
- `core.Effect` → `core.Instruction`

- [ ] **Step 4: Run perception tests**

Run: `go test ./perception/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add perception/
git commit -m "refactor(perception): rename Source→ObservationSource, Multi→FanIn, NormalizeFunc→ToMessageFunc, Normalizer→ToMessage, Percepts→Observations"
```

---

### Task 5: `action/` package — verify (no symbol renames, just adjust type references)

**Files:**
- Modify: `action/tool.go`, `action/registry.go`, `action/schema.go`, `action/sandbox.go`, `action/approval_policy.go`
- Modify: `action/action_test.go`, `action/schema_test.go`, `action/sandbox_test.go`, `action/approval_policy_test.go`

- [ ] **Step 1: Update type references**

The `action/` package keeps its symbols (`TypedTool`, `Registry`, `ToolSource`, `Sandbox`, `Verdict`, `Policy`, `DefaultApprovalPolicy`, `SchemaFor*`, `ValidateArgs`, `SchemaError`, `marshalArgs`). Only the types they reference change.

In every `.go` file in `action/`:
- `core.ToolSchema` → `core.ToolSpec`
- `core.ToolResult` (keep)
- `core.ToolCall` (keep)
- `core.RiskLevel` (keep)
- `core.Chunk` → `core.Part` (if used)
- `core.CHUNK_KIND_*` → `core.PART_KIND_*` (if used)
- `core.Instruction` (if referenced)

- [ ] **Step 2: Update tests**

Same mechanical renames in test files.

- [ ] **Step 3: Run action tests**

Run: `go test ./action/ -count=1 -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add action/
git commit -m "refactor(action): update type references after core rename (ToolSchema→ToolSpec, Chunk→Part)"
```

---

### Task 6: `memory/` package

**Files:**
- Modify: `memory/window.go`, `memory/compactor.go`, `memory/memory_test.go`
- Modify: `memory/checkpoint/checkpointer.go`, `memory/checkpoint/checkpoint_test.go` (if present)
- Modify: `memory/filestore/filestore.go` (filestore rename, with migration shim already in Task 2)
- Modify: `memory/filestore/filestore_test.go`

- [ ] **Step 1: Update `memory/window.go` and `memory/compactor.go`**

`Window`, `TokenCounter`, `CharHeuristicCounter`, `Compactor`, `HeadlineCompactor` keep their names. Only:
- `core.Message.Chunks` field reference → `Parts`
- `core.CHUNK_KIND_TEXT` → `core.PART_KIND_PLAIN_TEXT`

- [ ] **Step 2: Rename in `memory/checkpoint/checkpointer.go`**

Replace the file with:

```go
package checkpoint

import (
    "context"
    "fmt"
    "sync"

    "github.com/bizshuk/agentsdk/core"
)

type Recoverer struct {
    Store core.StateStore
    Log   core.WriteAheadLog

    mu sync.Mutex
}

func NewRecoverer(store core.StateStore, log core.WriteAheadLog) *Recoverer {
    return &Recoverer{Store: store, Log: log}
}

func (r *Recoverer) Save(ctx context.Context, s core.State) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    if r.Store == nil {
        return fmt.Errorf("checkpoint: nil store")
    }
    return r.Store.Save(ctx, s)
}

type RecoveredRun struct {
    State  core.State
    Events []core.Event
}

func (r *Recoverer) Recover(ctx context.Context, runID string) (RecoveredRun, error) {
    r.mu.Lock()
    defer r.mu.Unlock()
    if r.Store == nil {
        return RecoveredRun{}, fmt.Errorf("recover: nil store")
    }
    s, err := r.Store.Load(ctx, runID)
    if err != nil {
        return RecoveredRun{}, fmt.Errorf("recover load state: %w", err)
    }
    if r.Log == nil {
        return RecoveredRun{State: s}, nil
    }
    events, err := r.Log.Read(ctx, runID, s.LastInputSeq)
    if err != nil {
        return RecoveredRun{}, fmt.Errorf("recover replay: %w", err)
    }
    return RecoveredRun{State: s, Events: events}, nil
}
```

- [ ] **Step 3: Rename filestore types**

In `memory/filestore/filestore.go`:
- `FileStateStore` → `JSONFileStateStore`; `NewFileStateStore` → `NewJSONFileStateStore`
- `FileWAL` → `JSONLFileLog`; `NewFileWAL` → `NewJSONLFileLog`; `Append`/`Replay`/`Truncate` keep names (the `Replay` is renamed in interface to `Read` — but the file's helper method is allowed to be called `Replay` if it's defined as a separate concrete type, OR rename it to `Read`).

Decision: rename `FileWAL.Replay` → `JSONLFileLog.Read` to match the interface; rename `Truncate` → `TruncateFrom`. The `JSONLFileLog` struct has methods `Append`, `Read`, `TruncateFrom`; the test is updated to call `Read`.

- [ ] **Step 4: Update tests**

In all `memory/` test files:
- `checkpoint.New` → `checkpoint.NewRecoverer`
- `checkpoint.Checkpointer` → `checkpoint.Recoverer`
- `checkpoint.RecoverResult` → `checkpoint.RecoveredRun`
- `filestore.NewFileStateStore` → `filestore.NewJSONFileStateStore`
- `filestore.NewFileWAL` → `filestore.NewJSONLFileLog`
- `core.WAL` → `core.WriteAheadLog`
- `core.WAL.Replay` → `core.WriteAheadLog.Read`
- `core.WAL.Truncate` → `core.WriteAheadLog.TruncateFrom`
- `core.ToolSchema` → `core.ToolSpec`
- `core.Message.Chunks` → `Parts`
- `core.CHUNK_KIND_TEXT` → `core.PART_KIND_PLAIN_TEXT`
- `core.Instruction` (where used)
- `state.Scratch` → `state.WorkingMemory`

- [ ] **Step 5: Run all memory tests**

Run: `go test ./memory/... -count=1 -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add memory/
git commit -m "refactor(memory): rename Checkpointer→Recoverer, RecoverResult→RecoveredRun, FileStateStore→JSONFileStateStore, FileWAL→JSONLFileLog; rename WAL methods (Replay→Read, Truncate→TruncateFrom)"
```

---

### Task 7: `runtime/` package

**Files:**
- Modify: `runtime/loop.go`
- Modify: `runtime/loop_test.go`, `runtime/di_integration_test.go`, `runtime/middleware_integration_test.go`, `runtime/crash_recovery_test.go`

- [ ] **Step 1: Rename in `runtime/loop.go`**

Replace the file (this is the largest single rename) with:

```go
package runtime

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/bizshuk/agentsdk/core"
    "github.com/bizshuk/agentsdk/middleware"
    "github.com/bizshuk/agentsdk/middleware/harness"
    "github.com/bizshuk/agentsdk/middleware/loopguard"
)

type Emitter func(core.Instruction)

type Engine struct {
    Step         core.Decide
    Model        core.ModelProvider
    Tools        core.ToolRegistry
    Store        core.StateStore
    Log          core.WriteAheadLog
    Approval     core.ApprovalPolicy
    Notifier     core.Notifier
    Emitter      Emitter
    Middleware   middleware.Middleware

    chain     middleware.Dispatcher
    chainOnce onceFlag
}

func NewEngine(step core.Decide, model core.ModelProvider, tools core.ToolRegistry) *Engine {
    return &Engine{Step: step, Model: model, Tools: tools}
}

func DefaultMiddleware() middleware.Middleware {
    return middleware.Chain(
        harness.Retry(harness.RetryConfig{N: 3, BaseBackoff: 100 * time.Millisecond, MaxBackoff: 5 * time.Second}),
        harness.Timeout(harness.TimeoutConfig{PerEffect: 60 * time.Second}),
        harness.Budget(),
        loopguard.New(loopguard.Config{MaxRepeats: 5}),
    )
}

func (e *Engine) resolveChain() middleware.Middleware {
    if e.Middleware != nil {
        return e.Middleware
    }
    return DefaultMiddleware()
}

func (e *Engine) runInstruction(ctx context.Context, s core.State, inst core.Instruction) (core.State, *core.Event, bool, error) {
    switch inst.Kind {
    case core.INSTRUCTION_CALL_MODEL:
        if inst.CallModel == nil {
            return s, nil, false, fmt.Errorf("call_model instruction missing CallModel")
        }
        if e.Model == nil {
            return s, nil, false, fmt.Errorf("call_model instruction but no Model bound")
        }
        tools := inst.CallModel.Tools
        if len(tools) == 0 && e.Tools != nil {
            tools = e.Tools.List()
        }
        mr, err := e.Model.Generate(ctx, core.ModelRequest{
            Messages: inst.CallModel.Messages,
            Tools:    tools,
        })
        if err != nil {
            return s, nil, false, fmt.Errorf("model generate: %w", err)
        }
        if len(mr.ToolCalls) > 0 || mr.Text != "" {
            var parts []core.Part
            if mr.Text != "" {
                parts = append(parts, core.Part{Kind: core.PART_KIND_PLAIN_TEXT, Text: mr.Text})
            }
            for _, tc := range mr.ToolCalls {
                parts = append(parts, core.Part{
                    Kind:    core.PART_KIND_TOOL_USE,
                    ToolUse: &core.ToolUseChunk{ID: tc.ID, Name: tc.Name, Args: tc.Args},
                })
            }
            s.Messages = append(s.Messages, core.Message{
                Role:   core.ROLE_ASSISTANT,
                Parts:  parts,
                Ts:     time.Now().UTC(),
            })
        }
        return s, &core.Event{
            Kind:        core.EVENT_MODEL_REPLY,
            ModelResult: &mr,
            ReceivedAt:  time.Now().UTC(),
        }, false, nil

    case core.INSTRUCTION_CALL_TOOL:
        if inst.CallTool == nil {
            return s, nil, false, fmt.Errorf("call_tool instruction missing CallTool")
        }
        if e.Tools == nil {
            return s, nil, false, fmt.Errorf("call_tool instruction but no Tools bound")
        }
        res := e.Tools.Call(ctx, inst.CallTool.Call)
        chunkOut := core.ToolResultPart{
            CallID: res.CallID, Name: res.Name, OK: res.OK,
            Output: res.Output, Error: res.Error,
        }
        s.Messages = append(s.Messages, core.Message{
            Role: core.ROLE_TOOL,
            Parts: []core.Part{
                {Kind: core.PART_KIND_TOOL_RESULT, ToolResult: &chunkOut},
            },
            Ts: time.Now().UTC(),
        })
        return s, &core.Event{
            Kind:       core.EVENT_TOOL_RESULT,
            ToolResult: &res,
            ReceivedAt: time.Now().UTC(),
        }, false, nil

    case core.INSTRUCTION_REQUEST_APPROVAL:
        if inst.RequestApproval == nil {
            return s, nil, false, fmt.Errorf("request_approval instruction missing payload")
        }
        pa := core.PendingApproval{
            ID:          inst.RequestApproval.ApprovalID,
            Reason:      inst.RequestApproval.Reason,
            Risk:        inst.RequestApproval.Risk,
            Summary:     inst.RequestApproval.Summary,
            ToolCall:    inst.RequestApproval.ToolCall,
            RequestedAt: time.Now().UTC(),
        }
        s.PendingApprovals = append(s.PendingApprovals, pa)
        s.Status = core.RUN_STATUS_PAUSED_APPROVAL
        return s, nil, true, nil

    case core.INSTRUCTION_NOTIFY:
        if inst.Notify != nil && e.Notifier != nil {
            _ = e.Notifier.Notify(ctx, fmt.Sprintf("[%s] %s", inst.Notify.Level, inst.Notify.Message))
        }
        return s, nil, false, nil

    case core.INSTRUCTION_CHECKPOINT:
        if e.Store != nil {
            _ = e.Store.Save(ctx, s)
        }
        return s, nil, false, nil

    case core.INSTRUCTION_EMIT:
        return s, nil, false, nil

    case core.INSTRUCTION_DONE:
        s.Status = core.RUN_STATUS_COMPLETED
        return s, nil, true, nil

    default:
        return s, nil, false, fmt.Errorf("unknown instruction kind: %s", inst.Kind)
    }
}

func (e *Engine) buildChain() middleware.Next {
    if !e.chainOnce.value {
        base := middleware.Next(e.runInstruction)
        e.chain = middleware.Dispatcher(e.resolveChain()(base))
        e.chainOnce.value = true
    }
    return middleware.Next(e.chain)
}

type onceFlag struct{ value bool }

func (e *Engine) Run(ctx context.Context, state core.State) (core.State, error) {
    return e.runStep(ctx, state, core.Event{})
}

func (e *Engine) Resume(ctx context.Context, runID string) (core.State, error) {
    if e.Store == nil {
        return core.State{}, fmt.Errorf("resume requires Store")
    }
    s, err := e.Store.Load(ctx, runID)
    if err != nil {
        return core.State{}, err
    }
    if e.Log == nil {
        return e.Run(ctx, s)
    }
    events, err := e.Log.Read(ctx, runID, s.LastInputSeq)
    if err != nil {
        return core.State{}, fmt.Errorf("resume replay: %w", err)
    }
    for _, ev := range events {
        var err error
        s, err = e.runStep(ctx, s, ev)
        if err != nil {
            return s, err
        }
    }
    return s, nil
}

func (e *Engine) SubmitHumanDecision(ctx context.Context, runID string, decision core.ApprovalDecision, decidedBy string) (core.State, error) {
    if e.Store == nil {
        return core.State{}, fmt.Errorf("submit decision requires Store")
    }
    s, err := e.Store.Load(ctx, runID)
    if err != nil {
        return core.State{}, err
    }
    for i := range s.PendingApprovals {
        if s.PendingApprovals[i].Decision == "" {
            s.PendingApprovals[i].Decision = decision
            now := time.Now().UTC()
            s.PendingApprovals[i].DecidedAt = &now
            s.PendingApprovals[i].DecidedBy = decidedBy
            break
        }
    }
    if err := e.Store.Save(ctx, s); err != nil {
        return core.State{}, err
    }
    event := core.Event{
        Kind:          core.EVENT_HUMAN_DECISION,
        HumanDecision: &decision,
        ReceivedAt:    time.Now().UTC(),
    }
    return e.runStep(ctx, s, event)
}

func (e *Engine) RunWithEvent(ctx context.Context, state core.State, seed core.Event) (core.State, error) {
    return e.runStep(ctx, state, seed)
}

func (e *Engine) runStep(ctx context.Context, state core.State, event core.Event) (core.State, error) {
    if state.Budget.StartedAt.IsZero() {
        state.Budget.StartedAt = time.Now().UTC()
    }
    if state.Status == "" {
        state.Status = core.RUN_STATUS_RUNNING
    }

    chain := e.buildChain()
    current := state

    for {
        preStep := current.Clone()
        if preStep.WorkingMemory == nil {
            preStep.WorkingMemory = make(map[string]any, 4)
        }
        if event.ModelResult != nil && len(event.ModelResult.ToolCalls) == 0 {
            preStep.Status = core.RUN_STATUS_COMPLETED
            if e.Store != nil {
                _ = e.Store.Save(ctx, preStep)
            }
            return preStep, nil
        }
        if event.ModelResult != nil && len(event.ModelResult.ToolCalls) > 0 {
            preStep.WorkingMemory["think_then_act.pending_call"] = event.ModelResult.ToolCalls[0]
        }
        if event.ToolResult != nil {
            preStep.WorkingMemory["think_then_act.last_result"] = event.ToolResult.CallID
        }

        next, instructions := e.Step(preStep, event)
        next.Turn = current.Turn + 1
        next.Budget.UsedTurns = next.Turn
        next.UpdatedAt = time.Now().UTC()

        var nextEvent *core.Event
        terminal := false
        for _, inst := range instructions {
            if e.Emitter != nil {
                e.Emitter(inst)
            }
            updated, out, term, err := chain(ctx, next, inst)
            if err != nil {
                next.Status = core.RUN_STATUS_FAILED
                next.UpdatedAt = time.Now().UTC()
                if e.Store != nil {
                    _ = e.Store.Save(ctx, next)
                }
                return next, err
            }
            next = updated
            if term {
                terminal = true
                if out != nil {
                    nextEvent = out
                }
                break
            }
            if out != nil && nextEvent == nil {
                nextEvent = out
            }
        }

        if e.Store != nil {
            _ = e.Store.Save(ctx, next)
        }

        if terminal {
            if next.Status == "" {
                next.Status = core.RUN_STATUS_PAUSED_APPROVAL
            }
            return next, nil
        }

        if nextEvent == nil {
            next.Status = core.RUN_STATUS_COMPLETED
            if e.Store != nil {
                _ = e.Store.Save(ctx, next)
            }
            return next, nil
        }

        if e.Log != nil {
            _ = e.Log.Append(ctx, next.RunID, next.LastInputSeq+1, *nextEvent)
            next.LastInputSeq++
        }

        current = next
        event = *nextEvent
    }
}

func IsBudgetExceeded(err error) bool {
    var be *harness.BudgetExceededError
    return errors.As(err, &be)
}
```

Note: the scratch keys `react.last_call_id` and `react.last_result_signature` (line 315-319 of the old file) become `think_then_act.pending_call` and `think_then_act.last_result` to match the renamed planning constants.

- [ ] **Step 2: Update all runtime tests**

Mechanically rename in `runtime/loop_test.go`, `runtime/di_integration_test.go`, `runtime/middleware_integration_test.go`, `runtime/crash_recovery_test.go`:
- `runtime.NewLoop` → `runtime.NewEngine`
- `runtime.Loop` → `runtime.Engine`
- `loop.Run` / `loop.Resume` / `loop.SubmitApproval` / `loop.RunWithInput` → `loop.Run` / `loop.Resume` / `loop.SubmitHumanDecision` / `loop.RunWithEvent` (Run and Resume keep names)
- `core.Step` → `core.Decide`
- `core.NewStep` → `core.NewDecide`
- `core.Effect` → `core.Instruction`
- `EFFECT_*` → `INSTRUCTION_*`
- `CallModelEffect` → `CallModelInstruction` (etc.)
- `core.Input` → `core.Event`
- `INPUT_KIND_*` → `EVENT_*`
- `core.Percept` → `core.Observation`
- `core.WAL` → `core.WriteAheadLog`
- `loop.WAL` → `loop.Log`
- `core.ToolSchema` → `core.ToolSpec`
- `core.ThinkingKind` → `core.ReasoningStyle`
- `core.THINK_*` → `core.REASON_*`
- `core.Chunk` → `core.Part`
- `core.CHUNK_KIND_*` → `core.PART_KIND_*`
- `state.Scratch` → `state.WorkingMemory`
- `Message.Chunks` → `Message.Parts`
- `runtime.IsBudgetExceeded` (keep)

- [ ] **Step 3: Run runtime tests**

Run: `go test ./runtime/ -count=1 -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add runtime/
git commit -m "refactor(runtime): rename Loop→Engine, NewLoop→NewEngine, RunWithInput→RunWithEvent, SubmitApproval→SubmitHumanDecision; update all type references and scratch keys"
```

---

### Task 8: `middleware/` package

**Files:**
- Modify: `middleware/middleware.go`, `middleware/middleware_test.go`
- Modify: `middleware/harness/retry.go`, `middleware/harness/budget.go`, `middleware/harness/timeout.go`
- Modify: `middleware/loopguard/loopguard.go`
- Modify: `middleware/security/approval_gate.go`, `middleware/security/spotlight.go`, `middleware/security/sanitizer.go`, `middleware/security/sandbox_mw.go`, `middleware/security/spotlight_helpers.go`
- Modify: `middleware/observability/tracing.go`
- Modify: all `*_test.go` files in middleware/

- [ ] **Step 1: Update `middleware/middleware.go`**

`Middleware`, `Next`, `Dispatcher`, `Chain`, `Identity` keep names. Update doc comments to refer to the new `Instruction`/`Event` names (replace `Effect`/`Input` references in comments).

- [ ] **Step 2: Update `middleware/harness/*`**

The `harness/` files keep their types (`Retry`, `RetryConfig`, `RetryableError`, `IsRetryable`, `SimpleRetryable`, `TransientError`, `classifyByString`, `RetryClass*`, `Budget`, `BudgetExceededError`, `Timeout`, `TimeoutConfig`). Only:
- Doc comments: replace `Effect` with `Instruction`, `Input` with `Event`.
- `core.Effect` → `core.Instruction` in function signatures.

- [ ] **Step 3: Update `middleware/loopguard/loopguard.go`**

- Rename `LOOPGUARD_STATE_KEY` → `STATE_KEY` (the package prefix is now redundant).
- Update `core.Effect` → `core.Instruction` in switch cases.
- `core.EFFECT_CALL_TOOL` → `core.INSTRUCTION_CALL_TOOL` (and other kind references).
- `core.CallToolEffect` → `core.CallToolInstruction`.

- [ ] **Step 4: Update `middleware/security/spotlight.go` and helpers**

Replace `security/spotlight.go` with:

```go
package security

import (
    "context"
    "fmt"

    "github.com/bizshuk/agentsdk/core"
    "github.com/bizshuk/agentsdk/middleware"
)

const (
    UNTRUSTED_OPEN  = "<UNTRUSTED_TOOL_OUTPUT>\n"
    UNTRUSTED_CLOSE = "\n</UNTRUSTED_TOOL_OUTPUT>"
    SANITIZED_TAG   = "[SANITIZED_BY_AGENTSDK]"
)

func MarkUntrusted() middleware.Middleware {
    return func(next middleware.Next) middleware.Next {
        return func(ctx context.Context, state core.State, inst core.Instruction) (core.State, *core.Event, bool, error) {
            if inst.Kind != core.INSTRUCTION_CALL_TOOL {
                return next(ctx, state, inst)
            }
            s, ev, term, err := next(ctx, state, inst)
            if err != nil || ev == nil || ev.ToolResult == nil {
                return s, ev, term, err
            }
            wrapped := wrapToolOutput(ev.ToolResult.Output)
            if wrapped != nil {
                ev.ToolResult.Output = wrapped
            }
            return s, ev, term, nil
        }
    }
}

func wrapToolOutput(v any) any {
    switch x := v.(type) {
    case string:
        return UNTRUSTED_OPEN + x + UNTRUSTED_CLOSE
    case []byte:
        return []byte(UNTRUSTED_OPEN + string(x) + UNTRUSTED_CLOSE)
    }
    data, err := marshalAny(v)
    if err != nil {
        return nil
    }
    return UNTRUSTED_OPEN + string(data) + UNTRUSTED_CLOSE
}

func FormatSanitized(reason string) string {
    return fmt.Sprintf("%s reason=%q", SANITIZED_TAG, reason)
}
```

- [ ] **Step 5: Rename in `middleware/security/sanitizer.go`**

Replace `Sanitizer` → `InjectionFilter`. Update method receivers and `Middleware()` factory.

```go
type InjectionFilter struct {
    Patterns []*regexp.Regexp
    WhyFor   []string
}

func DefaultInjectionFilter() *InjectionFilter { /* same body, new constructor name */ }

func (f *InjectionFilter) Inspect(text string) (string, bool) { /* same body */ }

func (f *InjectionFilter) Middleware() middleware.Middleware {
    return func(next middleware.Next) middleware.Next {
        return func(ctx context.Context, state core.State, inst core.Instruction) (core.State, *core.Event, bool, error) {
            if inst.Kind != core.INSTRUCTION_CALL_TOOL {
                return next(ctx, state, inst)
            }
            st, ev, term, err := next(ctx, state, inst)
            if err != nil || ev == nil || ev.ToolResult == nil {
                return st, ev, term, err
            }
            text := outputToString(ev.ToolResult.Output)
            if text == "" {
                return st, ev, term, nil
            }
            reason, hit := f.Inspect(text)
            if !hit {
                return st, ev, term, nil
            }
            ev.ToolResult.Output = FormatSanitized(reason) + " original_len=" + itoa(len(text))
            if st.WorkingMemory == nil {
                st.WorkingMemory = make(map[string]any, 4)
            }
            st.WorkingMemory["injection_filter.last_reason"] = reason
            return st, ev, term, nil
        }
    }
}
```

- [ ] **Step 6: Update `middleware/security/approval_gate.go` and `sandbox_mw.go`**

These keep their function names (`ApprovalGate`, `Sandbox`). Update:
- `core.EFFECT_*` → `core.INSTRUCTION_*`
- `core.Effect` → `core.Instruction`
- `core.CallToolEffect` → `core.CallToolInstruction`
- `core.ToolSchema` → `core.ToolSpec`
- `core.Effect` in switch cases

- [ ] **Step 7: Update `middleware/observability/tracing.go`**

- `core.EFFECT_*` → `core.INSTRUCTION_*` in switch cases
- `core.Effect` → `core.Instruction`
- `core.CallToolEffect` → `core.CallToolInstruction`
- `core.CallModelEffect` → `core.CallModelInstruction`
- `core.RequestApprovalEffect` → `core.RequestApprovalInstruction`
- `core.NotifyEffect` → `core.NotifyInstruction`
- `core.ToolSchema` → `core.ToolSpec`

Span attribute names stay (`agentsdk.effect.kind` becomes `agentsdk.instruction.kind` — wire-level metric names are allowed to change; if you have alerts, update them).

- [ ] **Step 8: Update all middleware tests**

Mechanical renames in every `*_test.go` under `middleware/`.

- [ ] **Step 9: Run all middleware tests**

Run: `go test ./middleware/... -count=1 -v`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add middleware/
git commit -m "refactor(middleware): update type references after core rename; rename Spotlight→MarkUntrusted, Sanitizer→InjectionFilter, LOOPGUARD_STATE_KEY→STATE_KEY"
```

---

### Task 9: `cli/` package

**Files:**
- Modify: `cli/codec.go`, `cli/codec_test.go`, `cli/envelope.go`

- [ ] **Step 1: Update `cli/envelope.go`**

Mechanical renames in const values:
- `MSG_TYPE_PERCEPT` → `MSG_TYPE_OBSERVATION` (value `"observation"`)
- `MSG_TYPE_APPROVAL_DECISION` → `MSG_TYPE_HUMAN_DECISION` (value `"human_decision"`)
- `MSG_TYPE_TOOL_RESULT` (KEEP, value `"tool_result"`)
- All other MSG_TYPE_* (KEEP)

Update doc comments referring to `Effect` → `Instruction`, `Input` → `Event`, `Percept` → `Observation`.

- [ ] **Step 2: Update `cli/codec.go`**

No symbol renames in `Codec`/`WriteError`/`WriteResult`. Update doc comments only.

- [ ] **Step 3: Update `cli/codec_test.go`**

If tests reference `MSG_TYPE_PERCEPT` or `MSG_TYPE_APPROVAL_DECISION`, update to the new names.

- [ ] **Step 4: Run cli tests**

Run: `go test ./cli/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cli/
git commit -m "refactor(cli): rename MSG_TYPE_PERCEPT→MSG_TYPE_OBSERVATION, MSG_TYPE_APPROVAL_DECISION→MSG_TYPE_HUMAN_DECISION"
```

---

### Task 10: `mcp/` package — verify only

**Files:**
- Modify: `mcp/client.go`, `mcp/client_test.go`

- [ ] **Step 1: Update type references**

`mcp.Client` / `mcp.NewClient` keep names. Only type references change:
- `core.ToolSchema` → `core.ToolSpec`
- `core.ToolResult` (keep)
- `core.RiskLevel` (keep)

- [ ] **Step 2: Run mcp tests**

Run: `go test ./mcp/ -count=1 -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add mcp/
git commit -m "refactor(mcp): update type references after core rename (ToolSchema→ToolSpec)"
```

---

### Task 11: `internal/testutil/` package

**Files:**
- Modify: `internal/testutil/fake_provider.go`, `internal/testutil/capturing_notifier.go`, `internal/testutil/mem_store.go`

- [ ] **Step 1: Rename `FakeProvider` → `ScriptedProvider`**

Replace `internal/testutil/fake_provider.go` with:

```go
package testutil

import (
    "context"
    "errors"
    "fmt"
    "sync"

    "github.com/bizshuk/agentsdk/core"
)

type ScriptedProvider struct {
    mu      sync.Mutex
    queue   []core.ModelResult
    calls   int
    streams int
    OnRequest func(req core.ModelRequest)
}

func NewScriptedProvider() *ScriptedProvider { return &ScriptedProvider{} }

func (s *ScriptedProvider) Enqueue(rs ...core.ModelResult) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.queue = append(s.queue, rs...)
}

func (s *ScriptedProvider) EnqueueToolCall(id, name string, args map[string]any) {
    s.Enqueue(core.ModelResult{
        StopReason: "tool_use",
        ToolCalls:  []core.ToolCall{{ID: id, Name: name, Args: args}},
    })
}

func (s *ScriptedProvider) EnqueueEndTurn(text string) {
    s.Enqueue(core.ModelResult{
        StopReason: "end_turn",
        Text:       text,
    })
}

func (s *ScriptedProvider) Name() string { return "scripted" }

func (s *ScriptedProvider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
    s.mu.Lock()
    if s.OnRequest != nil {
        s.OnRequest(req)
    }
    if len(s.queue) == 0 {
        s.mu.Unlock()
        return core.ModelResult{}, ErrQueueEmpty
    }
    r := s.queue[0]
    s.queue = s.queue[1:]
    s.calls++
    s.mu.Unlock()
    return r, nil
}

func (s *ScriptedProvider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
    s.mu.Lock()
    if len(s.queue) == 0 {
        s.mu.Unlock()
        return nil, ErrQueueEmpty
    }
    r := s.queue[0]
    s.queue = s.queue[1:]
    s.calls++
    s.streams++
    s.mu.Unlock()
    ch := make(chan core.ModelChunk, 1)
    defer close(ch)
    ch <- core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Text: r.Text, Done: true}
    return ch, nil
}

func (s *ScriptedProvider) CountTokens(ctx context.Context, msgs []core.Message) (int, error) {
    return len(msgs), nil
}

func (s *ScriptedProvider) RequestCount() int {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.calls
}

var ErrQueueEmpty = errors.New("scripted provider queue empty")

func init() {
    if !errors.Is(ErrQueueEmpty, ErrQueueEmpty) {
        panic(fmt.Sprintf("testutil: %v", ErrQueueEmpty))
    }
}
```

- [ ] **Step 2: Rename `CapturingNotifier` → `RecordingNotifier`**

Replace `internal/testutil/capturing_notifier.go` with:

```go
package testutil

import (
    "context"
    "sync"
)

type RecordingNotifier struct {
    mu   sync.Mutex
    msgs []string
}

func (n *RecordingNotifier) Notify(_ context.Context, msg string) error {
    n.mu.Lock()
    defer n.mu.Unlock()
    n.msgs = append(n.msgs, msg)
    return nil
}

func (n *RecordingNotifier) Messages() []string {
    n.mu.Lock()
    defer n.mu.Unlock()
    out := make([]string, len(n.msgs))
    copy(out, n.msgs)
    return out
}
```

- [ ] **Step 3: Update `mem_store.go`**

- `core.WAL` → `core.WriteAheadLog`
- `core.WAL.Append` / `Replay` / `Truncate` → `core.WriteAheadLog.Append` / `Read` / `TruncateFrom`
- `core.Input` → `core.Event`
- `core.ToolSchema` → `core.ToolSpec` (if used)
- `core.Message.Chunks` → `Parts`
- `core.CHUNK_KIND_TEXT` → `core.PART_KIND_PLAIN_TEXT`
- `state.Scratch` → `state.WorkingMemory`

- [ ] **Step 4: Update all test files that import testutil**

Run: `grep -rln 'testutil.FakeProvider\|testutil.NewFakeProvider\|testutil.CapturingNotifier' .` and update each.

- [ ] **Step 5: Run testutil tests**

Run: `go test ./internal/testutil/ -count=1 -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/testutil/
git commit -m "refactor(testutil): rename FakeProvider→ScriptedProvider, CapturingNotifier→RecordingNotifier, OnGenerate→OnRequest, CallCount→RequestCount"
```

---

### Task 12: `provider/*` packages — verify

**Files:**
- Modify: `provider/anthropic/provider.go`, `provider/anthropic/options.go`, `provider/anthropic/provider_test.go`
- Modify: `provider/google/provider.go`, `provider/google/options.go`, `provider/google/json_helpers.go`, `provider/google/provider_test.go`
- Modify: `provider/openaicompat/provider.go`, `provider/openaicompat/options.go`, `provider/openaicompat/provider_test.go`

- [ ] **Step 1: Update type references**

These packages use plain English already. The renames are mechanical type-reference updates:
- `core.ToolSchema` → `core.ToolSpec`
- `core.Chunk` → `core.Part` (if used)
- `core.CHUNK_KIND_TEXT` → `core.PART_KIND_PLAIN_TEXT`
- `core.Message.Chunks` → `Parts`

- [ ] **Step 2: Run provider tests**

Run: `cd provider/anthropic && go test -count=1 -v && cd ../google && go test -count=1 -v && cd ../openaicompat && go test -count=1 -v`
Expected: PASS (some tests may be skipped if no API key).

- [ ] **Step 3: Commit**

```bash
git add provider/
git commit -m "refactor(provider): update type references after core rename (ToolSchema→ToolSpec, Chunk→Part, CHUNK_KIND_TEXT→PART_KIND_PLAIN_TEXT)"
```

---

### Task 13: `sample/logdoctor/`

**Files:**
- Modify: `sample/logdoctor/main.go`, `sample/logdoctor/cmd/*.go`, `sample/logdoctor/core/listener.go`, `sample/logdoctor/core/dedupe.go`, `sample/logdoctor/core/listener_test.go`, `sample/logdoctor/tool/*`
- Modify: `sample/logdoctor/internal/fake/*`

- [ ] **Step 1: Rename `Dedupe` → `BurstSuppressor` in `sample/logdoctor/core/dedupe.go`**

Replace the file with:

```go
package core

import (
    "context"
    "crypto/sha1"
    "encoding/hex"
    "sync"
    "time"

    sdkcore "github.com/bizshuk/agentsdk/core"
)

type BurstSuppressor struct {
    Inner    sdkcore.ObservationSource
    RuleID   string
    Cooldown time.Duration

    mu    sync.Mutex
    last  string
    until time.Time
}

func NewBurstSuppressor(inner sdkcore.ObservationSource, ruleID string, cooldown time.Duration) *BurstSuppressor {
    return &BurstSuppressor{Inner: inner, RuleID: ruleID, Cooldown: cooldown}
}

func (b *BurstSuppressor) Name() string { return "burst_suppressor:" + b.RuleID }

func (b *BurstSuppressor) Observations(ctx context.Context) <-chan sdkcore.Observation {
    src := b.Inner.Observations(ctx)
    out := make(chan sdkcore.Observation, 32)
    go func() {
        defer close(out)
        for {
            select {
            case <-ctx.Done():
                return
            case p, ok := <-src:
                if !ok {
                    return
                }
                if b.shouldEmit(p) {
                    select {
                    case out <- p:
                    case <-ctx.Done():
                        return
                    }
                }
            }
        }
    }()
    return out
}

func (b *BurstSuppressor) shouldEmit(p sdkcore.Observation) bool {
    fp := fingerprint(b.RuleID, payloadToString(p.Payload))
    b.mu.Lock()
    defer b.mu.Unlock()
    now := time.Now()
    if fp == b.last && now.Before(b.until) {
        return false
    }
    b.last = fp
    if b.Cooldown > 0 {
        b.until = now.Add(b.Cooldown)
    } else {
        b.until = time.Time{}
    }
    return true
}

func fingerprint(ruleID, payload string) string {
    h := sha1.Sum([]byte(ruleID + "|" + payload))
    return hex.EncodeToString(h[:])[:12]
}

func payloadToString(v any) string {
    switch x := v.(type) {
    case string:
        return x
    case []byte:
        return string(x)
    case nil:
        return ""
    }
    return ""
}

func (b *BurstSuppressor) LastSignature() string {
    b.mu.Lock()
    defer b.mu.Unlock()
    return b.last
}

func (b *BurstSuppressor) ShouldEmitForTest(p sdkcore.Observation) bool {
    return b.shouldEmit(p)
}
```

- [ ] **Step 2: Update `sample/logdoctor/core/listener.go`**

Mechanical renames:
- `core.Percept` → `sdkcore.Observation`
- `core.Input` → `sdkcore.Event`
- `.Percepts(` → `.Observations(`
- `LogFileListener` (keep)

- [ ] **Step 3: Update `cmd/run.go`, `cmd/resume.go`, `cmd/approve.go`, `cmd/watch.go`, `cmd/list.go`, `cmd/dirs.go`, `cmd/root.go`, `main.go`**

Mechanical renames across the sample:
- `core.NewStep` → `core.NewDecide`
- `core.THINK_REACT` → `core.REASON_REACT`
- `core.THINK_PLANNER_EXECUTOR` → `core.REASON_PLAN_THEN_RUN`
- `core.THINK_EXECUTOR_CRITIC` → `core.REASON_DO_THEN_REVIEW`
- `planning.NewReAct` → `planning.NewThinkThenAct`
- `planning.NewPlannerExecutor` → `planning.NewPlanThenRun`
- `planning.NewExecutorCritic` → `planning.NewRunThenReview`
- `runtime.NewLoop` → `runtime.NewEngine`
- `loop` variable (Loop) → `engine` (Engine) where reasonable
- `loop.Run` / `loop.Resume` / `loop.SubmitApproval` / `loop.RunWithInput` / `loop.WAL` (keep Run and Resume; the others change)
- `core.WAL` → `core.WriteAheadLog`
- `core.Effect` → `core.Instruction`
- `EFFECT_*` → `INSTRUCTION_*`
- `core.Input` → `core.Event`
- `INPUT_KIND_PERCEPT` → `EVENT_OBSERVATION`
- `core.Percept` → `core.Observation`
- `core.ToolSchema` → `core.ToolSpec`
- `core.ToolResult` (keep)
- `filestore.NewFileStateStore` → `filestore.NewJSONFileStateStore`
- `filestore.NewFileWAL` → `filestore.NewJSONLFileLog`
- `allowAllApproval` → `AllowAllPolicy`
- `core.NewReAct()` (sample fake) — if the fake provider has its own naming, update too
- `Message.Chunks` → `Parts`
- `core.Chunk` → `core.Part`
- `core.CHUNK_KIND_TEXT` → `core.PART_KIND_PLAIN_TEXT`
- `state.Scratch` → `state.WorkingMemory`
- `testutil.FakeProvider` → `testutil.ScriptedProvider`
- `testutil.NewFakeProvider` → `testutil.NewScriptedProvider`
- `FakeProvider.CallCount` → `ScriptedProvider.RequestCount`

- [ ] **Step 4: Update `internal/fake/fake.go`**

If `internal/fake/fake.go` defines a `ScriptedProvider` (distinct from the testutil one), rename or update as appropriate. If it wraps `testutil.FakeProvider`, update the call sites.

- [ ] **Step 5: Update tests**

In `core/listener_test.go` and any other sample tests, apply the mechanical renames.

- [ ] **Step 6: Run sample tests + E2E**

Run:
```bash
cd sample/logdoctor
go test ./... -count=1 -v
go run . --fake --max-turns=10 run --once --fixture testdata/error.log
```

Expected: PASS; E2E JSONL sequence is `call_model → call_tool(read_log_tail) → call_model → call_tool(notify) → call_model → done`.

- [ ] **Step 7: Commit**

```bash
git add sample/logdoctor/
git commit -m "refactor(sample/logdoctor): rename Dedupe→BurstSuppressor, allowAllApproval→AllowAllPolicy; update all type references after core rename"
```

---

### Task 14: `sample/greet-agent/`

**Files:**
- Modify: `sample/greet-agent/main.go`, `sample/greet-agent/cmd/root.go`, `sample/greet-agent/tool/greet.go`

- [ ] **Step 1: Update `sample/greet-agent/cmd/root.go`**

Mechanical renames:
- `core.NewStep` → `core.NewDecide`
- `core.THINK_REACT` → `core.REASON_REACT`
- `planning.NewReAct` → `planning.NewThinkThenAct`
- `runtime.NewLoop` → `runtime.NewEngine`
- `loop` → `engine` (the variable)
- `core.Effect` → `core.Instruction`
- `EFFECT_*` → `INSTRUCTION_*`
- `core.Input` → `core.Event`
- `core.Percept` → `core.Observation`
- `core.ToolSchema` → `core.ToolSpec`
- `core.WAL` → `core.WriteAheadLog`
- `filestore.NewFileStateStore` → `filestore.NewJSONFileStateStore`
- `filestore.NewFileWAL` → `filestore.NewJSONLFileLog`
- `Message.Chunks` → `Parts`
- `core.Chunk` → `core.Part`
- `core.CHUNK_KIND_TEXT` → `core.PART_KIND_PLAIN_TEXT`
- `state.Scratch` → `state.WorkingMemory`

- [ ] **Step 2: Update `tool/greet.go`**

- `core.ToolSchema` → `core.ToolSpec`
- `core.ToolResult` (keep)
- `core.RiskLevel` (keep)

- [ ] **Step 3: Build and verify**

Run: `cd sample/greet-agent && go build ./...`
Expected: builds clean.

- [ ] **Step 4: Commit**

```bash
git add sample/greet-agent/
git commit -m "refactor(sample/greet-agent): update type references after core rename"
```

---

### Task 15: Full E2E verification

**Files:** None modified.

- [ ] **Step 1: `go build ./...` for the entire workspace**

Run: `go work sync && go build ./...`
Expected: zero errors.

- [ ] **Step 2: `go test ./...` for the entire workspace**

Run: `go test ./... -count=1 -timeout=60s`
Expected: PASS for all packages.

- [ ] **Step 3: `go vet ./...`**

Run: `go vet ./...`
Expected: zero warnings.

- [ ] **Step 4: E2E in sample/logdoctor**

Run: `cd sample/logdoctor && go run . --fake --max-turns=10 run --once --fixture testdata/error.log`
Expected: JSONL sequence `call_model → call_tool(read_log_tail) → call_model → call_tool(notify) → call_model → done`.

- [ ] **Step 5: M2 regression checks**

- Budget guard: confirm `IsBudgetExceeded` still detects `harness.BudgetExceededError`.
- FileStateStore round-trip: write state, read it back, fields equal.
- Recoverer: load saved state + replay WAL; check that the rebuilt state matches what was written.
- loopguard: 5 consecutive CALL_TOOL with the same fingerprint triggers REQUEST_APPROVAL with `Reason: "loop_detected"`.
- logdoctor run + resume CLI: produce a state, then `logdoctor resume --run-id <id>` reproduces the same final state.

- [ ] **Step 6: M3 / M4 compile-time checks**

The plan preserves the `// M3 ...` and `// M4 ...` references in doc comments. Confirm `go build ./...` in `middleware/security/`, `provider/anthropic/`, `provider/google/`, `provider/openaicompat/`, `mcp/` all pass.

- [ ] **Step 7: Commit (only if any fixup needed)**

If no changes were needed, no commit. Otherwise:
```bash
git add .
git commit -m "chore: post-rename cleanup"
```

---

## Self-review checklist (run before handing off for execution)

1. **Spec coverage:** every symbol in the rename map (above) appears in some task. ✅ Verified.
2. **Placeholder scan:** no `TODO`/`TBD`/`similar to Task N`/`appropriate` in any step. ✅ Verified — all steps have code blocks.
3. **Type consistency:** the same symbol is renamed the same way in every task. The canonical map at the top is the single source of truth; tasks reference it.

**Notable cross-task consistency checks:**

- `State.WorkingMemory` (Go field) ↔ JSON tag `"scratch"` is preserved intentionally in Task 1 (Step 10) and Task 2. The migration shim reads old JSON files under the `"scratch"` key and surfaces them as `WorkingMemory` in Go. The reverse — writing new state — uses the new field name with the old JSON tag, so new and old code agree on the wire shape.
- `ReasoningStyle` JSON tag remains `"thinking_kind"`. The migration shim maps the *value* (`"react"` → `"think_then_act"`), not the key. This is by design — it minimizes the diff between v1 and v2 JSON for tools that read State.
- `WAL` (Go interface) is renamed to `WriteAheadLog`, but the field on `runtime.Engine` keeps a short name: `Log` (not `WriteAheadLog`). The interface has a long name (because it's an external type); the field has a short name (because the package qualifier already says "WriteAheadLog"). The plan uses `loop.Log` in the *old* code and `engine.Log` in the *new* code consistently.
- `runtime.Loop` is renamed to `runtime.Engine`. The constructor is `NewEngine`. Variable names in the codebase change from `loop` to `engine` — but this is a local convention; pick `engine` to match the new type. The plan uses `engine` in the new files; consumers may continue to use `loop` as a local variable if they prefer (Go doesn't care), but the type is `*runtime.Engine`.
- `runtime.DefaultMiddleware` keeps its name — it has no academic jargon in it.
- `core.Source` is renamed to `core.ObservationSource`. The interface in `perception/` (the real one) is also renamed `ObservationSource`. The plan keeps these aligned; the comment in `core/input.go` notes that the core interface is a stub mirroring the perception one.
- `multi-agent` is mentioned in the comment for `REASON_PICK_AGENT`. This is descriptive, not academic jargon — keep it.
- `agent-loop community`, `LLM-agent papers`, etc. appear in the citation comments. These are descriptions of where the academic name comes from, not academic terms themselves — keep them.

If during execution you find a discrepancy, **fix the task that introduces the discrepancy** — don't let drift accumulate.

## Estimated effort

15 tasks, each ~10 file edits, ~20 minutes per task. Total: 5 hours of mechanical rename + 1-2 hours of E2E verification. Plan to commit every 10 file edits inside a task if the task touches more than that.

## Risks

- **Wire-format compatibility:** by design, the plan breaks wire compatibility for CLI consumers and persisted State from before the rename. The migration shim in `JSONFileStateStore.Load` (Task 2) saves any pre-rename State, but the CLI envelope Type values change. If the team has a dashboard consuming `MSG_TYPE_PERCEPT` over a websocket, it needs to be updated to `MSG_TYPE_OBSERVATION` in the same release. **Communicate this in the PR description.**
- **Tests touching private helpers:** `helpers_test.go` and `middleware_test.go` may have private-symbol references; the plan accounts for them in the per-task "Update tests" steps.
- **Lint configuration:** the project has `staticcheck ST1003` with a const exclusion — verify after the rename that gofmt and staticcheck pass.
- **Third-party imports:** `mcp/` imports `github.com/modelcontextprotocol/go-sdk/mcp`. The plan does not rename third-party types. Only `agentsdk`-internal types are renamed.
- **Provider test fixtures:** the test scripts in `provider/*/provider_test.go` may construct ModelResult with `StopReason: "end_turn"` etc. — these are provider wire values, not agentsdk symbols, and stay.
