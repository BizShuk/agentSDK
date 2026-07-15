// Package frames extracts still frames from a video file via ffmpeg. It has
// no dependency on internal/preprocess/subtitles or on any HTTP/handler
// code — it is a standalone, independently testable unit: given a video path
// it returns image files on disk, nothing more.
package frames

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/bizshuk/agentsdk/video/ffmpegutil"
)

// Format is the still-image codec ffmpeg writes. JPEG keeps output small;
// PNG is available for lossless needs.
type Format string

const (
	JPEG Format = "jpg"
	PNG  Format = "png"
)

// Options controls how frames are sampled from the source video.
type Options struct {
	// Interval is the fixed spacing between sampled frames. Zero uses
	// DefaultInterval. Mutually exclusive with SceneThreshold in intent
	// (both fire per ffmpeg's own filtergraph if set); callers wanting pure
	// scene-change sampling should set Interval to 0 and SceneThreshold >0.
	Interval time.Duration
	// SceneThreshold, if >0, additionally selects frames where ffmpeg's
	// scene-change score exceeds this value (0..1). Useful for capturing
	// cuts an interval sampler would straddle.
	SceneThreshold float64
	// MaxFrames caps the number of frames ffmpeg is allowed to emit
	// (`-frames:v`), bounding output size for long videos. Zero = no cap.
	MaxFrames int
	Format    Format
}

// DefaultInterval matches the TS proxy's "a few representative frames"
// guidance from Anthropic's own video-via-frames recommendation.
const DefaultInterval = 2 * time.Second

// Frame is one extracted still, in source-video chronological order.
type Frame struct {
	Path      string
	Timestamp time.Duration
}

// Extract samples frames from videoPath into outDir (created if absent) and
// returns them ordered by timestamp. outDir is left populated on success —
// callers own cleanup (os.RemoveAll) once frames are consumed.
func Extract(ctx context.Context, videoPath, outDir string, opts Options) ([]Frame, error) {
	if err := ffmpegutil.CheckAvailable(); err != nil {
		return nil, err
	}
	if opts.Interval == 0 && opts.SceneThreshold == 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Format == "" {
		opts.Format = JPEG
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create out dir: %w", err)
	}

	// Chain `showinfo` after `select` so ffmpeg logs each emitted frame's
	// real presentation timestamp (pts_time) to stderr — the only way to get
	// accurate timing back, since the output image files carry none.
	filter := buildSelectFilter(opts) + ",showinfo"
	pattern := filepath.Join(outDir, "frame-%06d."+string(opts.Format))

	args := []string{"-y", "-i", videoPath, "-vf", filter, "-vsync", "vfr"}
	if opts.MaxFrames > 0 {
		args = append(args, "-frames:v", fmt.Sprintf("%d", opts.MaxFrames))
	}
	args = append(args, pattern)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg extract frames: %w: %s", err, out)
	}

	return frameTimestamps(outDir, opts, out)
}

var ptsTimeRe = regexp.MustCompile(`pts_time:([0-9.]+)`)

// buildSelectFilter renders ffmpeg's `select` expression. Interval sampling
// uses `not(mod(t,N))`-style time gating; scene sampling uses the built-in
// `gt(scene,T)` metric. Both can be OR'd together.
func buildSelectFilter(opts Options) string {
	var exprs []string
	if opts.Interval > 0 {
		secs := opts.Interval.Seconds()
		exprs = append(exprs, fmt.Sprintf("isnan(prev_selected_t)+gte(t-prev_selected_t\\,%.3f)", secs))
	}
	if opts.SceneThreshold > 0 {
		exprs = append(exprs, fmt.Sprintf("gt(scene\\,%.3f)", opts.SceneThreshold))
	}
	expr := exprs[0]
	for _, e := range exprs[1:] {
		expr = expr + "+" + e
	}
	return fmt.Sprintf("select='%s'", expr)
}

// frameTimestamps pairs the files ffmpeg wrote (sorted by their sequence
// number, which matches emission order) with the pts_time values `showinfo`
// logged to stderr, one per emitted frame in the same order.
func frameTimestamps(outDir string, opts Options, ffmpegLog []byte) ([]Frame, error) {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return nil, fmt.Errorf("read out dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == "."+string(opts.Format) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	matches := ptsTimeRe.FindAllSubmatch(ffmpegLog, -1)
	if len(matches) != len(names) {
		return nil, fmt.Errorf(
			"frame/timestamp count mismatch: %d files, %d pts_time entries in ffmpeg log",
			len(names), len(matches),
		)
	}

	frames := make([]Frame, 0, len(names))
	for i, name := range names {
		secs, err := strconv.ParseFloat(string(matches[i][1]), 64)
		if err != nil {
			return nil, fmt.Errorf("parse pts_time %q: %w", matches[i][1], err)
		}
		frames = append(frames, Frame{
			Path:      filepath.Join(outDir, name),
			Timestamp: time.Duration(secs * float64(time.Second)),
		})
	}
	return frames, nil
}
