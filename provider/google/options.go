package google

// config holds Provider construction-time options.
type config struct {
	apiKey string
	model  string
}

// Option mutates config during New.
type Option func(*config)

func defaultConfig() config {
	return config{model: "gemini-2.0-flash"}
}

// WithAPIKey overrides the GOOGLE_API_KEY env lookup.
func WithAPIKey(k string) Option {
	return func(c *config) { c.apiKey = k }
}

// WithModel picks the Gemini model id.
func WithModel(m string) Option {
	return func(c *config) { c.model = m }
}