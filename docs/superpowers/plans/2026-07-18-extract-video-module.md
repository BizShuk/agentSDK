# Extract Video Utilities Into An Independent Module Implementation Plan

> For agentic workers: required sub-skill: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

`Goal:` Move video preprocessing and its Cobra command into the independently versionable module `github.com/bizshuk/agentsdk/utils/video`, while keeping `auth-cli video` behavior stable.

`Architecture:` `utils/video` owns the standalone `audio`, `frames`, `subtitles`, and `ffmpegutil` packages plus a `cmd` composition package. The root `agentsdk` module depends on `utils/video/cmd` only to register `videocmd.NewCommand()` in its existing CLI; the child module never imports `agentsdk`.

`Tech Stack:` Go `1.26.0`, Go workspace modules, Cobra `v1.10.2`, standard `testing`, `ffmpeg`, and `ffprobe`.

## Global Constraints

- The child module path is exactly `github.com/bizshuk/agentsdk/utils/video`.
- The child module must not import `github.com/bizshuk/agentsdk`.
- Preserve the existing `auth-cli video audio|frames|subtitles` commands, flags, output, and library behavior.
- Keep `audio`, `frames`, `subtitles`, and `ffmpegutil` as separate high-cohesion packages.
- Use `NewCommand() *cobra.Command`; do not use package-global Cobra commands or flag state.
- The Qwen default script path is `utils/video/subtitles/pyasr/qwen_transcribe.py`.
- Follow TDD: each new behavior gets a failing test before production implementation.
- Use `go work sync`, `gofmt`, and fresh test/build output before claiming completion.

---

### Task 1: Scaffold the child module and write the command RED tests

`Files:`

- Create: `utils/video/go.mod`
- Create: `utils/video/cmd/video_test.go`

`Interfaces:`

- Consumes: the agreed `utils/video/cmd.NewCommand() *cobra.Command` contract.
- Produces: failing tests that define the command tree and per-call state isolation.

- [ ] `Step 1: Create the child module manifest`

Create `utils/video/go.mod` with exactly:

```go
module github.com/bizshuk/agentsdk/utils/video

go 1.26.0

require github.com/spf13/cobra v1.10.2
```

Do not add an `agentsdk` dependency. The library packages use only the standard library; Cobra is needed only by the command package.

- [ ] `Step 2: Write the failing command-construction tests`

Create `utils/video/cmd/video_test.go`:

```go
package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestNewCommandBuildsVideoCommandTree(t *testing.T) {
	got := NewCommand()
	if got == nil {
		t.Fatal("NewCommand returned nil")
	}
	if got.Use != "video" {
		t.Fatalf("Use = %q, want %q", got.Use, "video")
	}

	for _, use := range []string{"audio <video>", "frames <video>", "subtitles <video>"} {
		if findCommand(got, use) == nil {
			t.Errorf("missing child command %q", use)
		}
	}
}

func TestNewCommandDoesNotShareFlagState(t *testing.T) {
	first := NewCommand()
	second := NewCommand()
	if first == second {
		t.Fatal("NewCommand returned the same command pointer twice")
	}

	firstAudio := findCommand(first, "audio <video>")
	secondAudio := findCommand(second, "audio <video>")
	if firstAudio == nil || secondAudio == nil {
		t.Fatal("audio command missing")
	}

	firstOut := firstAudio.Flags().Lookup("out")
	secondOut := secondAudio.Flags().Lookup("out")
	if firstOut == nil || secondOut == nil {
		t.Fatal("audio --out flag missing")
	}
	if firstOut == secondOut {
		t.Fatal("NewCommand shared the audio --out flag")
	}

	if err := firstAudio.Flags().Set("out", "first.wav"); err != nil {
		t.Fatalf("set first command flag: %v", err)
	}
	if got := secondOut.Value.String(); got != "./audio.wav" {
		t.Fatalf("second command --out = %q after mutating first, want %q", got, "./audio.wav")
	}
}

func findCommand(root *cobra.Command, use string) *cobra.Command {
	for _, child := range root.Commands() {
		if child.Use == use {
			return child
		}
	}
	return nil
}
```

