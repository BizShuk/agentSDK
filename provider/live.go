package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// LiveConnector is the optional provider capability for opening a
// bidirectional realtime session — low-latency dialogue where client input
// (text or microphone audio) and model output (text or speech) flow over one
// long-lived connection. It remains separate from core.Provider because the
// agent runtime does not consume realtime sessions.
type LiveConnector interface {
	ConnectLive(ctx context.Context, request LiveRequest) (LiveSession, error)
}

// LiveFactory builds a live connector from registry-resolved options.
type LiveFactory func(ResolvedConfig) (LiveConnector, error)

// Live response modalities. A session produces text or audio, never both:
// the vendors that serve realtime sessions enforce one output modality per
// connection, so the contract states it instead of failing at the socket.
const (
	LIVE_MODALITY_TEXT  = "text"
	LIVE_MODALITY_AUDIO = "audio"
)

// LiveTranslation asks the session to translate incoming content instead of
// conversing. TargetLanguage is a BCP-47 / ISO-639 code ("es", "zh-TW").
// EchoTarget additionally plays back the source-language content alongside
// the translation, for vendors that support it.
type LiveTranslation struct {
	TargetLanguage string `json:"target_language"`
	EchoTarget     bool   `json:"echo_target,omitempty"`
}

// LiveRequest is the provider-neutral configuration for one realtime
// session. Empty optional fields keep the adapter's own defaults.
type LiveRequest struct {
	Model string `json:"model,omitempty"`

	// System is the session-wide system instruction.
	System string `json:"system,omitempty"`

	// ResponseModality selects the output kind — LIVE_MODALITY_TEXT or
	// LIVE_MODALITY_AUDIO. Empty means text.
	ResponseModality string `json:"response_modality,omitempty"`

	// ThinkingLevel tunes reasoning depth on models that expose it
	// ("minimal", "low", "medium", "high"). Empty keeps the model default.
	ThinkingLevel string `json:"thinking_level,omitempty"`

	// Voice is a provider-specific voice id for audio output. Empty selects
	// the adapter's default voice.
	Voice string `json:"voice,omitempty"`

	// TranscribeInput / TranscribeOutput ask the session to emit text
	// transcripts of the audio flowing in each direction.
	TranscribeInput  bool `json:"transcribe_input,omitempty"`
	TranscribeOutput bool `json:"transcribe_output,omitempty"`

	// Translation switches the session into translation mode on models that
	// support it. nil means a plain dialogue session.
	Translation *LiveTranslation `json:"translation,omitempty"`

	// Auth overrides construction-time or decorated credentials for this
	// session.
	Auth core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent live request invariants.
func (r LiveRequest) Validate() error {
	switch r.ResponseModality {
	case "", LIVE_MODALITY_TEXT, LIVE_MODALITY_AUDIO:
	default:
		return fmt.Errorf("live response modality %q is not %q or %q",
			r.ResponseModality, LIVE_MODALITY_TEXT, LIVE_MODALITY_AUDIO)
	}
	if r.Translation != nil && strings.TrimSpace(r.Translation.TargetLanguage) == "" {
		return fmt.Errorf("live translation target language is required")
	}
	return nil
}

// LiveEvent is one server turn fragment. A single event can carry several
// fields at once — an audio chunk plus its transcript, or text plus
// TurnComplete — so callers must consume every populated field, not switch on
// one.
type LiveEvent struct {
	// Text is model output text for text-modality sessions.
	Text string `json:"text,omitempty"`

	// Audio is a raw model output audio chunk; AudioMIME names its encoding
	// (e.g. "audio/pcm;rate=24000").
	Audio     []byte `json:"audio,omitempty"`
	AudioMIME string `json:"audio_mime,omitempty"`

	// InputTranscript / OutputTranscript are incremental transcripts of the
	// audio flowing in each direction, present when the session asked for
	// them.
	InputTranscript  string `json:"input_transcript,omitempty"`
	OutputTranscript string `json:"output_transcript,omitempty"`

	// Interrupted reports that new user input cut the model's turn short.
	Interrupted bool `json:"interrupted,omitempty"`

	// TurnComplete marks the end of one model turn.
	TurnComplete bool `json:"turn_complete,omitempty"`

	// Usage and Cost are populated on the TurnComplete event.
	Usage core.TokenUsage `json:"usage,omitempty"`
	Cost  core.Cost       `json:"cost,omitempty"`
}

// LiveSession is one open realtime connection. Send methods and Receive may
// be used from different goroutines; Receive itself must not be called
// concurrently. Receive returns io.EOF once the server closes the session
// normally. The caller owns Close.
type LiveSession interface {
	// SendText submits one complete user text turn.
	SendText(ctx context.Context, text string) error

	// SendAudio streams one chunk of user audio. The expected encoding is
	// adapter-specific; Gemini Live takes 16 kHz 16-bit mono little-endian
	// PCM.
	SendAudio(ctx context.Context, pcm []byte) error

	// Receive blocks for the next server event.
	Receive(ctx context.Context) (LiveEvent, error)

	Close() error
}

type decoratedLive struct {
	connector LiveConnector
	name      string
	decorate  Decorator
}

func (d *decoratedLive) ConnectLive(
	ctx context.Context,
	req LiveRequest,
) (LiveSession, error) {
	auth, err := resolveRequestAuth(ctx, d.name, d.decorate, req.Auth)
	if err != nil {
		return nil, err
	}
	req.Auth = auth
	return d.connector.ConnectLive(ctx, req)
}

// WithLiveDecorator returns a live connector that resolves credentials before
// every session dial. Resolving at connect time (not per message) is enough:
// the credential authenticates the websocket handshake, and an established
// session stays authenticated for its lifetime. A nil decorator returns
// connector unchanged.
func WithLiveDecorator(
	name string,
	connector LiveConnector,
	decorator Decorator,
) LiveConnector {
	if decorator == nil || connector == nil {
		return connector
	}
	return &decoratedLive{connector: connector, name: name, decorate: decorator}
}
