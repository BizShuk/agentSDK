package main

import (
	"context"
	"fmt"
	"io"

	"github.com/bizshuk/agentsdk/provider/sample/config"
	"github.com/bizshuk/agentsdk/provider/sample/svc"
)

func run(
	ctx context.Context,
	cfg config.Config,
	out io.Writer,
	lookupEnv func(string) string,
) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}
	if out == nil {
		return fmt.Errorf("output writer is required")
	}
	if _, err := cfg.Validate(); err != nil {
		return err
	}

	req := svc.Request{
		Provider:     cfg.Provider,
		Prompt:       cfg.Prompt,
		JSON:         cfg.JSON,
		Lyrics:       cfg.Lyrics,
		AudioURL:     cfg.AudioURL,
		OutputFormat: cfg.OutputFormat,
		SampleRate:   cfg.SampleRate,
		Bitrate:      cfg.Bitrate,
		AudioFormat:  cfg.AudioFormat,
		Voice:        cfg.Voice,
		SpeechFormat: cfg.SpeechFormat,
		AudioFile:    cfg.AudioFile,
		Language:     cfg.Language,
		Diarize:      cfg.Diarize,
		Options:      cfg.ProviderOptions(lookupEnv),
	}

	switch cfg.Type {
	case config.API_TYPE_CHAT:
		return svc.Chat(ctx, req, out)
	case config.API_TYPE_IMAGE:
		return svc.Image(ctx, req, out)
	case config.API_TYPE_MUSIC:
		return svc.Music(ctx, req, out)
	case config.API_TYPE_SPEECH:
		return svc.Speech(ctx, req, out)
	case config.API_TYPE_TRANSCRIBE:
		return svc.Transcribe(ctx, req, out)
	default:
		return fmt.Errorf("unreachable API type %q", cfg.Type)
	}
}
