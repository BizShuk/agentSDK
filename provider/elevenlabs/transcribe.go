package elevenlabs

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const defaultTranscribeRequestTimeout = 5 * time.Minute

// TranscribeProvider implements provider.Transcriber against ElevenLabs'
// speech-to-text endpoint.
type TranscribeProvider struct {
	baseURL string
	model   string
	auth    core.Auth
	client  *http.Client
}

// NewTranscriber returns an ElevenLabs transcriber from registry-resolved
// config.
func NewTranscriber(cfg provider.ResolvedConfig) (*TranscribeProvider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTranscribeRequestTimeout
	}
	return &TranscribeProvider{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   strings.TrimSpace(cfg.Model),
		auth:    cfg.Auth,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

// Transcribe converts one recording into text.
func (p *TranscribeProvider) Transcribe(
	ctx context.Context,
	request provider.TranscribeRequest,
) (provider.TranscribeResult, error) {
	if err := request.Validate(); err != nil {
		return provider.TranscribeResult{}, err
	}
	auth := p.auth.Merge(request.Auth)
	if auth.Token() == "" {
		return provider.TranscribeResult{}, fmt.Errorf(
			"elevenlabs transcribe credential is required",
		)
	}
	payload, contentType, err := encodeTranscribeRequest(
		request,
		p.resolveTranscribeModel(request),
	)
	if err != nil {
		return provider.TranscribeResult{}, err
	}
	response, err := p.postTranscribe(ctx, auth, payload, contentType)
	if err != nil {
		return provider.TranscribeResult{}, err
	}
	return foldTranscribeResponse(response), nil
}

func (p *TranscribeProvider) resolveTranscribeModel(
	request provider.TranscribeRequest,
) string {
	if model := strings.TrimSpace(request.Model); model != "" {
		return model
	}
	if p.model != "" {
		return p.model
	}
	return DefaultTranscribeModel
}
