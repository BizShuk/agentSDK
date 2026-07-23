package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/provider/registry"
	"github.com/bizshuk/agentsdk/runtime"
)

// Once runs a single prompt and returns the assistant's reply.
//
// It is a facade over the one-shot tier, NOT a second code path: the call
// still goes through core.Decide and runtime.Engine, with every optional
// port left nil. Nil is no-op throughout the engine, so the cost is one
// State and one loop iteration, and the payoff is that climbing to a
// higher tier changes configuration rather than API.
//
// Do not be tempted to replace this with a no-op DecisionRule that emits
// [CALL_MODEL, DONE] every step. That breaks core.Decide's pure-function
// invariant — a retry or a WAL replay would re-issue the model call.
// planning.OneShotReasoning already solves it with a two-phase FSM: think
// fires exactly once, then every subsequent step returns DONE.
//
// The config is prepared with tier forced to oneshot, so persistence stays
// off and Name is not required. A caller who wants a trace should build a
// full Agent instead.
func Once(ctx context.Context, cfg Config, prompt string, opts ...Option) (string, error) {
	eng, state, err := oneshot(cfg, prompt, opts)
	if err != nil {
		return "", err
	}
	final, err := eng.Run(ctx, state)
	if err != nil {
		return "", fmt.Errorf("agent: once: %w", err)
	}
	return LastAssistantText(final), nil
}

// OnceStream is Once with incremental delivery: fn receives each stream
// event as the engine emits it, and the assembled reply is returned at
// the end.
//
// fn runs on the engine's goroutine, so it must not block for long.
func OnceStream(ctx context.Context, cfg Config, prompt string, fn func(core.StreamEvent), opts ...Option) (string, error) {
	eng, state, err := oneshot(cfg, prompt, opts)
	if err != nil {
		return "", err
	}
	if fn != nil {
		eng.Sink = SinkFunc(fn)
	}
	final, err := eng.Run(ctx, state)
	if err != nil {
		return "", fmt.Errorf("agent: once stream: %w", err)
	}
	return LastAssistantText(final), nil
}

// oneshot builds the engine and opening state for a single call.
//
// M4 will fold this into the full build pipeline; until then it is the
// pipeline's first two stages (config → provider) plus a minimal
// assembly, kept deliberately small so the refactor is a deletion.
func oneshot(cfg Config, prompt string, opts []Option) (*runtime.Engine, core.State, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, core.State{}, fmt.Errorf("agent: once: prompt must not be empty")
	}

	var b builder
	if err := b.apply(opts); err != nil {
		return nil, core.State{}, err
	}

	cfg.Tier = spec.TIER_ONESHOT
	prepared, err := cfg.Prepare()
	if err != nil {
		return nil, core.State{}, err
	}

	prov, err := resolveProvider(prepared, b)
	if err != nil {
		return nil, core.State{}, err
	}

	// One-shot always runs the one-shot rule, whatever style the config
	// carries: Once is by definition a single call. A config that wants a
	// multi-step strategy should build an Agent instead — which is why
	// spec treats tier and reasoning as orthogonal rather than rejecting
	// the combination.
	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_ONE_SHOT: planning.NewOneShotReasoning(),
	})

	// Tools nil: with no tool specs the model cannot emit a tool call, so
	// the engine short-circuits to COMPLETED after the single reply.
	eng := runtime.NewEngine(step, prov, nil)
	eng.Sink = b.sink
	eng.Notifier = b.notifier

	state := core.State{
		RunID:          fmt.Sprintf("once-%d", time.Now().UnixNano()),
		ReasoningStyle: core.REASON_ONE_SHOT,
		Budget:         core.Budget{MaxTurns: prepared.Limits.MaxTurns},
	}
	if persona := strings.TrimSpace(prepared.Persona); persona != "" {
		state.Messages = append(state.Messages, message(core.ROLE_SYSTEM, persona))
	}
	state.Messages = append(state.Messages, message(core.ROLE_USER, prompt))

	return eng, state, nil
}

// message wraps text as a single-part message.
func message(role core.Role, text string) core.Message {
	return core.Message{
		Role:  role,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}},
		Ts:    time.Now().UTC(),
	}
}

// LastAssistantText returns the newest assistant text in a transcript, or
// "" when the run produced none.
func LastAssistantText(s core.State) string {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role != core.ROLE_ASSISTANT {
			continue
		}
		var sb strings.Builder
		for _, p := range s.Messages[i].Parts {
			if p.Kind == core.PART_KIND_PLAIN_TEXT && p.Text != "" {
				sb.WriteString(p.Text)
			}
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}
	return ""
}

// resolveProvider picks the model provider: an injected one wins outright,
// otherwise the registry builds the configured adapter. Keeping this in
// one function means Once and the full builder cannot diverge on
// precedence.
func resolveProvider(cfg Config, b builder) (core.Provider, error) {
	if b.provider != nil {
		return b.provider, nil
	}
	return registry.New(cfg.Model.Provider, registry.Options{
		Model:     cfg.Model.Name,
		BaseURL:   cfg.Model.BaseURL,
		APIKeyEnv: cfg.Model.APIKeyEnv,
	})
}
