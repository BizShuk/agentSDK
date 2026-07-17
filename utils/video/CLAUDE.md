# CLAUDE.md — video utilities technical context

`utils/video` is an independently versionable Go module for media preprocessing.
Its module path is `github.com/bizshuk/agentsdk/utils/video` and it uses Go
`1.26.0`.

## Structure

```text
utils/video/
├── go.mod
├── audio/       # video/audio track to transcriber-ready WAV
├── frames/      # interval and scene-change still-frame sampling
├── subtitles/   # audio extraction, transcription, and SRT output
├── ffmpegutil/  # ffmpeg availability and ffprobe duration helpers
└── cmd/         # reusable Cobra command tree
```

Dependencies are one-way:

```text
audio ──> ffmpegutil
frames ──> ffmpegutil
subtitles ──> audio
cmd ──> audio, frames, subtitles, ffmpegutil
```

The module must not import `github.com/bizshuk/agentsdk`. The root SDK imports
only `cmd` to compose `auth-cli video`; the library packages do not depend on
the command package.

## Public command contract

`cmd.NewCommand() *cobra.Command` returns a new command tree with independent
flag state on every call. It provides `audio`, `frames`, and `subtitles`
subcommands and keeps their existing flags and output behavior.

## Runtime requirements

Library operations invoke `ffmpeg` and `ffprobe` from `PATH`. Real subtitles
also require either Whisper.cpp or the Qwen3/MLX wrapper runtime. Tests that
need optional system runtimes skip themselves when those runtimes are absent.

## Development

```bash
go mod tidy
go test ./... -count=1 -timeout=120s
go build ./...
```
