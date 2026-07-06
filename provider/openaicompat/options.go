package openaicompat

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