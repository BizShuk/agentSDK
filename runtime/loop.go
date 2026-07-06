// Package runtime is the shell around core.Step. It dispatches effects
// to the bound ports (model, tools, store, notifier) and folds the
// results back as Inputs.
//
// M2 integrates the middleware chain: Retry / Timeout / Budget / Loopguard
// wrap the dispatcher (retry → timeout → budget → loopguard → base).
// M3 / M4 slots in tracing, sandbox, approval, spotlight / sanitizer at
// the appropriate positions without changing the public Loop API.
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

// Emitter is the callback for EFFECT_EMIT — typically wired to cli.Codec
// or a websocket writer.
type Emitter func(core.Effect)

// Loop is the agent runtime. All fields are required except Middleware
// (nil = uses DefaultMiddleware: retry → timeout → budget → loopguard),
// Approval (nil = no approval gate), and Emit (nil = drops emit effects).
type Loop struct {
	Step       core.Step
	Model      core.ModelProvider
	Tools      core.ToolRegistry
	Store      core.StateStore
	WAL        core.WAL
	Approval   core.ApprovalPolicy
	Notifier   core.Notifier
	Emitter    Emitter
	Middleware middleware.Middleware // overrides DefaultMiddleware when set

	// chain is the resolved chain — built lazily on first dispatch.
	chain     middleware.Dispatcher
	chainOnce boolError
}

// NewLoop is a constructor that accepts nil-able optionals and defaults them.
func NewLoop(step core.Step, model core.ModelProvider, tools core.ToolRegistry) *Loop {
	return &Loop{Step: step, Model: model, Tools: tools}
}

// DefaultMiddleware returns the M2 chain: retry → timeout → budget → loopguard.
// Order matches plans/plan-only-and-plan-breezy-pike.md.
func DefaultMiddleware() middleware.Middleware {
	return middleware.Chain(
		harness.Retry(harness.RetryConfig{N: 3, BaseBackoff: 100 * time.Millisecond, MaxBackoff: 5 * time.Second}),
		harness.Timeout(harness.TimeoutConfig{PerEffect: 60 * time.Second}),
		harness.Budget(),
		loopguard.New(loopguard.Config{MaxRepeats: 5}),
	)
}

// resolve returns the bound middleware chain, building it once on first
// use. Callers may override Loop.Middleware to inject their own chain.
func (l *Loop) resolve() middleware.Middleware {
	if l.Middleware != nil {
		return l.Middleware
	}
	return DefaultMiddleware()
}

