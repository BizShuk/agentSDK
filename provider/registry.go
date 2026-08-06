// Package provider is the layer that talks to model services: model and media
// capability contracts, the name → constructor registry, and the credential
// resolution that stands between a config value and a live client.
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

// Factory builds a model adapter from resolved options. Its signature is the
// source of truth for the paired generate + stream capability.
type Factory func(ResolvedConfig) (Adapter, error)

// Entry is everything callers need to know about one provider: how to build
// each supported capability, the metadata a CLI or wizard renders, and the
// bundled model catalog used when live discovery is unavailable.
type Entry struct {
	Name           string
	Metadata       Metadata
	New            Factory
	NewImage       ImageFactory
	NewVideo       VideoFactory
	NewMusic       MusicFactory
	NewTranscriber TranscriberFactory
	NewSpeech      SpeechFactory
	NewLive        LiveFactory
	NewTranslate   TranslateFactory
	Catalog        func() []ModelSpec
}

// entries is the registry proper. Keys are lower-case; Lookup normalizes.
// Registered adapters populate it from their package init().
var (
	mu      sync.RWMutex
	entries = map[string]Entry{}
)

// Register adds a provider entry to the registry. It panics on a duplicate
// name (idiomatic Go for init()-time contract violations — see
// database/sql/driver). Providers should call this exactly once from
// their package's init().
func Register(e Entry) {
	if strings.TrimSpace(e.Name) == "" || !e.hasFactory() {
		panic(fmt.Sprintf(
			"provider: Register requires Name and at least one factory (got %+v)",
			e,
		))
	}
	key := strings.ToLower(strings.TrimSpace(e.Name))
	mu.Lock()
	defer mu.Unlock()
	if _, exists := entries[key]; exists {
		panic(fmt.Sprintf("provider %q already registered", e.Name))
	}
	entries[key] = e
}