- [ ] `Step 3: Run the focused test to verify RED`

Run:

```bash
(cd utils/video && go test ./cmd -run 'TestNewCommand' -count=1)
```

Expected: FAIL at compile time because `NewCommand` is not defined yet. If the test fails for a missing dependency instead, run `go mod tidy` in `utils/video` and repeat until the failure is specifically the missing API.

- [ ] `Step 4: Commit the RED test and module scaffold`

```bash
git add utils/video/go.mod utils/video/cmd/video_test.go
git commit -m "test(video): define independent command contract"
```

### Task 2: Move the standalone video library into the child module

`Files:`

- Move: `video/audio/*` → `utils/video/audio/*`
- Move: `video/frames/*` → `utils/video/frames/*`
- Move: `video/ffmpegutil/*` → `utils/video/ffmpegutil/*`
- Move: `video/subtitles/*` → `utils/video/subtitles/*`
- Modify: moved Go files whose imports or package comments mention the old root path

`Interfaces:`

- Consumes: the existing library APIs and tests.
- Produces: the same APIs under `github.com/bizshuk/agentsdk/utils/video/...`, with no root-module import.

- [ ] `Step 1: Move the library source, tests, and Python wrapper`

Use repository-aware moves so Git records the migration:

```bash
mkdir -p utils/video
git mv video/audio utils/video/audio
git mv video/frames utils/video/frames
git mv video/ffmpegutil utils/video/ffmpegutil
git mv video/subtitles utils/video/subtitles
```

The destination must contain the existing implementation and tests before behavior changes are made.

- [ ] `Step 2: Update child-module import paths and stale package comments`

Replace only root video import paths inside `utils/video`:

```text
github.com/bizshuk/agentsdk/video/ffmpegutil
→ github.com/bizshuk/agentsdk/utils/video/ffmpegutil

github.com/bizshuk/agentsdk/video/audio
→ github.com/bizshuk/agentsdk/utils/video/audio
```

Update comments that still call these packages `internal/preprocess/...` so they describe `utils/video/...`. Do not alter ffmpeg arguments, option defaults, error wrapping, timestamp parsing, or transcription behavior.

- [ ] `Step 3: Generate child-module checksums and inspect import isolation`

Run:

```bash
(cd utils/video && go mod tidy)
rg -n 'github.com/bizshuk/agentsdk' utils/video --glob '*.go'
```

Expected: `go mod tidy` exits `0`. Every import match must begin with `github.com/bizshuk/agentsdk/utils/video`; there must be no import of the root module.

- [ ] `Step 4: Run the moved library tests`

```bash
(cd utils/video && go test ./audio ./frames ./ffmpegutil ./subtitles -count=1 -timeout=120s)
```

Expected: PASS, with the existing ffmpeg-dependent tests either passing or being skipped when `ffmpeg` is unavailable.

- [ ] `Step 5: Commit the library migration`

```bash
git add -A -- video utils/video
git commit -m "refactor(video): move libraries into independent module"
```

### Task 3: Implement the child-module Cobra command and turn RED green

`Files:`

- Create: `utils/video/cmd/video.go`
- Modify: `utils/video/cmd/video_test.go` only if a test assertion needs a clarified public contract

`Interfaces:`

- Consumes: `audio.Extract`, `audio.DefaultSampleRateHz`, `audio.DefaultChannels`, `ffmpegutil.Probe`, `frames.Extract`, `frames.DefaultInterval`, `subtitles.Transcriber`, `subtitles.WhisperCPPTranscriber`, `subtitles.QwenMLXTranscriber`, `subtitles.NoopTranscriber`, `subtitles.Generate`, and `subtitles.WriteSRT`.
- Produces: `NewCommand() *cobra.Command` with fresh command and flag state on every call.

- [ ] `Step 1: Create the constructor-based command implementation`

Create `utils/video/cmd/video.go` with this implementation. Keep the command text and runtime behavior identical to the old root command, except for the Qwen script default path and constructor-based state:

