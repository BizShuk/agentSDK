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

// Factory builds an adapter from resolved options. The provider.Factory
// signature is the source of truth for the generate + stream capability
// required of registered adapters.
type Factory func(ResolvedConfig) (Adapter, error)

// Entry is everything callers need to know about one adapter: how to
// build it, the static metadata a CLI listing or wizard menu renders,
// and the bundled model catalog used when live discovery is unavailable.
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
	if e.Metadata.CredentialRequired && resolved.Auth.Token() == "" && o.Decorator == nil {
		return nil, fmt.Errorf("provider %s: credential is required", e.Name)
	}
	p, err := e.New(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", e.Name, err)
	}
	return WithDecorator(e.Name, p, o.Decorator), nil
}