// hasFactory reports whether the entry can construct at least one capability.
// Model generation is not among the required ones: an audio-only vendor has
// no chat surface to expose, and demanding one would keep it out of the
// registry that discovery, credential resolution, and the CLI all read from.
func (e Entry) hasFactory() bool {
	return e.New != nil ||
		e.NewImage != nil ||
		e.NewVideo != nil ||
		e.NewMusic != nil ||
		e.NewTranscriber != nil ||
		e.NewSpeech != nil ||
		e.NewLive != nil ||
		e.NewTranslate != nil
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
	for _, entry := range entries {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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
func Catalog(name string) ([]ModelSpec, bool) {
	e, ok := Lookup(name)
	if !ok || e.Catalog == nil {
		return nil, false
	}
	return e.Catalog(), true
}

// New builds the named adapter, resolving credentials from Options and
// then the environment.
func New(name string, o Options) (Adapter, error) {
	e, err := lookup(name)
	if err != nil {
		return nil, err
	}
	if e.New == nil {
		return nil, &UnsupportedCapabilityError{
			Provider:   e.Name,
			Capability: CAPABILITY_CHAT,
		}
	}
	resolved, decorator, err := resolveOptions(e, o)
	if err != nil {
		return nil, err
	}
	p, err := e.New(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", e.Name, err)
	}
	return WithDecorator(e.Name, p, decorator), nil
}

// NewImage builds the named provider's image-generation capability using the
// same credential resolution and request-time decorator precedence as New.
func NewImage(name string, o Options) (ImageGenerator, error) {
	e, err := lookup(name)
	if err != nil {
		return nil, err
	}
	if e.NewImage == nil {
		return nil, &UnsupportedCapabilityError{
			Provider:   e.Name,
			Capability: CAPABILITY_IMAGE,
		}
	}
	resolved, decorator, err := resolveImageOptions(e, o)
	if err != nil {
		return nil, err
	}
	generator, err := e.NewImage(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: image: %w", e.Name, err)
	}
	return WithImageDecorator(e.Name, generator, decorator), nil
}

// NewVideo builds the named provider's video-generation capability using the
// same credential resolution and request-time decorator precedence as New.
func NewVideo(name string, o Options) (VideoGenerator, error) {
	e, err := lookup(name)
	if err != nil {
		return nil, err
	}
	if e.NewVideo == nil {
		return nil, &UnsupportedCapabilityError{
			Provider:   e.Name,
			Capability: CAPABILITY_VIDEO,
		}
	}
	resolved, decorator, err := resolveVideoOptions(e, o)
	if err != nil {
		return nil, err
	}
	generator, err := e.NewVideo(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: video: %w", e.Name, err)
	}
	return WithVideoDecorator(e.Name, generator, decorator), nil
}

// NewMusic builds the named provider's music-generation capability using the
// same credential resolution and request-time decorator precedence as New.
func NewMusic(name string, o Options) (MusicGenerator, error) {
	e, err := lookup(name)
	if err != nil {
		return nil, err
	}
	if e.NewMusic == nil {
		return nil, &UnsupportedCapabilityError{
			Provider:   e.Name,
			Capability: CAPABILITY_MUSIC,
		}
	}
	resolved, decorator, err := resolveMusicOptions(e, o)
	if err != nil {
		return nil, err
	}
	generator, err := e.NewMusic(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: music: %w", e.Name, err)
	}
	return WithMusicDecorator(e.Name, generator, decorator), nil
}

// NewTranscriber builds the named provider's speech-to-text capability using
// the same credential resolution and request-time decorator precedence as New.
func NewTranscriber(name string, o Options) (Transcriber, error) {
	e, err := lookup(name)
	if err != nil {
		return nil, err
	}
	if e.NewTranscriber == nil {
		return nil, &UnsupportedCapabilityError{
			Provider:   e.Name,
			Capability: CAPABILITY_TRANSCRIBE,
		}
	}
	resolved, decorator, err := resolveOptions(e, o)
	if err != nil {
		return nil, err
	}
	transcriber, err := e.NewTranscriber(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: transcribe: %w", e.Name, err)
	}
	return WithTranscriberDecorator(e.Name, transcriber, decorator), nil
}

// NewSpeech builds the named provider's text-to-speech capability using the
// same credential resolution and request-time decorator precedence as New.
func NewSpeech(name string, o Options) (SpeechGenerator, error) {
	e, err := lookup(name)
	if err != nil {
		return nil, err
	}
	if e.NewSpeech == nil {
		return nil, &UnsupportedCapabilityError{
			Provider:   e.Name,
			Capability: CAPABILITY_SPEECH,
		}
	}
	resolved, decorator, err := resolveSpeechOptions(e, o)
	if err != nil {
		return nil, err
	}
	generator, err := e.NewSpeech(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: speech: %w", e.Name, err)
	}
	return WithSpeechDecorator(e.Name, generator, decorator), nil
}

// NewLive builds the named provider's realtime-session capability using the
// same credential resolution and request-time decorator precedence as New.
func NewLive(name string, o Options) (LiveConnector, error) {
	e, err := lookup(name)
	if err != nil {
		return nil, err
	}
	if e.NewLive == nil {
		return nil, &UnsupportedCapabilityError{
			Provider:   e.Name,
			Capability: CAPABILITY_LIVE,
		}
	}
	resolved, decorator, err := resolveLiveOptions(e, o)
	if err != nil {
		return nil, err
	}
	connector, err := e.NewLive(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: live: %w", e.Name, err)
	}
	return WithLiveDecorator(e.Name, connector, decorator), nil
}

// NewTranslate builds the named provider's translation capability using the
// same credential resolution and request-time decorator precedence as New.
func NewTranslate(name string, o Options) (Translator, error) {
	e, err := lookup(name)
	if err != nil {
		return nil, err
	}
	if e.NewTranslate == nil {
		return nil, &UnsupportedCapabilityError{
			Provider:   e.Name,
			Capability: CAPABILITY_TRANSLATE,
		}
	}
	resolved, decorator, err := resolveLiveOptions(e, o)
	if err != nil {
		return nil, err
	}
	translator, err := e.NewTranslate(resolved)
	if err != nil {
		return nil, fmt.Errorf("provider %s: translate: %w", e.Name, err)
	}
	return WithTranslateDecorator(e.Name, translator, decorator), nil
}

func lookup(name string) (Entry, error) {
	e, ok := Lookup(name)
	if !ok {
		return Entry{}, fmt.Errorf("unknown provider %q (registered: %s)",
			name, strings.Join(Names(), ", "))
	}
	return e, nil
}

func resolveOptions(e Entry, o Options) (ResolvedConfig, Decorator, error) {
	return resolveOptionsWithMetadata(e, o, e.Metadata)
}

func resolveImageOptions(e Entry, o Options) (ResolvedConfig, Decorator, error) {
	metadata := e.Metadata
	if metadata.ImageBaseURLEnv != "" {
		metadata.BaseURLEnv = metadata.ImageBaseURLEnv
	}
	return resolveOptionsWithMetadata(e, o, metadata)
}

func resolveVideoOptions(e Entry, o Options) (ResolvedConfig, Decorator, error) {
	metadata := e.Metadata
	if metadata.VideoBaseURLEnv != "" {
		metadata.BaseURLEnv = metadata.VideoBaseURLEnv
	}
	return resolveOptionsWithMetadata(e, o, metadata)
}

func resolveMusicOptions(e Entry, o Options) (ResolvedConfig, Decorator, error) {
	metadata := e.Metadata
	if metadata.MusicBaseURLEnv != "" {
		metadata.BaseURLEnv = metadata.MusicBaseURLEnv
	}
	return resolveOptionsWithMetadata(e, o, metadata)
}

// resolveSpeechOptions applies the speech-specific base URL override.
// Transcription has no counterpart: no registered provider serves it from a
// different host than the one BaseURLEnv already names.
func resolveSpeechOptions(e Entry, o Options) (ResolvedConfig, Decorator, error) {
	metadata := e.Metadata
	if metadata.SpeechBaseURLEnv != "" {
		metadata.BaseURLEnv = metadata.SpeechBaseURLEnv
	}
	return resolveOptionsWithMetadata(e, o, metadata)
}

// resolveLiveOptions applies the realtime-endpoint base URL override. It
// serves both NewLive and NewTranslate: every registered translator rides the
// same realtime socket, so a separate translate override would name the same
// host twice.
func resolveLiveOptions(e Entry, o Options) (ResolvedConfig, Decorator, error) {
	metadata := e.Metadata
	if metadata.LiveBaseURLEnv != "" {
		metadata.BaseURLEnv = metadata.LiveBaseURLEnv
	}
	return resolveOptionsWithMetadata(e, o, metadata)
}

func resolveOptionsWithMetadata(
	e Entry,
	o Options,
	metadata Metadata,
) (ResolvedConfig, Decorator, error) {
	resolved, err := o.Resolve(metadata)
	if err != nil {
		return ResolvedConfig{}, nil, fmt.Errorf("provider %s: %w", e.Name, err)
	}
	if e.Metadata.CredentialRequired && resolved.Auth.Token() == "" && o.Decorator == nil {
		return ResolvedConfig{}, nil, fmt.Errorf("provider %s: credential is required", e.Name)
	}

	// Options.APIKey is the highest-precedence construction credential.
	// Resolve leaves it in Auth; do not then attach an ambient decorator
	// that could replace it. A later per-request Auth still outranks it.
	decorator := o.Decorator
	if !resolved.Auth.IsZero() {
		decorator = nil
	}
	return resolved, decorator, nil
}
