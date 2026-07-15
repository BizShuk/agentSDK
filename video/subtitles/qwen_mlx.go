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

// QwenMLXTranscriber shells out to pyasr/qwen_transcribe.py, a thin wrapper
// around mlx_audio.stt (Apple MLX). It has no dependency on
// WhisperCPPTranscriber — both merely implement Transcriber, so either can
// be swapped in to Generate() without touching subtitles.go.
//
// Apple Silicon only (MLX has no other backend). Requires `pip install
// mlx-audio` in whatever Python environment PythonBin resolves to; the model
// (~700MB for the 0.6B variant) downloads to ~/.cache/huggingface/ on first
// use.
type QwenMLXTranscriber struct {
	// PythonBin is the Python interpreter to run the wrapper script with.
	// Defaults to "python3".
	PythonBin string
	// ScriptPath is the path to qwen_transcribe.py. Required.
	ScriptPath string
	// Model is an mlx-community model id. Defaults to the wrapper script's
	// own default (mlx-community/Qwen3-ASR-0.6B-8bit).
	Model string
	// Language is a language code (e.g. "en", "zh"). Empty means auto-detect
	// — the reason this backend exists: unlike whisper.cpp's CLI, the
	// underlying generate() call accepts language=None directly, which is
	// what mixed EN/ZH content needs (see voiceon's asr.py).
	Language string
	// ChunkDuration bounds how long each timed segment can span. Shorter
	// values give tighter subtitle cues at the cost of more model calls.
	// Zero uses the wrapper script's default (10s).
	ChunkDuration time.Duration
}

// qwenJSON mirrors mlx_audio.stt.generate.save_as_json's plain-segments
// shape (the "segments" branch — Qwen3-ASR has no per-word "sentences").
type qwenJSON struct {
	Segments []struct {
		Text  string  `json:"text"`
		Start float64 `json:"start"` // seconds
		End   float64 `json:"end"`
	} `json:"segments"`
}

func (q QwenMLXTranscriber) Transcribe(ctx context.Context, wavPath string) ([]Segment, error) {
	if q.ScriptPath == "" {
		return nil, fmt.Errorf("qwen mlx transcriber: ScriptPath is required")
	}
	pythonBin := q.PythonBin
	if pythonBin == "" {
		pythonBin = "python3"
	}

	outPrefix := strings.TrimSuffix(wavPath, ".wav")
	jsonPath := outPrefix + ".json"
	defer os.Remove(jsonPath)

	args := []string{q.ScriptPath, "--audio", wavPath, "--output-path", outPrefix}
	if q.Model != "" {
		args = append(args, "--model", q.Model)
	}
	if q.Language != "" {
		args = append(args, "--language", q.Language)
	}
	if q.ChunkDuration > 0 {
		args = append(args, "--chunk-duration", fmt.Sprintf("%.3f", q.ChunkDuration.Seconds()))
	}

	cmd := exec.CommandContext(ctx, pythonBin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("qwen3-asr (mlx): %w: %s", err, out)
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read qwen3-asr json output: %w", err)
	}
	return parseQwenJSON(raw)
}

func parseQwenJSON(raw []byte) ([]Segment, error) {
	var parsed qwenJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse qwen3-asr json output: %w", err)
	}

	segments := make([]Segment, 0, len(parsed.Segments))
	for _, s := range parsed.Segments {
		segments = append(segments, Segment{
			Start: time.Duration(s.Start * float64(time.Second)),
			End:   time.Duration(s.End * float64(time.Second)),
			Text:  strings.TrimSpace(s.Text),
		})
	}
	return segments, nil
}
