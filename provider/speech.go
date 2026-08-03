package provider

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// SpeechGenerator is the optional provider capability for turning text into
// audio. It remains separate from core.Provider because the agent runtime does
// not consume speech-synthesis requests.
type SpeechGenerator interface {
	GenerateSpeech(ctx context.Context, request SpeechRequest) (SpeechResult, error)
}

// SpeechStreamer is the optional streaming variant of SpeechGenerator, paired
// with it the way core.StreamProvider is paired with core.Provider: an adapter
// implements it only when the vendor exposes a streaming endpoint, and callers
// type-assert to find out.
//
// The reader yields raw audio bytes in the requested output format. The caller
// owns Close.
type SpeechStreamer interface {
	StreamSpeech(ctx context.Context, request SpeechRequest) (io.ReadCloser, error)
}

// SpeechFactory builds a speech generator from registry-resolved options.
type SpeechFactory func(ResolvedConfig) (SpeechGenerator, error)

// VoiceSetting tunes delivery of a synthesized voice. Zero values leave every
// knob at the provider's own default; providers ignore the fields they have no
// equivalent for rather than approximating them.
type VoiceSetting struct {
	Stability  float64 `json:"stability,omitempty"`
	Similarity float64 `json:"similarity,omitempty"`
	Style      float64 `json:"style,omitempty"`
	Speed      float64 `json:"speed,omitempty"`
}

// SpeechRequest is the provider-neutral input to a text-to-speech API. Empty
// optional fields let the adapter apply its own model, voice, and encoding
// defaults.
type SpeechRequest struct {
	Model string `json:"model,omitempty"`
	Text  string `json:"text"`

	// Voice is a provider-specific voice id. Empty selects the adapter's
	// default voice.
	Voice string `json:"voice,omitempty"`

	// OutputFormat names the encoding — "pcm_16000", "mp3_44100_128", "mp3".
	// Empty leaves the provider's default in place.
	OutputFormat string `json:"output_format,omitempty"`

	VoiceSetting VoiceSetting `json:"voice_setting,omitempty"`

	// Auth overrides construction-time or decorated credentials for this call.
	Auth core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent speech request invariants.
func (r SpeechRequest) Validate() error {
	if strings.TrimSpace(r.Text) == "" {
		return fmt.Errorf("speech text is required")
	}
	return nil
}

// SpeechAsset is the synthesized audio payload. Bytes is canonical: a vendor
// that ships hex or base64 on the wire decodes it in its own adapter, so every
// caller sees one representation.
type SpeechAsset struct {
	Bytes  []byte `json:"bytes,omitempty"`
	Format string `json:"format,omitempty"`
}

// SpeechInfo is the common metadata for one synthesized clip. Fields stay zero
// when the provider does not report them.
type SpeechInfo struct {
	DurationMs int64 `json:"duration_ms,omitempty"`
	SampleRate int   `json:"sample_rate,omitempty"`
	Channels   int   `json:"channels,omitempty"`
	Bitrate    int   `json:"bitrate,omitempty"`
	SizeBytes  int64 `json:"size_bytes,omitempty"`
}

// SpeechResult is the folded response from one speech-synthesis request.
type SpeechResult struct {
	Audio SpeechAsset `json:"audio"`
	Info  SpeechInfo  `json:"info,omitempty"`
}

type decoratedSpeech struct {
	generator SpeechGenerator
	name      string
	decorate  Decorator
}

func (d *decoratedSpeech) GenerateSpeech(
	ctx context.Context,
	req SpeechRequest,
) (SpeechResult, error) {
	req, err := d.apply(ctx, req)
	if err != nil {
		return SpeechResult{}, err
	}
	return d.generator.GenerateSpeech(ctx, req)
}

func (d *decoratedSpeech) apply(
	ctx context.Context,
	req SpeechRequest,
) (SpeechRequest, error) {
	auth, err := resolveRequestAuth(ctx, d.name, d.decorate, req.Auth)
	if err != nil {
		return req, err
	}
	req.Auth = auth
	return req, nil
}

// decoratedSpeechStreamer is the wrapper used when the wrapped generator also
// streams. Two types rather than one keep the optional capability honest:
// callers decide between blocking and streaming with a type assertion, so a
// single wrapper that always advertised SpeechStreamer would make every
// decorated generator claim an endpoint half of them do not have.
type decoratedSpeechStreamer struct {
	decoratedSpeech
	streamer SpeechStreamer
}

func (d *decoratedSpeechStreamer) StreamSpeech(
	ctx context.Context,
	req SpeechRequest,
) (io.ReadCloser, error) {
	req, err := d.apply(ctx, req)
	if err != nil {
		return nil, err
	}
	return d.streamer.StreamSpeech(ctx, req)
}

// WithSpeechDecorator returns a speech generator that resolves credentials
// before every outbound request, preserving the wrapped value's streaming
// capability. A nil decorator returns generator unchanged.
func WithSpeechDecorator(
	name string,
	generator SpeechGenerator,
	decorator Decorator,
) SpeechGenerator {
	if decorator == nil || generator == nil {
		return generator
	}
	base := decoratedSpeech{
		generator: generator,
		name:      name,
		decorate:  decorator,
	}
	if streamer, ok := generator.(SpeechStreamer); ok {
		return &decoratedSpeechStreamer{decoratedSpeech: base, streamer: streamer}
	}
	return &base
}
