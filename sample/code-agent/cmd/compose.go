// compose.go maps the CLI's flags onto an agent.Config and hands the
// assembly to the agent package.
//
// This file used to be the composition root: 333 lines that constructed
// the tool registry, the context-file loader, the skill registry, the
// permission engine, the hook runner, the middleware chain, the session
// manager and the subagent spawner, in an order that mattered and was
// documented nowhere but here. All of that is now agent's job. What is
// left is the part that is genuinely this application's: which flags
// exist, and what they mean.
package cmd

import (
	"context"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// PROJECT_DIR is the project-local harness directory (skills / commands /
// agents), the `.claude` / `.pi` analog.
const PROJECT_DIR = ".agentsdk"

// agentParts is what the mode surfaces need: everything agent assembled,
// plus the opening state Bootstrap seeded.
type agentParts struct {
	*agent.Parts
	seed core.State
}

type composeOptions struct {
	cfg            *agent.AppConfig
	fake           bool
	provider       string
	model          string
	apiKey         string
	baseURL        string
	maxTurns       int
	permissionMode string
}

// compose declares the agent and lets agent.Bootstrap wire it.
func compose(o composeOptions) (*agentParts, error) {
	cfg := agent.Config{
		Name:      appName,
		Tier:      spec.TIER_FULL,
		Model:     agent.Model{Provider: o.provider, Name: o.model, BaseURL: o.baseURL},
		Limits:    spec.Limits{MaxTurns: o.maxTurns},
		Prompt:    &spec.Prompt{ProjectDir: PROJECT_DIR},
		Safety:    &spec.Safety{Mode: o.permissionMode, Sandbox: true, Deny: SAFETY_DENY, Ask: SAFETY_ASK},
		Subagents: &spec.Subagents{MaxTurns: min(o.maxTurns, 10)},
	}

	opts := []agent.Option{agent.WithHooks(blockDestructiveBash())}

	// A literal API key and the fake provider are both things a config
	// file must not carry — one is a secret, the other is a live object.
	// That is exactly the line between Config and Option.
	if p, err := explicitProvider(o); err != nil {
		return nil, err
	} else if p != nil {
		opts = append(opts, agent.WithProvider(p))
	}

	a, err := agent.New(cfg, opts...)
	if err != nil {
		return nil, err
	}
	_, seed, err := a.Bootstrap(context.Background(), o.cfg)
	if err != nil {
		return nil, err
	}
	return &agentParts{Parts: a.Parts(), seed: seed}, nil
}

// explicitProvider returns a provider only when a flag demands one that
// the config cannot express; nil means "let the config decide".
func explicitProvider(o composeOptions) (core.Provider, error) {
	if o.fake {
		return newFakeProvider(), nil
	}
	if o.apiKey == "" {
		return nil, nil
	}
	return provider.New(o.provider, provider.Options{
		Model: o.model, APIKey: o.apiKey, BaseURL: o.baseURL,
	})
}

func userMessage(text string) core.Message {
	return core.Message{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}},
	}
}

// lastAssistantText returns the newest assistant text in the transcript.
func lastAssistantText(s core.State) string { return agent.LastAssistantText(s) }