```go
package cmd

import (
	"fmt"
	"time"

	"github.com/bizshuk/agentsdk/utils/video/audio"
	"github.com/bizshuk/agentsdk/utils/video/ffmpegutil"
	"github.com/bizshuk/agentsdk/utils/video/frames"
	"github.com/bizshuk/agentsdk/utils/video/subtitles"
	"github.com/spf13/cobra"
)

const defaultQwenScript = "utils/video/subtitles/pyasr/qwen_transcribe.py"

type commandFlags struct {
	audioOut          string
	audioSampleRateHz int
	audioChannels     int

	framesOutDir    string
	framesInterval  time.Duration
	framesSceneThr  float64
	framesMaxFrames int

	subtitlesOut       string
	subtitlesWorkDir   string
	subtitlesKeepAudio bool
	subtitlesEngine    string
	whisperBin         string
	whisperModel       string
	whisperLang        string
	qwenScript         string
	qwenModel         string
	qwenLang           string
	qwenChunkDuration  time.Duration
}

func NewCommand() *cobra.Command {
	var flags commandFlags

	videoCmd := &cobra.Command{
		Use:   "video",
		Short: "Video preprocessing utilities (audio, frames, subtitles extraction)",
	}

	audioCmd := &cobra.Command{
		Use:   "audio <video>",
		Short: "Extract audio track from a video into a WAV file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAudio(cmd, args, &flags)
		},
	}
	audioCmd.Flags().StringVar(&flags.audioOut, "out", "./audio.wav", "output .wav path")
	audioCmd.Flags().IntVar(&flags.audioSampleRateHz, "sample-rate", audio.DefaultSampleRateHz, "sample rate in Hz")
	audioCmd.Flags().IntVar(&flags.audioChannels, "channels", audio.DefaultChannels, "number of audio channels (1=mono, 2=stereo)")

	framesCmd := &cobra.Command{
		Use:   "frames <video>",
		Short: "Extract still frames from a video into a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFrames(cmd, args, &flags)
		},
	}
	framesCmd.Flags().StringVar(&flags.framesOutDir, "out", "./frames-out", "output directory for extracted frames")
	framesCmd.Flags().DurationVar(&flags.framesInterval, "interval", frames.DefaultInterval, "sampling interval (e.g. 2s)")
	framesCmd.Flags().Float64Var(&flags.framesSceneThr, "scene-threshold", 0, "additionally sample on scene changes above this score (0..1)")
	framesCmd.Flags().IntVar(&flags.framesMaxFrames, "max-frames", 0, "cap on emitted frames (0 = unlimited)")

	subtitlesCmd := &cobra.Command{
		Use:   "subtitles <video>",
		Short: "Extract audio and transcribe it into a .srt subtitle file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSubtitles(cmd, args, &flags)
		},
	}
	subtitlesCmd.Flags().StringVar(&flags.subtitlesOut, "out", "./out.srt", "output .srt path")
	subtitlesCmd.Flags().StringVar(&flags.subtitlesWorkDir, "work-dir", "./subtitles-work", "scratch dir for the intermediate audio track")
	subtitlesCmd.Flags().BoolVar(&flags.subtitlesKeepAudio, "keep-audio", false, "keep the intermediate .wav after transcription")
	subtitlesCmd.Flags().StringVar(&flags.subtitlesEngine, "engine", "noop", "transcription engine: whisper | qwen3 | noop")
	subtitlesCmd.Flags().StringVar(&flags.whisperBin, "whisper-bin", "", "path to whisper.cpp whisper-cli/main binary (--engine whisper)")
	subtitlesCmd.Flags().StringVar(&flags.whisperModel, "whisper-model", "", "path to a ggml whisper model file (--engine whisper)")
	subtitlesCmd.Flags().StringVar(&flags.whisperLang, "whisper-lang", "auto", "whisper language code, or auto (--engine whisper)")
	subtitlesCmd.Flags().StringVar(&flags.qwenScript, "qwen-script", defaultQwenScript, "path to qwen_transcribe.py (--engine qwen3)")
	subtitlesCmd.Flags().StringVar(&flags.qwenModel, "qwen-model", "", "mlx-community model id, empty = wrapper script default (--engine qwen3)")
	subtitlesCmd.Flags().StringVar(&flags.qwenLang, "qwen-lang", "", "language code, empty = auto-detect mixed EN/ZH (--engine qwen3)")
	subtitlesCmd.Flags().DurationVar(&flags.qwenChunkDuration, "qwen-chunk-duration", 10*time.Second, "subtitle-cue chunk length (--engine qwen3)")

	videoCmd.AddCommand(audioCmd, framesCmd, subtitlesCmd)
	return videoCmd
}

func runAudio(cmd *cobra.Command, args []string, flags *commandFlags) error {
	if _, err := audio.Extract(cmd.Context(), args[0], flags.audioOut, audio.Options{
		SampleRateHz: flags.audioSampleRateHz,
		Channels:     flags.audioChannels,
	}); err != nil {
		return err
	}
	fmt.Printf("extracted audio to %s\n", flags.audioOut)
	return nil
}

func runFrames(cmd *cobra.Command, args []string, flags *commandFlags) error {
	if dur, err := ffmpegutil.Probe(cmd.Context(), args[0]); err == nil {
		fmt.Printf("source duration: %s\n", dur)
	}

	got, err := frames.Extract(cmd.Context(), args[0], frames.Options{
		Interval:       flags.framesInterval,
		SceneThreshold: flags.framesSceneThr,
		MaxFrames:      flags.framesMaxFrames,
	})
	if err != nil {
		return err
	}

	fmt.Printf("extracted %d frame(s) into %s\n", len(got), flags.framesOutDir)
	for _, frame := range got {
		fmt.Printf("  %s\t@%s\n", frame.Path, frame.Timestamp)
	}
	return nil
}

func runSubtitles(cmd *cobra.Command, args []string, flags *commandFlags) error {
	var transcriber subtitles.Transcriber
	switch flags.subtitlesEngine {
	case "whisper":
		if flags.whisperBin == "" || flags.whisperModel == "" {
			return fmt.Errorf("--engine whisper requires --whisper-bin and --whisper-model")
		}
		transcriber = subtitles.WhisperCPPTranscriber{
			BinPath:   flags.whisperBin,
			ModelPath: flags.whisperModel,
			Language:  flags.whisperLang,
		}
	case "qwen3":
		transcriber = subtitles.QwenMLXTranscriber{
			ScriptPath:    flags.qwenScript,
			Model:         flags.qwenModel,
			Language:      flags.qwenLang,
			ChunkDuration: flags.qwenChunkDuration,
		}
	case "noop":
		fmt.Println("warning: --engine noop produces 0 segments; pass --engine whisper or --engine qwen3 for real transcription")
		transcriber = subtitles.NoopTranscriber{}
	default:
		return fmt.Errorf("unknown --engine %q: want whisper | qwen3 | noop", flags.subtitlesEngine)
	}

	segments, err := subtitles.Generate(cmd.Context(), args[0], flags.subtitlesWorkDir, transcriber, flags.subtitlesKeepAudio)
	if err != nil {
		return err
	}
	if err := subtitles.WriteSRT(segments, flags.subtitlesOut); err != nil {
		return err
	}

	fmt.Printf("wrote %d segment(s) to %s\n", len(segments), flags.subtitlesOut)
	return nil
}
```

