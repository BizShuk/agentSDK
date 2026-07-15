package frames

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// synthTestVideo renders a short, deterministic clip via ffmpeg's lavfi
// testsrc — no fixture file needed, keeping this test hermetic and fast.
func synthTestVideo(t *testing.T, dir string, seconds int) string {
	t.Helper()
	path := filepath.Join(dir, "synth.mp4")
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=duration=%d:size=64x64:rate=10", seconds),
		"-pix_fmt", "yuv420p",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg unavailable or failed to synth test video: %v: %s", err, out)
	}
	return path
}

func TestExtract_IntervalSampling(t *testing.T) {
	dir := t.TempDir()
	video := synthTestVideo(t, dir, 6)
	outDir := filepath.Join(dir, "frames")

	got, err := Extract(context.Background(), video, outDir, Options{
		Interval: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one frame, got none")
	}

	for i, f := range got {
		if _, err := os.Stat(f.Path); err != nil {
			t.Errorf("frame %d file missing: %v", i, err)
		}
		if i > 0 && f.Timestamp <= got[i-1].Timestamp {
			t.Errorf("frame %d timestamp %v not increasing after %v", i, f.Timestamp, got[i-1].Timestamp)
		}
	}
}

func TestExtract_MaxFramesCap(t *testing.T) {
	dir := t.TempDir()
	video := synthTestVideo(t, dir, 10)
	outDir := filepath.Join(dir, "frames")

	got, err := Extract(context.Background(), video, outDir, Options{
		Interval:  1 * time.Second,
		MaxFrames: 3,
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(got) > 3 {
		t.Errorf("expected at most 3 frames, got %d", len(got))
	}
}

func TestExtract_MissingVideo(t *testing.T) {
	dir := t.TempDir()
	_, err := Extract(context.Background(), filepath.Join(dir, "nope.mp4"), filepath.Join(dir, "out"), Options{})
	if err == nil {
		t.Fatal("expected error for missing input video")
	}
}
