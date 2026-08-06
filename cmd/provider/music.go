package provider

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

// Music executes a non-streaming music-generation request against the target
// provider.
func Music(ctx context.Context, req Request, out io.Writer) error {
	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	generator, err := provider.NewMusic(req.Provider, req.Options)
	if err != nil {
		return err
	}
	result, err := generator.GenerateMusic(ctx, provider.MusicRequest{
		Prompt:       req.Prompt,
		Lyrics:       req.Lyrics,
		AudioURL:     req.AudioURL,
		OutputFormat: req.OutputFormat,
		AudioSetting: provider.MusicAudioSetting{
			SampleRate: req.SampleRate,
			Bitrate:    req.Bitrate,
			Format:     req.AudioFormat,
		},
	})
	if err != nil {
		return fmt.Errorf("music: %w", err)
	}
	if req.JSON {
		return WriteJSON(out, result)
	}
	if result.Audio.URL != "" {
		if _, err := fmt.Fprintf(out, "music.url=%s\n", result.Audio.URL); err != nil {
			return fmt.Errorf("write music URL: %w", err)
		}
	}
	if result.Audio.Hex != "" {
		if _, err := fmt.Fprintf(
			out,
			"music.hex_chars=%d format=%s\n",
			len(result.Audio.Hex),
			result.Audio.Format,
		); err != nil {
			return fmt.Errorf("write music metadata: %w", err)
		}
	}
	if _, err := fmt.Fprintf(
		out,
		"[status=%d trace_id=%s duration_ms=%d sample_rate=%d bitrate=%d size_bytes=%d]\n",
		result.Status,
		result.TraceID,
		result.Info.DurationMilliseconds,
		result.Info.SampleRate,
		result.Info.Bitrate,
		result.Info.SizeBytes,
	); err != nil {
		return fmt.Errorf("write music result: %w", err)
	}
	return nil
}