- [ ] `Step 2: Format and run the focused command tests to verify GREEN`

```bash
gofmt -w utils/video/cmd/video.go utils/video/cmd/video_test.go
(cd utils/video && go test ./cmd -run 'TestNewCommand' -count=1)
```

Expected: PASS.

- [ ] `Step 3: Run all child-module tests`

```bash
(cd utils/video && go test ./... -count=1 -timeout=120s)
```

Expected: PASS, with only environment-dependent ffmpeg/MLX end-to-end tests skipped as already encoded by the existing tests.

- [ ] `Step 4: Commit the child command implementation`

```bash
git add utils/video/cmd
git commit -m "feat(video): expose reusable cobra command"
```

### Task 4: Reconnect the root CLI and workspace modules

`Files:`

- Create: `cmd/root_test.go`
- Modify: `cmd/root.go`
- Delete: `cmd/video.go`
- Modify: `go.mod`
- Modify: `go.work`

`Interfaces:`

- Consumes: `github.com/bizshuk/agentsdk/utils/video/cmd.NewCommand`.
- Produces: a root command tree with an independently constructed `video` command and a workspace that builds both modules.

- [ ] `Step 1: Write the failing root isolation regression test`

Create `cmd/root_test.go`:

```go
package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestNewRootBuildsIndependentVideoCommands(t *testing.T) {
	first := findRootChild(t, NewRoot(), "video")
	second := findRootChild(t, NewRoot(), "video")
	if first == second {
		t.Fatal("NewRoot reused the same video command pointer")
	}
}

func findRootChild(t *testing.T, root *cobra.Command, use string) *cobra.Command {
	t.Helper()
	for _, child := range root.Commands() {
		if child.Use == use {
			return child
		}
	}
	t.Fatalf("root command %q not found", use)
	return nil
}
```

