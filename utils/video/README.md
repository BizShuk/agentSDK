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
