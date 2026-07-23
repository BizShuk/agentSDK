// Package registry is the single place that maps a provider name to its
// adapter constructor.
//
// It exists because the name → adapter mapping had two owners: the
// provider smoke-test CLI and every composition root that read a provider
// name from config. Two owners means drift — a newly added adapter that
// works from the CLI but not from a config file, or credential env
// precedence that differs between them.
//
// Environment lookup is INJECTED rather than imported. The CLI resolves
// credentials through gosdk/viper so that .env and config.yaml
// participate; a library caller usually wants plain os.Getenv. Making it
// a field keeps viper out of this package and lets each caller keep its
// own precedence rules.
package registry

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	anthropicprovider "github.com/bizshuk/agentsdk/provider/anthropic"
	googleprovider "github.com/bizshuk/agentsdk/provider/google"
	grokprovider "github.com/bizshuk/agentsdk/provider/grok"
	minimaxprovider "github.com/bizshuk/agentsdk/provider/minimax"
	ollamaprovider "github.com/bizshuk/agentsdk/provider/ollama"
)

// DEFAULT is the provider used when a name is empty. spec.DEFAULT_PROVIDER
// must agree with it; TestSpecDefaultProviderIsRegistered guards the pair,
// since spec is core-only and cannot import this package to check.
const DEFAULT = "minimax"

// Options are the per-construction overrides. Every field is optional:
// an empty field means "let the adapter apply its own environment
// fallback", which is why a zero Options still builds a working provider
// on a machine with the credentials exported.
type Options struct {
	Model   string
	APIKey  string
	BaseURL string

	// APIKeyEnv overrides which environment variable supplies the
	// credential, for deployments that do not use the adapter's
	// conventional name. Empty uses the entry's own list.
	APIKeyEnv string

	// LookupEnv resolves an environment variable. nil means os.Getenv.
	// The CLI passes a viper-backed lookup so .env files participate.
	LookupEnv func(string) string
}

func (o Options) lookup(key string) string {
	if key == "" {
		return ""
	}
	if o.LookupEnv != nil {
		return o.LookupEnv(key)
	}
	return os.Getenv(key)
}

// Resolve fills empty credential fields from the environment, trying the
// entry's key names in order, and returns the result. Anthropic relies on
// the ordering: an OAuth token outranks a long-lived API key when both
// are present.
//
// Exported because "which credential would this actually use" is a
// question callers legitimately ask before building anything — a preflight
// check, or a wizard showing which env var it found.
func (o Options) Resolve(e Entry) Options {
	if o.APIKey == "" {
		keys := e.APIKeyEnv
		if o.APIKeyEnv != "" {
			keys = []string{o.APIKeyEnv}
		}
		for _, k := range keys {
			if v := o.lookup(k); v != "" {
				o.APIKey = v
				break
			}
		}
	}
	if o.BaseURL == "" {
		o.BaseURL = o.lookup(e.BaseURLEnv)
	}
	return o
}

// Factory builds a provider from resolved options.
type Factory func(Options) (core.Provider, error)

// Entry is everything callers need to know about one adapter: how to
// build it, and enough metadata for a CLI listing or a wizard menu.
type Entry struct {
	Name       string
	Label      string
	Note       string
	APIKeyEnv  []string // credential env vars, highest precedence first
	BaseURLEnv string   // endpoint override env var; empty = adapter default only
	New        Factory
}