- [ ] `Step 2: Run the root regression test to verify RED`

```bash
go test ./cmd -run 'TestNewRootBuildsIndependentVideoCommands' -count=1
```

Expected: FAIL because the existing root `videoCmd` global is reused by both `NewRoot()` calls.

- [ ] `Step 3: Add the child module to the workspace and root dependency graph`

Add `./utils/video` to the `use` block in `go.work`.

Add this direct requirement to the root `go.mod`:

```go
require github.com/bizshuk/agentsdk/utils/video v0.0.0
```

Add this local-development replacement to the root `go.mod`:

```go
replace github.com/bizshuk/agentsdk/utils/video => ./utils/video
```

Keep the child module independent: do not add a reverse replacement or a child dependency on the root module.

- [ ] `Step 4: Replace the root video command with the child constructor`

In `cmd/root.go`, add the import:

```go
videocmd "github.com/bizshuk/agentsdk/utils/video/cmd"
```

Replace the `videoCmd` entry in `root.AddCommand` with:

```go
videocmd.NewCommand(),
```

Delete `cmd/video.go`. Do not leave a compatibility wrapper under the old `video/...` or `cmd/video.go` paths.

- [ ] `Step 5: Format, sync, and run the root regression test to verify GREEN`

```bash
gofmt -w cmd/root.go cmd/root_test.go
go work sync
go test ./cmd -run 'TestNewRootBuildsIndependentVideoCommands' -count=1
```

Expected: PASS.

- [ ] `Step 6: Commit the workspace and root CLI integration`

```bash
git add cmd go.mod go.work go.work.sum
git commit -m "refactor(cli): compose video command from child module"
```

### Task 5: Add child-module documentation and synchronize root documentation

`Files:`

- Create: `utils/video/README.md`
- Create: `utils/video/CLAUDE.md`
- Create symlink: `utils/video/AGENTS.md` → `CLAUDE.md`
- Create: `utils/video/README.todo`
- Create: `utils/video/docs/memory/README.md`
- Modify: `README.md`
- Modify: `CLAUDE.md`

`Interfaces:`

- Consumes: the final module tree, module path, CLI constructor, and verification commands.
- Produces: documentation that describes the same package boundary and dependency direction as the code.

- [ ] `Step 1: Write the child module README`

Create `utils/video/README.md`:

````markdown
# Video Utilities

Standalone Go module for ffmpeg-based media preprocessing:
`github.com/bizshuk/agentsdk/utils/video`.

## Packages

- `audio`: extract an audio track into a transcriber-ready WAV.
- `frames`: sample still frames with interval and scene-change options.
- `subtitles`: extract audio, run a pluggable transcriber, and write SRT.
- `ffmpegutil`: check ffmpeg availability and probe media duration.
- `cmd`: reusable Cobra command tree returned by `NewCommand()`.

The module does not import `github.com/bizshuk/agentsdk`.

## Requirements

- Go `1.26+`
- `ffmpeg` and `ffprobe` on `PATH`
- Optional Whisper.cpp or Qwen3/MLX runtime for real transcription

## Development

```bash
go test ./... -count=1 -timeout=120s
```

