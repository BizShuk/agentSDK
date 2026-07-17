// Package ffmpegutil holds shared, dependency-free helpers (binary discovery,
// duration probing) used by both utils/video/frames and
// utils/video/subtitles. Neither of those packages depends on the
// other — this package is the only thing they share.
package ffmpegutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// ErrNotInstalled is returned by CheckAvailable when ffmpeg/ffprobe are not
// on PATH. Callers should surface this as a clear setup error rather than a
// generic exec failure.
var ErrNotInstalled = errors.New("ffmpeg/ffprobe not found on PATH")

// CheckAvailable verifies both binaries this package's callers need are
// resolvable, so a missing dependency fails fast with one clear message
// instead of an opaque exec.ErrNotFound deep inside a probe/extract call.
func CheckAvailable() error {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%w: %s", ErrNotInstalled, bin)
		}
	}
	return nil
}

// ffprobeFormat mirrors the subset of `ffprobe -show_format -of json` output
// this package reads.
type ffprobeFormat struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// Probe returns the media duration of the file at path using ffprobe.
func Probe(ctx context.Context, path string) (time.Duration, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", path, exitErr(err))
	}

	var parsed ffprobeFormat
	if err := json.Unmarshal(out, &parsed); err != nil {
		return 0, fmt.Errorf("parse ffprobe output: %w", err)
	}
	secs, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", parsed.Format.Duration, err)
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// exitErr unwraps *exec.ExitError to include stderr, which ffmpeg/ffprobe use
// for the actual diagnostic message.
func exitErr(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, ee.Stderr)
	}
	return err
}
