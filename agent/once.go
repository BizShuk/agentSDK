package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/reasoning"
)

// Once runs one model call without persistence or a required agent name.
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

// OnceStream is Once with incremental events delivered to fn.
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

func oneshot(cfg Config, prompt string, opts []Option) (*Engine, core.State, error) {
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

	prov, err := b.buildProvider(prepared.Model)
	if err != nil {
		return nil, core.State{}, err
	}

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_ONE_SHOT: reasoning.NewOneShotReasoning(),
	})

	eng := NewEngine(step, prov, nil)
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
