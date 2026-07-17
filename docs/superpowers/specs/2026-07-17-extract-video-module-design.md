# Extract Video Utilities Into an Independent Module

## Goal

Extract the video preprocessing utilities from the `agentsdk` root module into
the independently versionable module
`github.com/bizshuk/agentsdk/utils/video`, while keeping the existing
`auth-cli video` command available through a small composition adapter.

## Scope

### In scope

- Move the existing `video` library packages into `utils/video`:
  - `audio`
  - `frames`
  - `subtitles`
  - `ffmpegutil`
- Move `cmd/video.go` into `utils/video/cmd/video.go`.
- Expose `utils/video/cmd.NewCommand() *cobra.Command` for embedding by the
  root CLI.
- Add `utils/video/go.mod` and include the module in `go.work`.
- Update the root module dependency and root CLI registration.
- Preserve the existing `auth-cli video audio|frames|subtitles` command names,
  flags, output shape, and library behavior.
- Update root and module documentation to describe the new boundary.

### Out of scope

- Adding a second standalone video binary.
- Adding a facade that re-exports all child package APIs from a root `video`
  package.
- Changing ffmpeg invocation semantics, subtitle formats, sampling behavior,
  transcription backends, or provider/agent APIs.
- Extracting other root packages such as `auth`, `proxy`, `config`, `cli`, or
  `internal/testutil`.

## Current architecture

The video packages currently live in the root module and are only consumed by
the root CLI:

```text
agentsdk/cmd/video.go
  ├── agentsdk/video/audio
  ├── agentsdk/video/frames
  ├── agentsdk/video/subtitles
  └── agentsdk/video/ffmpegutil
```

The library packages do not import `core`, `runtime`, `action`, providers, or
other Agent SDK packages. `subtitles` depends on `audio`, while `audio` and
`frames` share only `ffmpegutil`.

## Target architecture

```text
github.com/bizshuk/agentsdk
└── cmd/root.go
    └── github.com/bizshuk/agentsdk/utils/video/cmd.NewCommand()

github.com/bizshuk/agentsdk/utils/video
├── audio        ──> ffmpegutil
├── frames       ──> ffmpegutil
├── subtitles    ──> audio
├── ffmpegutil
└── cmd          ──> audio, frames, subtitles, ffmpegutil
```

The `utils/video` module must not import the root `agentsdk` module. The root
module may depend on `utils/video` only to compose its CLI. No library package
in the new module may depend on the `cmd` package.

## Module boundary

The new module uses:

```text
module github.com/bizshuk/agentsdk/utils/video
go 1.26.0
```

Its library packages use only the Go standard library and the installed
`ffmpeg`/`ffprobe` executables. The `cmd` package adds the existing
`github.com/spf13/cobra v1.10.2` dependency. Tests continue to use the standard
testing package and retain their existing skip behavior when ffmpeg is not
available.

The root module will require the video module at the workspace development
version and use a local replacement for this checkout. `go.work` will list
`./utils/video` as a workspace module. This keeps local builds reproducible
while allowing the child module to receive its own tags and releases later.

## CLI composition contract

`utils/video/cmd` will expose exactly one composition entry point:

```go
func NewCommand() *cobra.Command
```

`NewCommand` constructs a fresh command tree with fresh flag storage on every
call. The root `agentsdk/cmd/root.go` will import this package under an alias
and add `videocmd.NewCommand()` to the existing root command. The old root
`cmd/video.go` file will be removed.

The following command surface remains stable:

```text
auth-cli video audio <video>
auth-cli video frames <video>
auth-cli video subtitles <video>
```

All current flags remain available. The Qwen wrapper's repository-relative
default path changes from `video/subtitles/pyasr/qwen_transcribe.py` to
`utils/video/subtitles/pyasr/qwen_transcribe.py`, matching its new location.
Users running the command outside this checkout can continue to provide an
explicit `--qwen-script` path.

## File and documentation changes

### New module files

- `utils/video/go.mod`
- `utils/video/go.sum`
- `utils/video/README.md`
- `utils/video/CLAUDE.md`
- `utils/video/AGENTS.md` as a symlink to `CLAUDE.md`
- `utils/video/README.todo`
- `utils/video/docs/memory/README.md`
- `utils/video/cmd/video.go`

### Moved files

- `video/audio/*` → `utils/video/audio/*`
- `video/frames/*` → `utils/video/frames/*`
- `video/ffmpegutil/*` → `utils/video/ffmpegutil/*`
- `video/subtitles/*` → `utils/video/subtitles/*`

### Root files

- `go.work`: add `./utils/video`.
- `go.mod`: require and locally replace the video module for workspace builds.
- `cmd/root.go`: register `videocmd.NewCommand()`.
- `cmd/video.go`: remove after migration.
- `README.md`: describe `utils/video` as an independent module and remove the
  root `video/` entry.
- `CLAUDE.md`: update module count, tree, module mapping, CLI composition, and
  verification commands.

## Error handling and behavior

The move must preserve existing error wrapping and external command behavior.
No new fallback or silent error handling is introduced. The command package
continues to return errors from the underlying library calls through Cobra.
The existing best-effort duration display before frame extraction remains
unchanged.

## Testing strategy

The migration will follow a red-green-refactor cycle:

1. Add command-construction tests that initially fail for the new
   `NewCommand` API.
2. Move the library tests with the library and update imports to the new module
   paths.
3. Implement `NewCommand` with fresh per-call state and make the command tests
   pass.
4. Verify the root CLI still exposes the `video` command and that repeated root
   construction does not reuse command state.
5. Run the child module tests, root module tests, all workspace module tests,
   and builds after the migration.

The final verification must include:

```bash
go work sync
(cd utils/video && go test ./... -count=1 -timeout=120s)
go test ./... -count=1 -timeout=120s
go build ./...
```

The existing multi-module verification loop must also include `utils/video`.

## Acceptance criteria

- `utils/video` is an independent module with the exact module path above.
- No file under `utils/video` imports `github.com/bizshuk/agentsdk`.
- The root module no longer contains `video/` or `cmd/video.go`.
- `auth-cli video` retains its three subcommands and existing flags.
- `NewCommand()` returns an independent command tree on each call.
- Library tests and command tests pass in the child module.
- Root and workspace builds/tests pass with the new module listed in
  `go.work`.
- Root and child module documentation accurately describe the final structure.
