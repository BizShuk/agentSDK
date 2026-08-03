package svc

import (
	"context"
	"fmt"
	"io"

	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/all"
)

// Speech executes a text-to-speech request against the target provider.
func Speech(ctx context.Context, req Request, out io.Writer) error {
	generator, err := provider.NewSpeech(req.Provider, req.Options)
	if err != nil {
		return err
	}
	result, err := generator.GenerateSpeech(ctx, provider.SpeechRequest{
		Text:         req.Prompt,
		Voice:        req.Voice,
		OutputFormat: req.SpeechFormat,
	})
	if err != nil {
		return fmt.Errorf("speech: %w", err)
	}
	if req.JSON {
		return WriteJSON(out, result)
	}
	// Non-JSON mode reports size and metadata only: raw audio written to a
	// terminal is noise at best.
	if _, err := fmt.Fprintf(
		out,
		"speech.bytes=%d format=%s\n",
		len(result.Audio.Bytes),
		result.Audio.Format,
	); err != nil {
		return fmt.Errorf("write speech metadata: %w", err)
	}
	if _, err := fmt.Fprintf(
		out,
		"[duration_ms=%d sample_rate=%d channels=%d bitrate=%d size_bytes=%d]\n",
		result.Info.DurationMs,
		result.Info.SampleRate,
		result.Info.Channels,
		result.Info.Bitrate,
		result.Info.SizeBytes,
	); err != nil {
		return fmt.Errorf("write speech result: %w", err)
	}
	return nil
}
