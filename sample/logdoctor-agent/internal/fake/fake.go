// Package fake hosts the sample's FakeProvider — a scripted
// ModelProvider that returns a fixed sequence of ModelResults so the
// e2e test can run with no network access.
//
// Mirrors the agentsdk/internal/testutil.ScriptedProvider, but lives in
// the sample so production callers cannot accidentally import test-only code.
package fake

import (
	"context"

	"github.com/bizshuk/agentsdk/core"
)

// ScriptedProvider returns a deterministic transcript for the e2e demo:
//
//  1. tool_use: read_log_tail n=5
//  2. tool_use: notify {message: "log contains 2 ERROR lines"}
//  3. end_turn: "diagnostic complete"
//
// This mirrors the canonical logdoctor flow: read the log, notify the
// operator, then end. The Loop short-circuits to COMPLETED on the end_turn.
type ScriptedProvider struct {
	idx int
}

// NewScriptedProvider returns the canned provider.
func NewScriptedProvider() *ScriptedProvider { return &ScriptedProvider{} }

// ID implements core.Provider.
func (p *ScriptedProvider) ID() string { return "fake-scripted" }

// Name implements core.Provider.
func (p *ScriptedProvider) Name() string { return "fake-scripted" }

// Models implements core.Provider — the scripted provider advertises a
// single deterministic entry so picker UIs can render it.
func (p *ScriptedProvider) Models() []core.ModelSpec {
	return []core.ModelSpec{{
		ID: "fake-scripted", Family: "fake", Reasoning: false,
		Input:         []core.Modality{core.MODALITY_TEXT},
		ContextWindow: 128000, MaxTokens: 4096,
	}}
}

// AuthSchemes implements core.Provider.
func (p *ScriptedProvider) AuthSchemes() []string {
	return []string{"keyless"}
}

// Generate returns the next scripted ModelResult. After the last one
// (end_turn), it keeps returning end_turn so a buggy caller never crashes.
func (p *ScriptedProvider) Generate(_ context.Context, _ core.ModelRequest) (core.ModelResult, error) {
	switch p.idx {
	case 0:
		p.idx++
		return core.ModelResult{
			StopReason: "tool_use",
			ToolCalls: []core.ToolCall{{
				ID:   "c1",
				Name: "read_log_tail",
				Args: map[string]any{"n": 5},
				Risk: core.RISK_LEVEL_LOW,
			}},
		}, nil
	case 1:
		p.idx++
		return core.ModelResult{
			StopReason: "tool_use",
			ToolCalls: []core.ToolCall{{
				ID:   "c2",
				Name: "notify",
				Args: map[string]any{"level": "warn", "message": "log contains ERROR lines"},
				Risk: core.RISK_LEVEL_LOW,
			}},
		}, nil
	default:
		return core.ModelResult{
			StopReason: "end_turn",
			Text:       "diagnostic complete",
		}, nil
	}
}

// Stream implements core.Provider.
func (p *ScriptedProvider) Stream(_ context.Context, _ core.ModelRequest) (<-chan core.ModelChunk, error) {
	ch := make(chan core.ModelChunk, 1)
	defer close(ch)
	ch <- core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Text: "diagnostic complete", Done: true}
	return ch, nil
}

// CountTokens implements core.Provider.
func (p *ScriptedProvider) CountTokens(_ context.Context, msgs []core.Message) (int, error) {
	return len(msgs), nil
}
