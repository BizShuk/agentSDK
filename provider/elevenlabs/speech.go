package elevenlabs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const defaultSpeechRequestTimeout = 2 * time.Minute

// SpeechProvider implements provider.SpeechGenerator and the optional
// provider.SpeechStreamer against ElevenLabs' text-to-speech endpoints.
type SpeechProvider struct {
	baseURL string
	model   string
	auth    core.Auth
	client  *http.Client
}

// NewSpeech returns an ElevenLabs speech generator from registry-resolved
// config.
func NewSpeech(cfg provider.ResolvedConfig) (*SpeechProvider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultSpeechRequestTimeout
	}
	return &SpeechProvider{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   strings.TrimSpace(cfg.Model),
		auth:    cfg.Auth,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

// GenerateSpeech synthesizes one complete clip and returns its bytes.
func (p *SpeechProvider) GenerateSpeech(
	ctx context.Context,
	request provider.SpeechRequest,
) (provider.SpeechResult, error) {
	call, err := p.prepare(request)
	if err != nil {
		return provider.SpeechResult{}, err
	}
	audio, err := p.postSpeech(ctx, call, speechPath(request.Voice))
	if err != nil {
		return provider.SpeechResult{}, err
	}
	return provider.SpeechResult{
		Audio: provider.SpeechAsset{
			Bytes:  audio,
			Format: call.format,
		},
	}, nil
}

// StreamSpeech implements provider.SpeechStreamer. The returned reader yields
// the same encoding GenerateSpeech would have returned; the caller owns Close.
func (p *SpeechProvider) StreamSpeech(
	ctx context.Context,
	request provider.SpeechRequest,
) (io.ReadCloser, error) {
	call, err := p.prepare(request)
	if err != nil {
		return nil, err
	}
	return p.postSpeechStream(ctx, call, speechStreamPath(request.Voice))
}

// speechCall is one prepared request: everything resolved from defaults and
// validated, so the two transport paths share identical semantics.
type speechCall struct {
	auth        core.Auth
	payload     []byte
	outputQuery string
	format      string
}

func (p *SpeechProvider) prepare(request provider.SpeechRequest) (speechCall, error) {
	if err := request.Validate(); err != nil {
		return speechCall{}, err
	}
	auth := p.auth.Merge(request.Auth)
	if auth.Token() == "" {
		return speechCall{}, fmt.Errorf("elevenlabs speech credential is required")
	}
	payload, err := encodeSpeechRequest(request, p.resolveSpeechModel(request))
	if err != nil {
		return speechCall{}, err
	}

	outputFormat := strings.TrimSpace(request.OutputFormat)
	format := outputFormat
	if format == "" {
		format = DefaultSpeechOutputFormat
	}
	return speechCall{
		auth:        auth,
		payload:     payload,
		outputQuery: outputFormat,
		format:      format,
	}, nil
}

func (p *SpeechProvider) resolveSpeechModel(request provider.SpeechRequest) string {
	if model := strings.TrimSpace(request.Model); model != "" {
		return model
	}
	if p.model != "" {
		return p.model
	}
	return DefaultSpeechModel
}

func resolveVoiceID(voice string) string {
	if trimmed := strings.TrimSpace(voice); trimmed != "" {
		return trimmed
	}
	return DefaultVoiceID
}
