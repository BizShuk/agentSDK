// Package subtitles turns a video's audio track into timestamped text
// segments ("字幕"). It depends only on utils/video/audio for the
// extraction step — transcription itself is behind the Transcriber
// interface, so this package has no hard dependency on any specific speech
// backend and can be exercised standalone with a stub Transcriber.
package subtitles

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bizshuk/agentsdk/utils/video/audio"
)

// Segment is one timed line of transcript text.
type Segment struct {
	Start time.Duration
	End   time.Duration
	Text  string
}

// Transcriber turns a WAV file into timed segments. Implementations shell
// out to whatever ASR backend is configured (whisper.cpp, an API, etc.);
// this package never talks to a backend directly.
type Transcriber interface {
	Transcribe(ctx context.Context, wavPath string) ([]Segment, error)
}

// Generate extracts videoPath's audio track and runs it through t, returning
// timed segments. The intermediate WAV is written under workDir (created if
// absent) and removed before returning unless keepAudio is true.
func Generate(ctx context.Context, videoPath, workDir string, t Transcriber, keepAudio bool) ([]Segment, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	wavPath := workDir + "/audio.wav"

	if _, err := audio.Extract(ctx, videoPath, wavPath, audio.Options{}); err != nil {
		return nil, fmt.Errorf("extract audio: %w", err)
	}
	if !keepAudio {
		defer os.Remove(wavPath)
	}

	segments, err := t.Transcribe(ctx, wavPath)
	if err != nil {
		return nil, fmt.Errorf("transcribe: %w", err)
	}
	return segments, nil
}

// WriteSRT renders segments as a standard SubRip (.srt) file at outPath.
func WriteSRT(segments []Segment, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create srt file: %w", err)
	}
	defer f.Close()

	for i, seg := range segments {
		if _, err := fmt.Fprintf(f, "%d\n%s --> %s\n%s\n\n",
			i+1, srtTimestamp(seg.Start), srtTimestamp(seg.End), seg.Text,
		); err != nil {
			return fmt.Errorf("write segment %d: %w", i+1, err)
		}
	}
	return nil
}

// srtTimestamp formats a duration as SRT's HH:MM:SS,mmm.
func srtTimestamp(d time.Duration) string {
	ms := d.Milliseconds()
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1_000
	ms %= 1_000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}
