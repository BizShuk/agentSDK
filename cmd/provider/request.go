// Package provider holds the per-type handlers behind the root binary's
// "provider" subcommand — one file per API type (chat, image, music, speech,
// transcribe) plus the capability matrix and catalog dump. The cobra wiring,
// flags, and dispatch stay in cmd/provider.go; this package owns what each
// type actually does against the SDK's provider layer.
package provider

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/bizshuk/agentsdk/provider"
)

// Request holds the parameter payload required to execute one provider API
// request. Each handler consumes the fields its type understands and ignores
// the rest; Options carries the registry-level construction input.
type Request struct {
	Provider string
	Prompt   string
	JSON     bool
	Options  provider.Options

	// Chat.
	System    string
	MaxTokens int
	Stream    bool

	// Music.
	Lyrics       string
	OutputFormat string
	SampleRate   int
	Bitrate      int
	AudioFormat  string

	// Speech.
	Voice        string
	SpeechFormat string

	// Transcribe (AudioURL doubles as the music cover reference).
	AudioURL  string
	AudioFile string
	Language  string
	Diarize   bool
}

// WriteJSON encodes value as formatted JSON to out.
func WriteJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode JSON output: %w", err)
	}
	return nil
}
