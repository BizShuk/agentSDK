package google

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

// TranslateProvider serves text translation over the Gemini Live API socket.
// The gateway has no request/response translation endpoint — the translate
// model is a realtime session with a translationConfig — so both the blocking
// and streaming surfaces fold the same live session.
type TranslateProvider struct {
	live  *LiveProvider
	model string
}

// NewTranslate builds the translation capability from registry-resolved
// options, sharing the live websocket transport.
func NewTranslate(cfg provider.ResolvedConfig) (*TranslateProvider, error) {
	live, err := NewLive(cfg)
	if err != nil {
		return nil, err
	}
	return &TranslateProvider{live: live, model: cfg.Model}, nil
}

// connect opens one translation-mode live session for the request.
func (t *TranslateProvider) connect(
	ctx context.Context,
	req provider.TranslateRequest,
) (provider.LiveSession, error) {
	model := req.Model
	if model == "" {
		model = t.model
	}
	if model == "" {
		model = DefaultTranslateModel
	}
	return t.live.ConnectLive(ctx, provider.LiveRequest{
		Model:            model,
		ResponseModality: provider.LIVE_MODALITY_TEXT,
		Translation: &provider.LiveTranslation{
			TargetLanguage: req.TargetLanguage,
		},
		Auth: req.Auth,
	})
}

// Translate implements provider.Translator by folding one streamed turn.
func (t *TranslateProvider) Translate(
	ctx context.Context,
	req provider.TranslateRequest,
) (provider.TranslateResult, error) {
	if err := req.Validate(); err != nil {
		return provider.TranslateResult{}, err
	}
	session, err := t.connect(ctx, req)
	if err != nil {
		return provider.TranslateResult{}, err
	}
	defer session.Close()

	if err := session.SendText(ctx, req.Text); err != nil {
		return provider.TranslateResult{}, fmt.Errorf("google translate: send: %w", err)
	}
	var text strings.Builder
	for {
		event, err := session.Receive(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return provider.TranslateResult{}, fmt.Errorf("google translate: receive: %w", err)
		}
		text.WriteString(event.Text)
		if event.TurnComplete {
			break
		}
	}
	return provider.TranslateResult{Text: text.String()}, nil
}

// StreamTranslation implements provider.TranslateStreamer: the channel
// carries text increments as the model produces them and closes after the
// turn completes.
func (t *TranslateProvider) StreamTranslation(
	ctx context.Context,
	req provider.TranslateRequest,
) (<-chan provider.TranslateChunk, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	session, err := t.connect(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := session.SendText(ctx, req.Text); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("google translate: send: %w", err)
	}

	out := make(chan provider.TranslateChunk)
	go func() {
		defer close(out)
		defer session.Close()
		for {
			event, err := session.Receive(ctx)
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				out <- provider.TranslateChunk{Err: err}
				return
			}
			if event.Text != "" {
				out <- provider.TranslateChunk{Text: event.Text}
			}
			if event.TurnComplete {
				return
			}
		}
	}()
	return out, nil
}
