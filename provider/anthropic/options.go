package anthropic

import "github.com/anthropics/anthropic-sdk-go"

type config struct {
	apiKey string
	model  anthropic.Model
}

type Option func(*config)

func defaultConfig() config {
	return config{model: "claude-3-5-sonnet-latest"}
}

// WithAPIKey overrides the ANTHROPIC_API_KEY env lookup.
func WithAPIKey(k string) Option { return func(c *config) { c.apiKey = k } }

// WithModel picks the Claude model id.
func WithModel(m string) Option {
	return func(c *config) { c.model = anthropic.Model(m) }
}