// dispatch is the terminal Next — what the middleware chain wraps.
// Implements the core.ModelProvider / ToolRegistry calls and other ports.
func (l *Loop) dispatch(ctx context.Context, s core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
	switch eff.Kind {
	case core.EFFECT_CALL_MODEL:
		if eff.CallModel == nil {
			return s, nil, false, fmt.Errorf("call_model effect missing CallModel")
		}
		if l.Model == nil {
			return s, nil, false, fmt.Errorf("call_model effect but no Model bound")
		}
		mr, err := l.Model.Generate(ctx, core.ModelRequest{
			Messages: eff.CallModel.Messages,
			Tools:    eff.CallModel.Tools,
		})
		if err != nil {
			return s, nil, false, fmt.Errorf("model generate: %w", err)
		}
		return s, &core.Input{
			Kind:        core.INPUT_KIND_MODEL_RESULT,
			ModelResult: &mr,
			ReceivedAt:  time.Now().UTC(),
		}, false, nil

	case core.EFFECT_CALL_TOOL:
		if eff.CallTool == nil {
			return s, nil, false, fmt.Errorf("call_tool effect missing CallTool")
		}
		if l.Tools == nil {
			return s, nil, false, fmt.Errorf("call_tool effect but no Tools bound")
		}
		res := l.Tools.Call(ctx, eff.CallTool.Call)
		chunkOut := core.ToolResultChunk{
			CallID: res.CallID, Name: res.Name, OK: res.OK,
			Output: res.Output, Error: res.Error,
		}
		s.Messages = append(s.Messages, core.Message{
			Role: core.ROLE_TOOL,
			Chunks: []core.Chunk{
				{Kind: core.CHUNK_KIND_TOOL_RESULT, ToolResult: &chunkOut},
			},
			Ts: time.Now().UTC(),
		})
		return s, &core.Input{
			Kind:       core.INPUT_KIND_TOOL_RESULT,
			ToolResult: &res,
			ReceivedAt: time.Now().UTC(),
		}, false, nil

	case core.EFFECT_REQUEST_APPROVAL:
		if eff.RequestApproval == nil {
			return s, nil, false, fmt.Errorf("request_approval effect missing payload")
		}
		pa := core.PendingApproval{
			ID:          eff.RequestApproval.ApprovalID,
			Reason:      eff.RequestApproval.Reason,
			Risk:        eff.RequestApproval.Risk,
			Summary:     eff.RequestApproval.Summary,
			ToolCall:    eff.RequestApproval.ToolCall,
			RequestedAt: time.Now().UTC(),
		}
		s.PendingApprovals = append(s.PendingApprovals, pa)
		s.Status = core.RUN_STATUS_PAUSED_APPROVAL
		return s, nil, true, nil

	case core.EFFECT_NOTIFY:
		if eff.Notify != nil && l.Notifier != nil {
			_ = l.Notifier.Notify(ctx, fmt.Sprintf("[%s] %s", eff.Notify.Level, eff.Notify.Message))
		}
		return s, nil, false, nil

	case core.EFFECT_CHECKPOINT:
		if l.Store != nil {
			_ = l.Store.Save(ctx, s)
		}
		return s, nil, false, nil

	case core.EFFECT_EMIT:
		// Already emitted by Loop.Run before dispatch. Nothing to do.
		return s, nil, false, nil

	case core.EFFECT_DONE:
		s.Status = core.RUN_STATUS_COMPLETED
		return s, nil, true, nil

	default:
		return s, nil, false, fmt.Errorf("unknown effect kind: %s", eff.Kind)
	}
}

// ensureChain builds the wrapped dispatch function the first time it is
// needed. We capture chain functions inside runtime state so the middleware
// chain can have its own bookkeeping between dispatch calls.
func (l *Loop) ensureChain() middleware.Next {
	if !l.chainOnce.value {
		base := middleware.Next(l.dispatch)
		l.chain = middleware.Dispatcher(l.resolve()(base))
		l.chainOnce.value = true
	}
	return middleware.Next(l.chain)
}

// boolError is a tiny once-guard type to avoid sync.Once overhead.
type boolError struct{ value bool }

// Run drives the run from `state` until a terminal status is reached or
// the context is canceled. The first iteration has no Input — patterns
// are expected to read state.Messages / state.PendingApprovals instead.
//
// Returns the final State and any terminal error.
func (l *Loop) Run(ctx context.Context, state core.State) (core.State, error) {
	return l.runWithInput(ctx, state, core.Input{})
}

// Resume loads a paused run by ID and replays the WAL, then continues
// from the last checkpoint. The runtime never re-issues model calls during
// replay — the WAL already contains ModelResult entries.
func (l *Loop) Resume(ctx context.Context, runID string) (core.State, error) {
	if l.Store == nil {
		return core.State{}, fmt.Errorf("resume requires Store")
	}
	s, err := l.Store.Load(ctx, runID)
	if err != nil {
		return core.State{}, err
	}
	// For M2, replay is delegated to checkpoint.Recover if a WAL is bound.
	// The simple case is "Load + Run with empty input"; with a WAL we
	// can replay prior Inputs by feeding them in order. We pass them as a
	// queued slice handled by runWithInput.
	if l.WAL == nil {
		return l.Run(ctx, s)
	}
	inputs, err := l.WAL.Replay(ctx, runID, s.LastInputSeq)
	if err != nil {
		return core.State{}, fmt.Errorf("resume replay: %w", err)
	}
	for _, in := range inputs {
		var err error
		s, err = l.runWithInput(ctx, s, in)
		if err != nil {
			return s, err
		}
	}
	return s, nil
}

