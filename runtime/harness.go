// Harness-facing engine surface: lifecycle hooks, stream event sink, and
// the pi-style steering / follow-up queues. All of it is optional — a nil
// Hooks / Sink and empty queues leave runStep behavior unchanged.
package runtime

import (
	"context"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// Steer queues a user message that is injected into the conversation
// before the next Decide iteration — "talk to the agent while it works".
// Safe for concurrent use with a running engine.
func (e *Engine) Steer(msg string) {
	if msg == "" {
		return
	}
	e.queueMu.Lock()
	defer e.queueMu.Unlock()
	e.steerQueue = append(e.steerQueue, msg)
}

// FollowUp queues a user message delivered when the run would otherwise
// complete: instead of finishing, the engine appends the message and keeps
// going — one queued message per would-be completion.
func (e *Engine) FollowUp(msg string) {
	if msg == "" {
		return
	}
	e.queueMu.Lock()
	defer e.queueMu.Unlock()
	e.followUpQueue = append(e.followUpQueue, msg)
}

// drainSteering returns and clears every queued steering message.
func (e *Engine) drainSteering() []string {
	e.queueMu.Lock()
	defer e.queueMu.Unlock()
	out := e.steerQueue
	e.steerQueue = nil
	return out
}

// popFollowUp pops the oldest follow-up message, if any.
func (e *Engine) popFollowUp() (string, bool) {
	e.queueMu.Lock()
	defer e.queueMu.Unlock()
	if len(e.followUpQueue) == 0 {
		return "", false
	}
	msg := e.followUpQueue[0]
	e.followUpQueue = e.followUpQueue[1:]
	return msg, true
}

// fireHook is the nil-safe hook dispatch. A Fire error is treated as hook
// infrastructure failure, not a block — the zero decision is returned so
// the run proceeds.
func (e *Engine) fireHook(ctx context.Context, ev core.HookEvent) core.HookDecision {
	if e.Hooks == nil {
		return core.HookDecision{}
	}
	dec, err := e.Hooks.Fire(ctx, ev)
	if err != nil {
		return core.HookDecision{}
	}
	return dec
}

// emitStream is the nil-safe sink dispatch.
func (e *Engine) emitStream(ev core.StreamEvent) {
	if e.Sink == nil {
		return
	}
	e.Sink.OnStreamEvent(ev)
}

// emitFolded translates a folded loop Event into presentation StreamEvents.
func (e *Engine) emitFolded(s core.State, ev *core.Event) {
	if e.Sink == nil || ev == nil {
		return
	}
	switch {
	case ev.ModelResult != nil:
		if ev.ModelResult.Text != "" {
			e.emitStream(core.StreamEvent{
				Kind: core.STREAM_MESSAGE, RunID: s.RunID, Turn: s.Turn,
				Text: ev.ModelResult.Text,
			})
		}
		for i := range ev.ModelResult.ToolCalls {
			tc := ev.ModelResult.ToolCalls[i]
			e.emitStream(core.StreamEvent{
				Kind: core.STREAM_TOOL_START, RunID: s.RunID, Turn: s.Turn,
				ContentIndex: i, ToolCall: &tc,
			})
		}
	case ev.ToolResult != nil:
		e.emitStream(core.StreamEvent{
			Kind: core.STREAM_TOOL_RESULT, RunID: s.RunID, Turn: s.Turn,
			ToolResult: ev.ToolResult,
		})
	}
}

// userMessage wraps plain text as a user-role Message.
func userMessage(text string) core.Message {
	return core.Message{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}},
		Ts:    time.Now().UTC(),
	}
}

// blockedToolResult synthesizes the failed result a hook-blocked call folds
// back as, so the model sees the refusal instead of a silent skip.
func blockedToolResult(call core.ToolCall, reason string) core.ToolResult {
	if reason == "" {
		reason = "blocked by hook"
	} else {
		reason = "blocked by hook: " + reason
	}
	return core.ToolResult{CallID: call.ID, Name: call.Name, OK: false, Error: reason}
}

// skippedToolResult is the result of a call that never ran. It is kept
// distinct from blockedToolResult so the model can tell "policy refused
// this" apart from "there was no room for this".
func skippedToolResult(call core.ToolCall, reason string) core.ToolResult {
	return core.ToolResult{CallID: call.ID, Name: call.Name, OK: false, Error: reason}
}

// settleSkipped closes out tool calls that will never dispatch.
//
// The invariant it protects: an assistant message carrying N tool_use
// parts must be followed by N tool_result messages before the next
// CALL_MODEL. Anthropic-format providers reject the request otherwise,
// and every model reads a missing result as "still running", so it
// re-requests the same operations next round. A pause, a tool-call
// budget trip, or any terminal instruction mid-batch leaves calls
// unrun — each one gets an explicit failed result instead of vanishing.
func settleSkipped(s core.State, calls []core.ToolCall, reason string) core.State {
	for _, c := range calls {
		s = appendToolResultMessage(s, skippedToolResult(c, reason))
	}
	return s
}

// settleUnrun is settleSkipped over the tail of an instruction slice,
// for the mid-batch case where the remaining work is still in
// instruction form. Non-CALL_TOOL instructions carry no transcript
// obligation and are ignored.
func settleUnrun(s core.State, rest []core.Instruction, reason string) core.State {
	calls := make([]core.ToolCall, 0, len(rest))
	for _, inst := range rest {
		if inst.Kind == core.INSTRUCTION_CALL_TOOL && inst.CallTool != nil {
			calls = append(calls, inst.CallTool.Call)
		}
	}
	return settleSkipped(s, calls, reason)
}

// appendToolResultMessage mirrors runInstruction's CALL_TOOL fold for
// synthesized (hook-blocked) results.
func appendToolResultMessage(s core.State, res core.ToolResult) core.State {
	s.Messages = append(s.Messages, core.Message{
		Role: core.ROLE_TOOL,
		Parts: []core.Part{{
			Kind: core.PART_KIND_TOOL_RESULT,
			ToolResult: &core.ToolResultPart{
				CallID: res.CallID, Name: res.Name, OK: res.OK,
				Output: res.Output, Error: res.Error,
			},
		}},
		Ts: time.Now().UTC(),
	})
	return s
}
