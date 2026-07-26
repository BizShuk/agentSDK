// Package provider is the layer that talks to LLM services: the adapter
// contract (Adapter, Metadata), the name → constructor registry, and the
// credential resolution that stands between a config value and a live
// client.
//
// Adapters register themselves from their package init() — this package
// imports no adapter. Each binary decides which adapters to link by
// blank-importing them (or provider/all for the full set); Go's linker
// drops the rest, so a slim binary only pays for the adapters it asked
// for. The set of "registered providers" is therefore a property of the
// linking binary, not of this package.
//
// Registration is one-shot per name. A duplicate Register panics so
// init-time contract violations surface immediately, the way
// database/sql/driver registers do.
//
// Environment lookup is INJECTED rather than imported. The CLI resolves
// credentials through gosdk/viper so that .env and config.yaml
// participate; a library caller usually wants plain os.Getenv. Making it
// a field keeps viper out of this package and lets each caller keep its
// own precedence rules.
package provider

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// DEFAULT_NAME is the registry key used when a name is empty.
//
// It lives here, not in core and not in agent/spec, because it names a
// vendor: core is a pure state machine that must not change when the
// default vendor does, and spec is declarative data that cannot know
// which adapters a binary linked in. An empty Config.Model.Provider
// therefore stays empty through validation and expansion, and is
// resolved here at Lookup time — the one place that can see the linked
// set.
const DEFAULT_NAME = "minimax"

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

	// CredentialKind selects which credential class is consulted. The
	// values are core.CREDENTIAL_KIND_* — they are core vocabulary
	// because they are what core.Provider.AuthSchemes reports.
	//
	// AUTO ("") tries OAuthEnv first, then APIKeyEnv (the legacy
	// precedence). OAUTH restricts to OAuthEnv and returns an error when
	// no OAuth env resolves. APIKEY restricts to APIKeyEnv and returns an
	// error when no API key env resolves. The strict modes catch the case
	// where a stale OAuth token outranks a fresh API key (or vice versa)
	// on a shared machine.
	CredentialKind string
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

// firstEnv returns the first non-empty value found in keys, or "" when
// none resolve. The override (when set) restricts the search to that
// single key.
func (o Options) firstEnv(keys []string, override string) string {
	if override != "" {
		return o.lookup(override)
	}
	for _, k := range keys {
		if v := o.lookup(k); v != "" {
			return v
		}
	}
	return ""
}

// Resolve fills empty credential fields from the environment, honouring
// the strict modes in Options.CredentialKind. The returned Options is
// always usable; the error is non-nil only when a strict mode rejects
// the resolved state.
//
// Precedence (any mode):
//  1. An explicit Options.APIKey wins outright, before any env lookup.
//  2. Otherwise the chosen env class supplies the key.
//  3. Options.BaseURL still resolves from Metadata.BaseURLEnv, independent of CredentialKind.
//
// Exported because "which credential would this actually use" is a
// question callers legitimately ask before building anything — a preflight
// check, or a wizard showing which env var it found.
func (o Options) Resolve(m Metadata) (Options, error) {
	if o.APIKey == "" {
		switch o.CredentialKind {
		case core.CREDENTIAL_KIND_OAUTH:
			if len(m.OAuthEnv) == 0 {
				return o, fmt.Errorf("provider %q: not OAuth-capable (no OAuth env registered)", m.Label)
			}
			if v := o.firstEnv(m.OAuthEnv, o.APIKeyEnv); v != "" {
				o.APIKey = v
			} else {
				return o, fmt.Errorf("provider %q: requires OAuth credential but %v is unset",
					m.Label, m.OAuthEnv)
			}
		case core.CREDENTIAL_KIND_APIKEY:
			if len(m.APIKeyEnv) == 0 {
				return o, fmt.Errorf("provider %q: OAuth-only, does not accept api_key credential", m.Label)
			}
			if v := o.firstEnv(m.APIKeyEnv, o.APIKeyEnv); v != "" {
				o.APIKey = v
			} else {
				return o, fmt.Errorf("provider %q: requires api_key credential but %v is unset",
					m.Label, m.APIKeyEnv)
			}
		default: // "" / "auto" — preserve the legacy precedence
			keys := append(append([]string{}, m.OAuthEnv...), m.APIKeyEnv...)
			if v := o.firstEnv(keys, o.APIKeyEnv); v != "" {
				o.APIKey = v
			}
		}
	}
	if o.BaseURL == "" {
		o.BaseURL = o.lookup(m.BaseURLEnv)
	}
	return o, nil
}

// Factory builds an adapter from resolved options. The provider.Factory
// signature is the public source of truth for what an adapter must
// produce: a provider.Adapter, not just a core.Provider — adapters
// must also expose Name() and Metadata() at runtime.
type Factory func(Options) (Adapter, error)

// Entry is everything callers need to know about one adapter: how to
// build it, and the static registration metadata a CLI listing or
// wizard menu renders from. The post-construction view of the same
// metadata lives on Adapter.Metadata(); the two must agree —
// register.go is responsible for sourcing both from one literal.
type Entry struct {
	Name     string
	Metadata Metadata
	New      Factory
	Catalog  func() []core.ModelSpec
}

// entries is the registry proper. Keys are lower-case; Lookup normalizes.
// Registered adapters populate it from their package init().
var (
	mu      sync.RWMutex
	entries = map[string]Entry{}
)

// Register adds an adapter to the registry. It panics on a duplicate
// name (idiomatic Go for init()-time contract violations — see
// database/sql/driver). Adapters should call this exactly once from
// their package's init().
func Register(e Entry) {
	if e.Name == "" || e.New == nil {
		panic(fmt.Sprintf("provider: Register requires Name and New (got %+v)", e))
	}
	key := strings.ToLower(strings.TrimSpace(e.Name))
	mu.Lock()
	defer mu.Unlock()
	if _, exists := entries[key]; exists {
		panic(fmt.Sprintf("provider %q already registered", e.Name))
	}
	entries[key] = e
}

// Names lists the registered provider names, sorted.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
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
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Entry, 0, len(entries))
	for _, name := range Names() {
		out = append(out, entries[name])
	}
	return out
}

// Lookup resolves a name to its entry. Matching is case-insensitive and
// trims surrounding space; an empty name resolves to DEFAULT_NAME.
func Lookup(name string) (Entry, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = DEFAULT_NAME
	}
	mu.RLock()
	defer mu.RUnlock()
	e, ok := entries[key]
	return e, ok
}

// Catalog returns the bundled model list for a registered provider. The
// returned slice is the adapter's own DefaultCatalog — the registry does
// not synthesize one. Unknown names return false.
func Catalog(name string) ([]core.ModelSpec, bool) {
	e, ok := Lookup(name)
	if !ok || e.Catalog == nil {
		return nil, false
	}
	return e.Catalog(), true
}

// New builds the named adapter, resolving credentials from Options and
// then the environment.
func New(name string, o Options) (Adapter, error) {
	e, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (registered: %s)",
			name, strings.Join(Names(), ", "))
	}
	resolved, err := o.Resolve(e.Metadata)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", e.Name, err)
	}
	p, err := e.New(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", e.Name, err)
	}
	return p, nil
}
