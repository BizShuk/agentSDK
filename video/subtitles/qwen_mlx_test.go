package subtitles

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestParseQwenJSON verifies the adapter's JSON parsing against a canned
// fixture matching mlx_audio.stt.generate.save_as_json's plain-segments
// shape, independent of Python/mlx-audio/model availability — this is the
// part of QwenMLXTranscriber that can be exercised in isolation everywhere.
func TestParseQwenJSON(t *testing.T) {
	raw := []byte(`{
		"text": "hello world 你好世界",
		"segments": [
			{"text": "hello world", "start": 0.0, "end": 1.8, "duration": 1.8},
			{"text": "你好世界", "start": 1.8, "end": 3.2, "duration": 1.4}
		]
	}`)

	got, err := parseQwenJSON(raw)
	if err != nil {
		t.Fatalf("parseQwenJSON: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(got))
	}
	if got[0].Text != "hello world" || got[0].Start != 0 || got[0].End != 1800*time.Millisecond {
		t.Errorf("segment 0 = %+v", got[0])
	}
	if got[1].Text != "你好世界" || got[1].Start != 1800*time.Millisecond || got[1].End != 3200*time.Millisecond {
		t.Errorf("segment 1 = %+v", got[1])
	}
}

func TestParseQwenJSON_NoSegments(t *testing.T) {
	got, err := parseQwenJSON([]byte(`{"text": "no timed segments", "segments": []}`))
	if err != nil {
		t.Fatalf("parseQwenJSON: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 segments, got %d", len(got))
	}
}

func TestParseQwenJSON_Malformed(t *testing.T) {
	if _, err := parseQwenJSON([]byte(`not json`)); err == nil {
		t.Fatal("expected error for malformed json")
	}
}

func TestQwenMLXTranscriber_RequiresScriptPath(t *testing.T) {
	_, err := QwenMLXTranscriber{}.Transcribe(context.Background(), "audio.wav")
	if err == nil {
		t.Fatal("expected error when ScriptPath is unset")
	}
}

// TestQwenMLXTranscriber_EndToEnd actually invokes qwen_transcribe.py against
// mlx-audio, which downloads a ~700MB model to ~/.cache/huggingface/ on
// first run. Skipped unless explicitly opted in, both to avoid an
// unattended multi-hundred-MB network fetch and because it requires Apple
// Silicon + `pip install mlx-audio`.
func TestQwenMLXTranscriber_EndToEnd(t *testing.T) {
	if os.Getenv("PROXY_QWEN_ASR_E2E") != "1" {
		t.Skip("set PROXY_QWEN_ASR_E2E=1 to run (downloads an mlx-audio model on first use)")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	dir := t.TempDir()
	video := synthTestClip(t, dir, 3)
	scriptPath, err := filepath.Abs("pyasr/qwen_transcribe.py")
	if err != nil {
		t.Fatal(err)
	}

	transcriber := QwenMLXTranscriber{ScriptPath: scriptPath, ChunkDuration: 5 * time.Second}
	segments, err := Generate(context.Background(), video, filepath.Join(dir, "work"), transcriber, false)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	t.Logf("got %d segment(s)", len(segments))
}
