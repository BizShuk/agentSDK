// Package runtime is the shell around core.Decide. It dispatches
// instructions to the bound ports (model, tools, store, notifier) and
// folds the results back as Events.
//
// Middleware chain is wired by the caller. For the M2 defaults use:
//
//	loop.Middleware = config.DefaultMiddleware()
//
// M3 / M4 slots in tracing, sandbox, approval, spotlight / sanitizer at
// the appropriate positions without changing the public Engine API.
package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// Emitter is the callback for INSTRUCTION_EMIT — typically wired to a
// websocket writer or other transport that consumes runtime effects.
type Emitter func(core.Instruction)

// Engine is the agent runtime. All fields are required except Middleware
// (nil = middleware.Identity(), a no-op chain), Approval (nil = no approval
// gate), and Emit (nil = drops emit instructions).
// Callers that want the M2 defaults should wire:
//
//	loop.Middleware = config.DefaultMiddleware()
type Engine struct {
	Step       core.Decide
	Model      core.ModelProvider
	Tools      core.ToolRegistry
	Store      core.StateStore
	Log        core.WriteAheadLog
	Approval   core.ApprovalPolicy
	Notifier   core.Notifier
	Emitter    Emitter
	Middleware middleware.Middleware // nil = Identity(); wire config.DefaultMiddleware() for M2 chain

	// chain is the resolved chain — built lazily on first dispatch.
	chain     middleware.Dispatcher
	chainOnce onceFlag
}

// NewEngine is a constructor that accepts nil-able optionals and defaults them.
func NewEngine(step core.Decide, model core.ModelProvider, tools core.ToolRegistry) *Engine {
	return &Engine{Step: step, Model: model, Tools: tools}
}

// resolveChain returns the bound middleware chain, building it once on first
// use. Callers may override Engine.Middleware to inject their own chain.
// When Middleware is nil, the chain defaults to middleware.Identity (no-ops).
// Callers that want retry/timeout/budget/loopguard should wire:
//
//	loop.Middleware = config.DefaultMiddleware()
func (e *Engine) resolveChain() middleware.Middleware {
	if e.Middleware != nil {
		return e.Middleware
	}
	return middleware.Identity()
}

