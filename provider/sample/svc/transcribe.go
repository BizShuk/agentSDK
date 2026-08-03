package svc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/all"
)

// Transcribe executes a speech-to-text request against the target provider.
// A local --audio-file is uploaded as bytes; --audio-url is handed to the
// provider to fetch.
func Transcribe(ctx context.Context, req Request, out io.Writer) error {
	audio, err := readAudioSource(req)
	if err != nil {
		return err
	}
	transcriber, err := provider.NewTranscriber(req.Provider, req.Options)
	if err != nil {
		return err
	}
	result, err := transcriber.Transcribe(ctx, provider.TranscribeRequest{
		Audio:    audio,
		Language: req.Language,
		Diarize:  req.Diarize,
	})
	if err != nil {
		return fmt.Errorf("transcribe: %w", err)
	}
	if req.JSON {
		return WriteJSON(out, result)
	}
	if _, err := fmt.Fprintf(out, "%s\n", result.Text); err != nil {
		return fmt.Errorf("write transcript: %w", err)
	}
	if _, err := fmt.Fprintf(
		out,
		"[language=%s words=%d]\n",
		result.Language,
		len(result.Words),
	); err != nil {
		return fmt.Errorf("write transcript metadata: %w", err)
	}
	return nil
}

func readAudioSource(req Request) (provider.AudioSource, error) {
	path := strings.TrimSpace(req.AudioFile)
	if path == "" {
		return provider.AudioSource{URL: req.AudioURL, Format: req.AudioFormat}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return provider.AudioSource{}, fmt.Errorf("read audio file: %w", err)
	}
	format := req.AudioFormat
	if extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."); extension != "" {
		format = extension
	}
	return provider.AudioSource{Bytes: raw, Format: format}, nil
}
