package subtitles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// WhisperCPPTranscriber shells out to a whisper.cpp CLI binary (the
// `whisper-cli` / legacy `main` executable, not the Python project) and
// parses its `-oj` JSON output. Kept separate from Generate/Transcriber so
// this backend is opt-in: callers without whisper.cpp installed can supply
// any other Transcriber, or NoopTranscriber for wiring tests.
type WhisperCPPTranscriber struct {
	// BinPath is the whisper-cli/main executable. Required.
	BinPath string
	// ModelPath is a ggml model file (e.g. ggml-base.en.bin). Required.
	ModelPath string
	// Language is a whisper language code, or "auto" to detect. Empty
	// defaults to "auto".
	Language string
}

// whisperJSON mirrors the subset of whisper.cpp's `-oj` output this
// transcriber reads. Field names match whisper.cpp's json_output.cpp.
type whisperJSON struct {
	Transcription []struct {
		Offsets struct {
			From int64 `json:"from"` // milliseconds
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

func (w WhisperCPPTranscriber) Transcribe(ctx context.Context, wavPath string) ([]Segment, error) {
	if w.BinPath == "" || w.ModelPath == "" {
		return nil, fmt.Errorf("whisper.cpp transcriber: BinPath and ModelPath are required")
	}
	lang := w.Language
	if lang == "" {
		lang = "auto"
	}

	// whisper.cpp writes `<wavPath minus ext>.json` next to the input when
	// given `-of <prefix>`; use the wav's own path (sans extension) as the
	// prefix so output lands beside it, then clean it up after reading.
	outPrefix := strings.TrimSuffix(wavPath, ".wav")
	jsonPath := outPrefix + ".json"
	defer os.Remove(jsonPath)

	args := []string{
		"-m", w.ModelPath,
		"-f", wavPath,
		"-l", lang,
		"-oj",
		"-of", outPrefix,
		"-nt", // no timestamps in stdout text — we read them from the JSON
	}
	cmd := exec.CommandContext(ctx, w.BinPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("whisper.cpp: %w: %s", err, out)
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read whisper.cpp json output: %w", err)
	}
	var parsed whisperJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse whisper.cpp json output: %w", err)
	}

	segments := make([]Segment, 0, len(parsed.Transcription))
	for _, t := range parsed.Transcription {
		segments = append(segments, Segment{
			Start: time.Duration(t.Offsets.From) * time.Millisecond,
			End:   time.Duration(t.Offsets.To) * time.Millisecond,
			Text:  strings.TrimSpace(t.Text),
		})
	}
	return segments, nil
}

// NoopTranscriber returns no segments for any input. It exists so callers —
// and this package's own tests — can exercise Generate's orchestration
// (audio extraction, work-dir handling, cleanup) without a real ASR backend
// installed.
type NoopTranscriber struct{}

func (NoopTranscriber) Transcribe(ctx context.Context, wavPath string) ([]Segment, error) {
	return nil, nil
}