// SubmitApproval injects an out-of-band HITL decision.
func (l *Loop) SubmitApproval(ctx context.Context, runID string, decision core.ApprovalDecision, decidedBy string) (core.State, error) {
	if l.Store == nil {
		return core.State{}, fmt.Errorf("submit approval requires Store")
	}
	s, err := l.Store.Load(ctx, runID)
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
	if err := l.Store.Save(ctx, s); err != nil {
		return core.State{}, err
	}
	input := core.Input{
		Kind:             core.INPUT_KIND_APPROVAL_DECISION,
		ApprovalDecision: &decision,
		ReceivedAt:       time.Now().UTC(),
	}
	return l.runWithInput(ctx, s, input)
}

// RunWithInput is exported so tests / sample can drive a one-shot
// deterministic loop with a caller-provided first input.
func (l *Loop) RunWithInput(ctx context.Context, state core.State, seed core.Input) (core.State, error) {
	return l.runWithInput(ctx, state, seed)
}

// runWithInput is the shared engine for Run and RunWithInput.
func (l *Loop) runWithInput(ctx context.Context, state core.State, input core.Input) (core.State, error) {
	if state.Budget.StartedAt.IsZero() {
		state.Budget.StartedAt = time.Now().UTC()
	}
	if state.Status == "" {
		state.Status = core.RUN_STATUS_RUNNING
	}

	chain := l.ensureChain()
	current := state

	for {
		// Pre-populate scratch from the incoming input BEFORE Step runs, so
		// patterns can read it via Decide. Patterns are pure and cannot
		// inspect Input directly.
		preStep := current.Clone()
		if preStep.Scratch == nil {
			preStep.Scratch = make(map[string]any, 4)
		}
		// end_turn / max_tokens / error → short-circuit to DONE before Step
		// sees the input. This avoids the case where ReAct (or any pattern
		// that set its phase to "act" on the prior turn) would otherwise
		// emit DONE on stale scratch.
		if input.ModelResult != nil && len(input.ModelResult.ToolCalls) == 0 {
			preStep.Status = core.RUN_STATUS_COMPLETED
			if l.Store != nil {
				_ = l.Store.Save(ctx, preStep)
			}
			return preStep, nil
		}
		if input.ModelResult != nil && len(input.ModelResult.ToolCalls) > 0 {
			preStep.Scratch["react.last_call_id"] = input.ModelResult.ToolCalls[0]
		}
		if input.ToolResult != nil {
			preStep.Scratch["react.last_result_signature"] = input.ToolResult.CallID
		}

		next, effects := l.Step(preStep, input)
		next.Turn = current.Turn + 1
		next.Budget.UsedTurns = next.Turn
		next.UpdatedAt = time.Now().UTC()

		var nextInput *core.Input
		terminal := false
		for _, eff := range effects {
			if l.Emitter != nil {
				l.Emitter(eff)
			}
			updated, out, term, err := chain(ctx, next, eff)
			if err != nil {
				next.Status = core.RUN_STATUS_FAILED
				next.UpdatedAt = time.Now().UTC()
				if l.Store != nil {
					_ = l.Store.Save(ctx, next)
				}
				// Surface BudgetExceededError verbatim — callers can
				// errors.As against harness.BudgetExceededError{}.
				return next, err
			}
			next = updated
			if term {
				terminal = true
				if out != nil {
					nextInput = out
				}
				break
			}
			if out != nil && nextInput == nil {
				nextInput = out
			}
		}

		if l.Store != nil {
			_ = l.Store.Save(ctx, next)
		}

		if terminal {
			if next.Status == "" {
				next.Status = core.RUN_STATUS_PAUSED_APPROVAL
			}
			return next, nil
		}

		if nextInput == nil {
			next.Status = core.RUN_STATUS_COMPLETED
			if l.Store != nil {
				_ = l.Store.Save(ctx, next)
			}
			return next, nil
		}

		if l.WAL != nil {
			_ = l.WAL.Append(ctx, next.RunID, next.LastInputSeq+1, *nextInput)
			next.LastInputSeq++
		}

		current = next
		input = *nextInput
	}
}

// IsBudgetExceeded is a small helper for callers that want to detect the
// loop exiting for budget reasons. errors.As(err, &harness.BudgetExceededError{})
// is the canonical check.
func IsBudgetExceeded(err error) bool {
	var be *harness.BudgetExceededError
	return errors.As(err, &be)
}