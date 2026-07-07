package openaicompat

import "github.com/spf13/viper"

// config holds Provider construction-time options.
type config struct {
	baseURL string
	apiKey  string
	model   string
}

// Option mutates config during New.
type Option func(*config)

func defaultConfig() config {
	return config{model: "llama3.2"}
}

// WithBaseURL sets the chat-completions endpoint.
func WithBaseURL(u string) Option { return func(c *config) { c.baseURL = u } }

// WithAPIKey sets the bearer token (empty for local key-less hosts).
func WithAPIKey(k string) Option { return func(c *config) { c.apiKey = k } }

// WithModel picks the model id passed to /chat/completions.
func WithModel(m string) Option { return func(c *config) { c.model = m } }

// WithViper reads configuration keys from a viper instance to fill in
// gaps before explicit With*() options. Keys:
//
//	openai.base_url
//	openai.api_key
//	openai.model
//
// Example with gosdk config.Default(WithAppName("myapp")):
//
//	// ~/.config/myapp/settings.json:
//	// {"openai": {"base_url": "http://localhost:11434/v1", "model": "llama3.2"}}
//
//	provider, _ := openaicompat.New(openaicompat.WithViper(viper.GetViper()))
//
// Place WithViper before explicit With*() options so the latter win:
//
//	openaicompat.New(openaicompat.WithViper(v), openaicompat.WithModel("qwen3"))
func WithViper(v *viper.Viper) Option {
	return func(c *config) {
		if baseURL := v.GetString("openai.base_url"); baseURL != "" {
			c.baseURL = baseURL
		}
		if apiKey := v.GetString("openai.api_key"); apiKey != "" {
			c.apiKey = apiKey
		}
		if model := v.GetString("openai.model"); model != "" {
			c.model = model
		}
	}
}