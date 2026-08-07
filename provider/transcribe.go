package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// Transcriber is the optional provider capability for turning recorded audio
// into text. It remains separate from core.Provider because the agent runtime
// does not consume transcription requests.
type Transcriber interface {
	Transcribe(ctx context.Context, request TranscribeRequest) (TranscribeResult, error)
}

// TranscriberFactory builds a transcriber from registry-resolved options.
type TranscriberFactory func(ResolvedConfig) (Transcriber, error)

// AudioSource is one audio input. Bytes and URL are mutually exclusive and
// exactly one is required: an adapter handed both cannot tell which the
// caller meant, and the two reach the vendor over different wire fields.
type AudioSource struct {
	Bytes []byte `json:"bytes,omitempty"`
	URL   string `json:"url,omitempty"`

	// Format labels the encoding — "mp3", "wav", "pcm_16000". Empty leaves
	// detection to the provider.
	Format string `json:"format,omitempty"`

	// DurationMilliseconds lets callers report the source duration when the
	// provider response does not repeat it.
	DurationMilliseconds int64 `json:"duration_milliseconds,omitempty"`
}

// TranscribeRequest is the provider-neutral input to a speech-to-text API.
// Empty optional fields let the adapter apply its own model and detection
// defaults.
type TranscribeRequest struct {
	Model string      `json:"model,omitempty"`
	Audio AudioSource `json:"audio"`

	// Language is an optional ISO-639 hint. Empty asks the provider to
	// detect the language itself.
	Language string `json:"language,omitempty"`

	// Diarize asks the provider to attribute words to distinct speakers.
	Diarize bool `json:"diarize,omitempty"`

	// Auth overrides construction-time or decorated credentials for this call.
	Auth core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent transcription request invariants.
func (r TranscribeRequest) Validate() error {
	hasBytes := len(r.Audio.Bytes) > 0
	hasURL := strings.TrimSpace(r.Audio.URL) != ""
	switch {
	case !hasBytes && !hasURL:
		return fmt.Errorf("transcribe audio bytes or URL is required")
	case hasBytes && hasURL:
		return fmt.Errorf("transcribe audio bytes and URL are mutually exclusive")
	}
	if r.Audio.DurationMilliseconds < 0 {
		return fmt.Errorf("transcribe audio duration must not be negative")
	}
	return nil
}

// TranscribedWord is one timed token of a transcript. Providers that do not
// report timings or speakers leave the corresponding fields zero.
type TranscribedWord struct {
	Text    string `json:"text"`
	StartMs int64  `json:"start_ms,omitempty"`
	EndMs   int64  `json:"end_ms,omitempty"`
	Speaker string `json:"speaker,omitempty"`
}

// TranscribeUsage is the billable work completed by one transcription.
type TranscribeUsage struct {
	AudioDurationMilliseconds int64 `json:"audio_duration_milliseconds,omitempty"`
}

// TranscribeResult is the folded response from one transcription request.
// Words is optional: Text is the complete transcript either way.
type TranscribeResult struct {
	Text     string            `json:"text"`
	Language string            `json:"language,omitempty"`
	Words    []TranscribedWord `json:"words,omitempty"`
	Usage    TranscribeUsage   `json:"usage,omitempty"`
	Cost     core.Cost         `json:"cost"`
}

type decoratedTranscriber struct {
	transcriber Transcriber
	name        string
	decorate    Decorator
}

func (d *decoratedTranscriber) Transcribe(
	ctx context.Context,
	req TranscribeRequest,
) (TranscribeResult, error) {
	auth, err := resolveRequestAuth(ctx, d.name, d.decorate, req.Auth)
	if err != nil {
		return TranscribeResult{}, err
	}
	req.Auth = auth
	return d.transcriber.Transcribe(ctx, req)
}

// WithTranscriberDecorator returns a transcriber that resolves credentials
// before every outbound request. A nil decorator returns transcriber unchanged.
func WithTranscriberDecorator(
	name string,
	transcriber Transcriber,
	decorator Decorator,
) Transcriber {
	if decorator == nil || transcriber == nil {
		return transcriber
	}
	return &decoratedTranscriber{
		transcriber: transcriber,
		name:        name,
		decorate:    decorator,
	}
}
