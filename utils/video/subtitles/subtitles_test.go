package subtitles

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func synthTestClip(t *testing.T, dir string, seconds int) string {
	t.Helper()
	path := filepath.Join(dir, "synth.mp4")
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=duration=%d:size=64x64:rate=10", seconds),
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=440:duration=%d", seconds),
		"-pix_fmt", "yuv420p",
		"-shortest",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg unavailable or failed to synth test clip: %v: %s", err, out)
	}
	return path
}

// stubTranscriber returns a fixed set of segments, letting Generate's
// orchestration (audio extraction, work-dir/cleanup handling) be verified
// without a real ASR backend.
type stubTranscriber struct {
	segments []Segment
	gotPath  string
}

func (s *stubTranscriber) Transcribe(ctx context.Context, wavPath string) ([]Segment, error) {
	s.gotPath = wavPath
	return s.segments, nil
}

func TestGenerate_OrchestratesExtractAndTranscribe(t *testing.T) {
	dir := t.TempDir()
	video := synthTestClip(t, dir, 3)
	workDir := filepath.Join(dir, "work")

	want := []Segment{{Start: 0, End: time.Second, Text: "hello"}}
	stub := &stubTranscriber{segments: want}

	got, err := Generate(context.Background(), video, workDir, stub, false)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(got) != 1 || got[0].Text != "hello" {
		t.Errorf("segments = %+v, want %+v", got, want)
	}
	if stub.gotPath == "" {
		t.Fatal("transcriber never received a wav path")
	}
	if _, err := os.Stat(stub.gotPath); err == nil {
		t.Error("intermediate wav should have been removed (keepAudio=false)")
	}
}

func TestGenerate_KeepAudio(t *testing.T) {
	dir := t.TempDir()
	video := synthTestClip(t, dir, 2)
	workDir := filepath.Join(dir, "work")
	stub := &stubTranscriber{}

	if _, err := Generate(context.Background(), video, workDir, stub, true); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := os.Stat(stub.gotPath); err != nil {
		t.Errorf("expected wav to survive with keepAudio=true: %v", err)
	}
}

func TestNoopTranscriber(t *testing.T) {
	dir := t.TempDir()
	video := synthTestClip(t, dir, 1)
	workDir := filepath.Join(dir, "work")

	got, err := Generate(context.Background(), video, workDir, NoopTranscriber{}, false)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no segments from NoopTranscriber, got %d", len(got))
	}
}

func TestWriteSRT(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.srt")
	segments := []Segment{
		{Start: 0, End: 1500 * time.Millisecond, Text: "Hello world"},
		{Start: 1500 * time.Millisecond, End: 3200 * time.Millisecond, Text: "Second line"},
	}

	if err := WriteSRT(segments, outPath); err != nil {
		t.Fatalf("WriteSRT: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read srt: %v", err)
	}
	want := "1\n00:00:00,000 --> 00:00:01,500\nHello world\n\n" +
		"2\n00:00:01,500 --> 00:00:03,200\nSecond line\n\n"
	if string(data) != want {
		t.Errorf("srt output mismatch:\ngot:  %q\nwant: %q", data, want)
	}
}

func TestGenerate_MissingVideo(t *testing.T) {
	dir := t.TempDir()
	_, err := Generate(context.Background(), filepath.Join(dir, "nope.mp4"), filepath.Join(dir, "work"), NoopTranscriber{}, false)
	if err == nil {
		t.Fatal("expected error for missing input video")
	}
}
