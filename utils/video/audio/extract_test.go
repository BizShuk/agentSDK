package audio

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bizshuk/agentsdk/utils/video/ffmpegutil"
)

// synthTestClip renders a short sine-wave audio clip via ffmpeg's lavfi
// sine source, muxed with a blank video track so Extract exercises the same
// "-vn demux audio out of a video container" path production callers use.
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

func TestExtract_DefaultsToMono16kHzWav(t *testing.T) {
	dir := t.TempDir()
	video := synthTestClip(t, dir, 3)
	outPath := filepath.Join(dir, "audio.wav")

	got, err := Extract(context.Background(), video, outPath, Options{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != outPath {
		t.Errorf("expected returned path %q, got %q", outPath, got)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("output wav missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output wav is empty")
	}

	sampleRate, channels := readWavFormat(t, outPath)
	if sampleRate != DefaultSampleRateHz {
		t.Errorf("sample rate = %d, want %d", sampleRate, DefaultSampleRateHz)
	}
	if channels != DefaultChannels {
		t.Errorf("channels = %d, want %d", channels, DefaultChannels)
	}
}

func TestExtract_CustomSampleRateAndChannels(t *testing.T) {
	dir := t.TempDir()
	video := synthTestClip(t, dir, 2)
	outPath := filepath.Join(dir, "audio.wav")

	_, err := Extract(context.Background(), video, outPath, Options{
		SampleRateHz: 8000,
		Channels:     2,
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	sampleRate, channels := readWavFormat(t, outPath)
	if sampleRate != 8000 {
		t.Errorf("sample rate = %d, want 8000", sampleRate)
	}
	if channels != 2 {
		t.Errorf("channels = %d, want 2", channels)
	}
}

func TestExtract_MissingSource(t *testing.T) {
	dir := t.TempDir()
	_, err := Extract(context.Background(), filepath.Join(dir, "nope.mp4"), filepath.Join(dir, "out.wav"), Options{})
	if err == nil {
		t.Fatal("expected error for missing input file")
	}
}

func TestCheckAvailable(t *testing.T) {
	if err := ffmpegutil.CheckAvailable(); err != nil {
		t.Skipf("ffmpeg not installed: %v", err)
	}
}

// readWavFormat parses the minimal fields this test needs directly out of
// the canonical WAV "fmt " chunk header, avoiding a third-party dependency
// for a single assertion.
func readWavFormat(t *testing.T, path string) (sampleRate, channels int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read wav: %v", err)
	}
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("not a canonical WAV file (len=%d)", len(data))
	}
	channels = int(binary.LittleEndian.Uint16(data[22:24]))
	sampleRate = int(binary.LittleEndian.Uint32(data[24:28]))
	return sampleRate, channels
}
