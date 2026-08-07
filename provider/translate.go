package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// Translator is the optional provider capability for translating text between
// languages. It remains separate from core.Provider because the agent runtime
// does not consume translation requests.
type Translator interface {
	Translate(ctx context.Context, request TranslateRequest) (TranslateResult, error)
}

// TranslateStreamer is the optional streaming variant of Translator, paired
// with it the way core.StreamProvider is paired with core.Provider: an
// adapter implements it only when the vendor serves translation over a
// realtime surface, and callers type-assert to find out. The channel closes
// after the final chunk; a chunk with a non-nil Err terminates the stream.
type TranslateStreamer interface {
	StreamTranslation(ctx context.Context, request TranslateRequest) (<-chan TranslateChunk, error)
}

// TranslateFactory builds a translator from registry-resolved options.
type TranslateFactory func(ResolvedConfig) (Translator, error)

// TranslateRequest is the provider-neutral input to a translation API.
// TargetLanguage is a BCP-47 / ISO-639 code ("es", "zh-TW"); the source
// language is detected by the provider.
type TranslateRequest struct {
	Model          string `json:"model,omitempty"`
	Text           string `json:"text"`
	TargetLanguage string `json:"target_language"`

	// Auth overrides construction-time or decorated credentials for this call.
	Auth core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent translation request invariants.
func (r TranslateRequest) Validate() error {
	if strings.TrimSpace(r.Text) == "" {
		return fmt.Errorf("translate text is required")
	}
	if strings.TrimSpace(r.TargetLanguage) == "" {
		return fmt.Errorf("translate target language is required")
	}
	return nil
}

// TranslateResult is the folded response from one translation request.
type TranslateResult struct {
	Text  string          `json:"text"`
	Usage core.TokenUsage `json:"usage,omitempty"`
	Cost  core.Cost       `json:"cost"`
}

// TranslateChunk is one increment of a streaming translation.
type TranslateChunk struct {
	Text string `json:"text,omitempty"`
	Err  error  `json:"-"`
}

type decoratedTranslator struct {
	translator Translator
	name       string
	decorate   Decorator
}

func (d *decoratedTranslator) Translate(
	ctx context.Context,
	req TranslateRequest,
) (TranslateResult, error) {
	req, err := d.apply(ctx, req)
	if err != nil {
		return TranslateResult{}, err
	}
	return d.translator.Translate(ctx, req)
}

func (d *decoratedTranslator) apply(
	ctx context.Context,
	req TranslateRequest,
) (TranslateRequest, error) {
	auth, err := resolveRequestAuth(ctx, d.name, d.decorate, req.Auth)
	if err != nil {
		return req, err
	}
	req.Auth = auth
	return req, nil
}

// decoratedTranslateStreamer is the wrapper used when the wrapped translator
// also streams. Two types keep the optional capability honest: callers decide
// between blocking and streaming with a type assertion, so a single wrapper
// that always advertised TranslateStreamer would make every decorated
// translator claim a surface half of them do not have.
type decoratedTranslateStreamer struct {
	decoratedTranslator
	streamer TranslateStreamer
}

func (d *decoratedTranslateStreamer) StreamTranslation(
	ctx context.Context,
	req TranslateRequest,
) (<-chan TranslateChunk, error) {
	req, err := d.apply(ctx, req)
	if err != nil {
		return nil, err
	}
	return d.streamer.StreamTranslation(ctx, req)
}

// WithTranslateDecorator returns a translator that resolves credentials
// before every outbound request, preserving the wrapped value's streaming
// capability. A nil decorator returns translator unchanged.
func WithTranslateDecorator(
	name string,
	translator Translator,
	decorator Decorator,
) Translator {
	if decorator == nil || translator == nil {
		return translator
	}
	base := decoratedTranslator{
		translator: translator,
		name:       name,
		decorate:   decorator,
	}
	if streamer, ok := translator.(TranslateStreamer); ok {
		return &decoratedTranslateStreamer{decoratedTranslator: base, streamer: streamer}
	}
	return &base
}
