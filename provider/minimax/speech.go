package minimax

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const (
	defaultSpeechModel          = "speech-02-hd"
	defaultSpeechVoiceID        = "male-qn-qingse"
	defaultSpeechFormat         = "mp3"
	defaultSpeechRequestTimeout = 2 * time.Minute
)

// SpeechProvider implements provider.SpeechGenerator against MiniMax's
// non-streaming t2a_v2 endpoint. The streaming variant is a separate wire
// contract and is deliberately not implemented here, so callers that type
// assert for provider.SpeechStreamer correctly find nothing.
type SpeechProvider struct {
	baseURL string
	model   string
	auth    core.Auth
	client  *http.Client
}

// NewSpeech returns a MiniMax speech generator from registry-resolved config.
func NewSpeech(cfg provider.ResolvedConfig) (*SpeechProvider, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultSpeechRequestTimeout
	}
	return &SpeechProvider{
		baseURL: speechBaseURL(cfg.BaseURL),
		model:   strings.TrimSpace(cfg.Model),
		auth:    cfg.Auth,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

// speechBaseURL resolves the t2a_v2 root, in precedence order:
//
//  1. Whatever the registry resolved — MINIMAX_SPEECH_BASE_URL (the
//     capability-specific override, which replaces the base wholesale) or an
//     explicit Options.BaseURL.
//  2. DefaultSpeechBaseURL when nothing resolved.
//
// The trailing "/anthropic" trim is the fallback for step 1 handing over a
// base that names the Anthropic-compat chat surface, which sits one segment
// below the account root t2a_v2 is served from. That happens when a caller
// passes the chat base URL explicitly rather than setting the speech env, and
// concatenating it would produce a 404 instead of audio. The trim applies to
// any resolved base carrying the suffix — ResolvedConfig does not record
// which source supplied it.
func speechBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return DefaultSpeechBaseURL
	}
	return strings.TrimSuffix(trimmed, anthropicCompatSuffix)
}

// GenerateSpeech synthesizes one complete clip.
func (p *SpeechProvider) GenerateSpeech(
	ctx context.Context,
	request provider.SpeechRequest,
) (provider.SpeechResult, error) {
	if err := request.Validate(); err != nil {
		return provider.SpeechResult{}, err
	}
	auth := p.auth.Merge(request.Auth)
	if auth.Token() == "" {
		return provider.SpeechResult{}, fmt.Errorf("minimax speech credential is required")
	}

	format, sampleRate := parseSpeechOutputFormat(request.OutputFormat)
	payload, err := encodeSpeechRequest(
		request,
		p.resolveSpeechModel(request),
		format,
		sampleRate,
	)
	if err != nil {
		return provider.SpeechResult{}, err
	}
	response, id, err := p.createSpeech(ctx, auth, payload)
	if err != nil {
		return provider.SpeechResult{}, err
	}
	if response.BaseResp.StatusCode != 0 {
		return provider.SpeechResult{}, newSpeechAPIError(
			http.StatusOK,
			id,
			response.BaseResp,
		)
	}
	return foldSpeechResponse(response, format)
}

func (p *SpeechProvider) resolveSpeechModel(request provider.SpeechRequest) string {
	if model := strings.TrimSpace(request.Model); model != "" {
		return model
	}
	if p.model != "" {
		return p.model
	}
	return defaultSpeechModel
}

// parseSpeechOutputFormat splits a provider-neutral output-format label into
// the container MiniMax's audio_setting.format expects plus any sample rate
// encoded in the suffix: "pcm_16000" becomes ("pcm", 16000) and "mp3" becomes
// ("mp3", 0).
//
// A label whose suffix is not a plain sample rate passes through verbatim as
// the format. Guessing would be worse than forwarding: an unrecognized label
// the vendor does accept still works, and one it rejects produces the vendor's
// own error instead of a silently substituted encoding.
func parseSpeechOutputFormat(value string) (string, int) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "", 0
	}
	name, suffix, found := strings.Cut(trimmed, "_")
	if !found {
		return trimmed, 0
	}
	rate, err := strconv.Atoi(suffix)
	if err != nil || rate <= 0 {
		return trimmed, 0
	}
	return name, rate
}