// entries is the registry proper. Keys are lower-case; Lookup normalizes.
var entries = map[string]Entry{
	"minimax": {
		Name: "minimax", Label: "MiniMax", Note: "default; OpenAI-compatible",
		APIKeyEnv: []string{"MINIMAX_API_KEY"}, BaseURLEnv: "MINIMAX_BASE_URL",
		New: func(o Options) (core.Provider, error) {
			var opts []minimaxprovider.Option
			if o.Model != "" {
				opts = append(opts, minimaxprovider.WithModel(o.Model))
			}
			if o.APIKey != "" {
				opts = append(opts, minimaxprovider.WithAPIKey(o.APIKey))
			}
			if o.BaseURL != "" {
				opts = append(opts, minimaxprovider.WithBaseURL(o.BaseURL))
			}
			return minimaxprovider.New(opts...)
		},
	},
	"anthropic": {
		Name: "anthropic", Label: "Anthropic", Note: "OAuth token outranks API key",
		APIKeyEnv: []string{"ANTHROPIC_OAUTH_TOKEN", "ANTHROPIC_API_KEY"},
		New: func(o Options) (core.Provider, error) {
			var opts []anthropicprovider.Option
			if o.Model != "" {
				opts = append(opts, anthropicprovider.WithModel(o.Model))
			}
			if o.APIKey != "" {
				opts = append(opts, anthropicprovider.WithAPIKey(o.APIKey))
			}
			if o.BaseURL != "" {
				opts = append(opts, anthropicprovider.WithBaseURL(o.BaseURL))
			}
			return anthropicprovider.New(opts...)
		},
	},
	"google": {
		Name: "google", Label: "Google Gemini",
		APIKeyEnv: []string{"GOOGLE_API_KEY"}, BaseURLEnv: "GOOGLE_BASE_URL",
		New: func(o Options) (core.Provider, error) {
			var opts []googleprovider.Option
			if o.Model != "" {
				opts = append(opts, googleprovider.WithModel(o.Model))
			}
			if o.APIKey != "" {
				opts = append(opts, googleprovider.WithAPIKey(o.APIKey))
			}
			if o.BaseURL != "" {
				opts = append(opts, googleprovider.WithBaseURL(o.BaseURL))
			}
			return googleprovider.New(opts...)
		},
	},
	"grok": {
		Name: "grok", Label: "xAI Grok",
		APIKeyEnv: []string{"XAI_API_KEY"}, BaseURLEnv: "XAI_BASE_URL",
		New: func(o Options) (core.Provider, error) {
			var opts []grokprovider.Option
			if o.Model != "" {
				opts = append(opts, grokprovider.WithModel(o.Model))
			}
			if o.APIKey != "" {
				opts = append(opts, grokprovider.WithAPIKey(o.APIKey))
			}
			if o.BaseURL != "" {
				opts = append(opts, grokprovider.WithBaseURL(o.BaseURL))
			}
			return grokprovider.New(opts...)
		},
	},
	"ollama": {
		Name: "ollama", Label: "Ollama", Note: "local; keyless by default",
		APIKeyEnv: []string{"OPENAI_API_KEY"}, BaseURLEnv: "OPENAI_BASE_URL",
		New: func(o Options) (core.Provider, error) {
			var opts []ollamaprovider.Option
			if o.Model != "" {
				opts = append(opts, ollamaprovider.WithModel(o.Model))
			}
			if o.APIKey != "" {
				opts = append(opts, ollamaprovider.WithAPIKey(o.APIKey))
			}
			if o.BaseURL != "" {
				opts = append(opts, ollamaprovider.WithBaseURL(o.BaseURL))
			}
			return ollamaprovider.New(opts...)
		},
	},
}

// Names lists the registered provider names, sorted.
func Names() []string {
	out := make([]string, 0, len(entries))
	for k := range entries {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Entries lists every registry entry, sorted by name — the source a
// wizard menu or a --list-providers listing renders from.
func Entries() []Entry {
	out := make([]Entry, 0, len(entries))
	for _, name := range Names() {
		out = append(out, entries[name])
	}
	return out
}

// Lookup resolves a name to its entry. Matching is case-insensitive and
// trims surrounding space; an empty name resolves to DEFAULT.
func Lookup(name string) (Entry, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = DEFAULT
	}
	e, ok := entries[key]
	return e, ok
}

// New builds the named provider, resolving credentials from Options and
// then the environment.
func New(name string, o Options) (core.Provider, error) {
	e, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("registry: unknown provider %q (registered: %s)",
			name, strings.Join(Names(), ", "))
	}
	p, err := e.New(o.Resolve(e))
	if err != nil {
		return nil, fmt.Errorf("registry: %s provider: %w", e.Name, err)
	}
	return p, nil
}