The root `auth-cli` composes the reusable command as `auth-cli video`.
````

- [ ] `Step 2: Write child technical context and required workspace files`

Create `utils/video/CLAUDE.md` describing the module path, Go version, package tree, one-way dependencies, the `NewCommand() *cobra.Command` contract, runtime requirements, test/build commands, and the rule that the module must not import `agentsdk`.

Create `utils/video/README.todo`:

```markdown
# TODO

## Release

- [ ] Publish the first tagged version of `github.com/bizshuk/agentsdk/utils/video`.

## Archive
```

Create `utils/video/docs/memory/README.md` explaining that the directory stores historical video-module decisions and retrospectives. Create the required symlink:

```bash
ln -s CLAUDE.md utils/video/AGENTS.md
```

- [ ] `Step 3: Update root README and CLAUDE.md claims`

Update the root documents consistently:

- Change the workspace module count from `11` to `12`.
- Replace the root `video/` tree entry with `utils/video/` and identify it as an independent module.
- Add `./utils/video` to the module list and all multi-module verification loops.
- State that root `cmd/root.go` composes `utils/video/cmd.NewCommand()`.
- Update the CLI command mapping so `video` belongs to the child module while `auth-cli` remains the root binary.
- Remove any old import path claims using `github.com/bizshuk/agentsdk/video/...`.
- Keep the existing Agent SDK core/proxy/auth descriptions unchanged.

- [ ] `Step 4: Run documentation consistency checks`

```bash
rg -n '11 個 module|github.com/bizshuk/agentsdk/video|cmd/video.go' README.md CLAUDE.md utils/video --glob '*.md' --glob '*.go'
```

Expected: no stale root-module claims; any remaining `video` match must be part of the new `utils/video` path or the `auth-cli video` command.

- [ ] `Step 5: Commit documentation`

```bash
git add README.md CLAUDE.md utils/video
git commit -m "docs(video): document independent module boundary"
```

### Task 6: Run complete verification and inspect the final migration

`Files:`

- Verify: all changed files and workspace metadata.

`Interfaces:`

- Consumes: the implementation from Tasks 1–5.
- Produces: fresh evidence that the child module, root module, and workspace remain buildable and behaviorally compatible.

- [ ] `Step 1: Confirm the final path and dependency boundaries`

```bash
git status --short
test -f utils/video/go.mod
test -L utils/video/AGENTS.md
test "$(readlink utils/video/AGENTS.md)" = "CLAUDE.md"
! test -e video
! test -e cmd/video.go
! rg -n 'github.com/bizshuk/agentsdk/video' . --glob '*.go' --glob '*.md'
```

Expected: only intended changes are present, the symlink points to `CLAUDE.md`, old directories/files are absent, and no old import path remains.

- [ ] `Step 2: Synchronize and test the child module`

```bash
go work sync
(cd utils/video && go test ./... -count=1 -timeout=120s)
```

Expected: PASS, with only the existing environment-gated tests skipped.

- [ ] `Step 3: Test and build the root module`

```bash
go test ./... -count=1 -timeout=120s
go build ./...
```

Expected: both commands exit `0`.

- [ ] `Step 4: Test every workspace module`

```bash
for mod in utils/video mcp provider/anthropic provider/google provider/openaicompat \
  sample/file-agent sample/greet-agent sample/logdoctor sample/memory-demo \
  sample/middleware-demo sample/strategy-demo; do
  (cd "$mod" && go test ./... -count=1 -timeout=120s) || exit 1
done
```

Expected: every module exits `0`; report any environment-gated skips without treating them as failures.

- [ ] `Step 5: Verify the CLI command surface`

```bash
go run . video --help
go run . video audio --help
go run . video frames --help
go run . video subtitles --help
```

Expected: each command exits `0`, shows the existing usage and flags, and does not require ffmpeg merely to render help.

- [ ] `Step 6: Review diff and commit only after evidence is captured`

```bash
git diff --check
git status --short
git diff --stat
```

Confirm the final diff contains only the approved module extraction, root CLI composition, module metadata, and documentation synchronization. Do not claim completion until all prior commands have fresh exit-0 evidence.