// runInstruction is the terminal Next — what the middleware chain wraps.
// Implements the core.ModelProvider / ToolRegistry calls and other ports.
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
		// Append assistant message with tool_use parts so the next
		// CALL_MODEL sees the full Anthropic-style turn pairing
		// (assistant tool_use → tool result).
		if len(mr.ToolCalls) > 0 || mr.Text != "" {
			var parts []core.Part
			if mr.Text != "" {
				parts = append(parts, core.Part{
					Kind: core.PART_KIND_PLAIN_TEXT,
					Text: mr.Text,
				})
			}
			for _, tc := range mr.ToolCalls {
				parts = append(parts, core.Part{
					Kind: core.PART_KIND_TOOL_USE,
					ToolUse: &core.ToolUseChunk{
						ID:   tc.ID,
						Name: tc.Name,
						Args: tc.Args,
					},
				})
			}
			s.Messages = append(s.Messages, core.Message{
				Role:  core.ROLE_ASSISTANT,
				Parts: parts,
				Ts:    time.Now().UTC(),
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
		// Already emitted by Engine.Run before dispatch. Nothing to do.
		return s, nil, false, nil

	case core.INSTRUCTION_DONE:
		s.Status = core.RUN_STATUS_COMPLETED
		return s, nil, true, nil

	default:
		return s, nil, false, fmt.Errorf("unknown instruction kind: %s", inst.Kind)
	}
}

// buildChain builds the wrapped dispatch function the first time it is
// needed. We capture chain functions inside runtime state so the middleware
// chain can have its own bookkeeping between dispatch calls.
func (e *Engine) buildChain() middleware.Next {
	if !e.chainOnce.value {
		base := middleware.Next(e.runInstruction)
		e.chain = middleware.Dispatcher(e.resolveChain()(base))
		e.chainOnce.value = true
	}
	return middleware.Next(e.chain)
}

// onceFlag is a tiny once-guard type to avoid sync.Once overhead.
type onceFlag struct{ value bool }

// hasDecidedApproval reports whether state carries any PendingApproval
// that has already received a human decision (Decision != ""). Such a
// run was paused and is now ready to resume: the decision must be
// consumed by consumeApprovedPendingCall and the approved tool call
// re-issued.
func hasDecidedApproval(s core.State) bool {
	for _, pa := range s.PendingApprovals {
		if pa.Decision != "" {
			return true
		}
	}
	return false
}

// consumeApprovedPendingCall inspects state.PendingApprovals for any entry
// that has been decided out-of-band (Decision != ""). For an approved
// call it re-seeds the tool call into working memory so the active rule
// (e.g. ReAct's dispatch phase) re-issues it; for a rejected call it
// marks the run DONE. The decided approval is removed from the slice so
// the resume fires exactly once.
//
// This is the seam between out-of-band approval (SubmitHumanDecision /
// the `approve` CLI writing into persisted State) and the pure Decide:
// the rule never sees the approval, only the re-seeded working memory.
func (e *Engine) consumeApprovedPendingCall(s core.State) core.State {
	if len(s.PendingApprovals) == 0 {
		return s
	}
	kept := s.PendingApprovals[:0]
	approved := false
	for _, pa := range s.PendingApprovals {
		if pa.Decision == "" {
			// Still open — leave for a future decision.
			kept = append(kept, pa)
			continue
		}
		// Decided. Consume it.
		if pa.Decision == core.APPROVAL_DECISION_APPROVE && pa.ToolCall != nil {
			if s.WorkingMemory == nil {
				s.WorkingMemory = make(map[string]any, 4)
			}
			s.WorkingMemory["think_then_act.pending_call"] = *pa.ToolCall
			s.WorkingMemory["think_then_act.phase"] = "dispatch"
			// Mark the call as already-approved so the ApprovalGate does
			// not re-intercept it on re-dispatch (which would loop).
			s.WorkingMemory["think_then_act.approved_call_id"] = pa.ToolCall.ID
			s.Status = core.RUN_STATUS_RUNNING
			approved = true
		} else if pa.Decision == core.APPROVAL_DECISION_REJECT {
			s.Status = core.RUN_STATUS_COMPLETED
		}
	}
	s.PendingApprovals = kept
	_ = approved
	return s
}

// Resume loads a paused run by ID and replays the WAL, then continues
// from the last checkpoint. The runtime never re-issues model calls during
// replay — the WAL already contains ModelResult entries.
func (e *Engine) Resume(ctx context.Context, runID string) (core.State, error) {
	if e.Store == nil {
		return core.State{}, fmt.Errorf("resume requires Store")
	}
	s, err := e.Store.Load(ctx, runID)
	if err != nil {
		return core.State{}, err
	}
	// If the run was paused for approval and a decision has since been
	// recorded out-of-band (e.g. by the `approve` CLI), skip WAL replay
	// and drive a fresh step so consumeApprovedPendingCall re-seeds the
	// approved tool call. WAL replay is only for crash recovery, where
	// the persisted State already reflects the last executed step.
	if hasDecidedApproval(s) {
		return e.Run(ctx, s)
	}
	// For M2, replay is delegated to checkpoint.Recoverer if a Log is bound.
	// The simple case is "Load + Run with empty input"; with a Log we
	// can replay prior Events by feeding them in order. We pass them as a
	// queued slice handled by runStep.
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

// SubmitHumanDecision injects an out-of-band HITL decision.
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

// Run drives the run from `state` until a terminal status is reached or
// the context is canceled. The first iteration has no Event — rules
// are expected to read state.Messages / state.PendingApprovals instead.
//
// Returns the final State and any terminal error.
func (e *Engine) Run(ctx context.Context, state core.State) (core.State, error) {
	return e.runStep(ctx, state, core.Event{})
}

// RunWithEvent is exported so tests / sample can drive a one-shot
// deterministic loop with a caller-provided first event.
func (e *Engine) RunWithEvent(ctx context.Context, state core.State, seed core.Event) (core.State, error) {
	return e.runStep(ctx, state, seed)
}

// runStep is the shared engine for Run and RunWithEvent.
func (e *Engine) runStep(ctx context.Context, state core.State, event core.Event) (core.State, error) {
	if state.Budget.StartedAt.IsZero() {
		state.Budget.StartedAt = time.Now().UTC()
	}
	if state.Status == "" {
		state.Status = core.RUN_STATUS_RUNNING
	}

	chain := e.buildChain()
	current := state

	// If the run already reached a terminal status before this step
	// (e.g. a rejected approval set COMPLETED in consumeApprovedPendingCall),
	// honor it immediately rather than running another Decide.
	if current.Status == core.RUN_STATUS_COMPLETED || current.Status == core.RUN_STATUS_FAILED {
		if e.Store != nil {
			_ = e.Store.Save(ctx, current)
		}
		return current, nil
	}

	for {
		// Pre-populate working memory from the incoming event BEFORE Decide runs,
		// so rules can read it via NextStep. Rules are pure and cannot
		// inspect Event directly.
		preStep := current.Clone()
		if preStep.WorkingMemory == nil {
			preStep.WorkingMemory = make(map[string]any, 4)
		}
		// end_turn / max_tokens / error → short-circuit to DONE before
		// Decide sees the event. This avoids the case where ThinkThenAct
		// (or any rule that set its phase to "dispatch" on the prior turn)
		// would otherwise emit DONE on stale working memory.
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
		// HITL resume: if the run was paused for approval and a decision
		// has since been recorded (out-of-band via SubmitHumanDecision or
		// by the `approve` CLI writing into persisted State), re-seed the
		// approved tool call into working memory so ReAct's dispatch phase
		// re-issues it. Rejected approvals short-circuit to DONE. We
		// consume (remove) the decided approval so it fires once.
		preStep = e.consumeApprovedPendingCall(preStep)

		// consumeApprovedPendingCall may have short-circuited the run
		// (rejected approval → COMPLETED). Honor it before running Decide.
		if preStep.Status == core.RUN_STATUS_COMPLETED || preStep.Status == core.RUN_STATUS_FAILED {
			if e.Store != nil {
				_ = e.Store.Save(ctx, preStep)
			}
			return preStep, nil
		}

		next, instructions := e.Step(preStep, event)
		next.Turn = current.Turn + 1
		next.Budget.UsedTurns = next.Turn
		next.UpdatedAt = time.Now().UTC()

		var nextEvent *core.Event
		terminal := false
		for _, inst := range instructions {
			// Backfill the tool's declared risk onto CALL_TOOL
			// instructions before they hit the chain. The model's ToolCall
			// carries no risk; the ApprovalGate needs the tool-defined
			// risk to decide ALLOW vs ASK.
			if inst.Kind == core.INSTRUCTION_CALL_TOOL && inst.CallTool != nil && e.Tools != nil {
				if tool, ok := e.Tools.Get(inst.CallTool.Call.Name); ok {
					inst.CallTool.Call.Risk = tool.Risk()
				}
			}
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
				// Surface BudgetExceededError verbatim — callers can
				// errors.As against harness.BudgetExceededError{}.
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

