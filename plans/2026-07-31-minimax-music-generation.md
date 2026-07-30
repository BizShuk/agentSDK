# MiniMax Music Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: execute inline with
> `test-driven-development`; do not delegate or create commits in this session.

**Goal:** Add a provider-neutral, non-streaming `MusicGenerator` capability and
implement MiniMax `POST /v1/music_generation`, including the one-step
`music-cover` request supplied by the user.

**Architecture:** Music generation remains an optional `provider` capability,
separate from `core.Provider` and from the intentionally unresolved generic
`audio` API. `provider.Entry` owns discovery and construction,
`provider/minimax` owns MiniMax model rules and wire transport, and
`provider/sample` is the executable consumer.

**Tech Stack:** Go 1.26, stdlib `net/http` / `encoding/json`, AgentSDK provider
registry, Cobra, `httptest`, `testify`.

## Global Constraints

- Preserve `core/` as stdlib-only and do not add music instructions to the
  agent runtime.
- Keep `speech synthesis`, `transcription`, `audio-chat input`, lyrics
  generation, cover preprocess, and music streaming outside this change.
- Support the supplied `music-cover` fields: `model`, `audio_url`, `prompt`,
  `audio_setting`, and `output_format`.
- Also preserve the current MiniMax model override path so callers may select
  `music-3.0`, `music-2.6`, or free-tier variants without a registry change.
- Credential precedence remains
  `request Auth → Options.APIKey → Decorator → environment`.
- Use a music-specific `MINIMAX_MUSIC_BASE_URL`; never send music requests to
  the Anthropic-compatible `MINIMAX_BASE_URL`.
- Bound success and error response bodies; never include credentials or an
  unbounded upstream body in an error.
- Do not call the paid live MiniMax API during automated verification.
- Do not commit, push, or tag.

---

### Task 1: Add the provider-neutral music capability

**Files:**

- Create: `provider/music.go`
- Create: `provider/music_test.go`
- Modify: `provider/adapter.go`
- Modify: `provider/capability.go`
- Modify: `provider/registry.go`
- Modify: `provider/registry_test.go`

**Interfaces:**

- Produces:
  `MusicGenerator.GenerateMusic(context.Context, MusicRequest) (MusicResult, error)`.
- Produces: `MusicFactory`, `Entry.NewMusic`, `NewMusic`, and
  `WithMusicDecorator`.
- Produces: `CAPABILITY_MUSIC_GENERATE`.
- Produces: `MusicRequest`, `MusicAudioSetting`, `MusicAsset`, `MusicInfo`, and
  `MusicResult`.
- Produces: `Metadata.MusicBaseURLEnv`.

- [ ] **Step 1: Write failing contract tests**

  Add tests that exercise provider-independent request validation, per-request
  credential decoration, explicit-auth precedence, and decorator failure.
  `MusicRequest.Validate` must reject an empty request, multiple simultaneous
  cover sources, negative sample rate, and negative bitrate.

- [ ] **Step 2: Write failing registry tests**

  Assert MiniMax advertises `CAPABILITY_MUSIC_GENERATE`, all other built-in
  adapters do not, unsupported construction returns
  `*UnsupportedCapabilityError`, and a request decorator permits deferred
  credential construction.

- [ ] **Step 3: Verify RED**

  Run:

  ```bash
  go test ./provider -run 'Test(Music|NewMusic)' -count=1
  ```

  Expected: compile failure because the music capability symbols do not exist.

- [ ] **Step 4: Implement the minimal contract**

  Add these public shapes without importing them into `core`:

  ```go
  type MusicGenerator interface {
      GenerateMusic(context.Context, MusicRequest) (MusicResult, error)
  }

  type MusicAudioSetting struct {
      SampleRate int
      Bitrate    int
      Format     string
  }

  type MusicRequest struct {
      Model            string
      Prompt           string
      Lyrics           string
      AudioURL         string
      AudioBase64      string
      CoverFeatureID   string
      LyricsOptimizer  bool
      Instrumental     bool
      OutputFormat     string
      AudioSetting     MusicAudioSetting
      Auth             core.Auth
  }
  ```

  `MusicAsset` separates `URL` from `Hex`; `MusicInfo` exposes duration in
  milliseconds, sample rate, channels, bitrate, and size in bytes.

- [ ] **Step 5: Implement registry discovery and construction**

  Extend the one-shot registration invariant to accept `NewMusic`, add stable
  capability ordering after video, resolve `Metadata.MusicBaseURLEnv`, and wrap
  the result with `WithMusicDecorator`.

- [ ] **Step 6: Verify GREEN**

  Run:

  ```bash
  go test ./provider -count=1
  ```

  Expected: PASS.

### Task 2: Implement the MiniMax music adapter

**Files:**

- Create: `provider/minimax/music.go`
- Create: `provider/minimax/music_client.go`
- Create: `provider/minimax/music_dto.go`
- Create: `provider/minimax/music_test.go`
- Modify: `provider/minimax/config.go`
- Modify: `provider/minimax/register.go`

**Interfaces:**

- Consumes: `provider.MusicGenerator`, `provider.MusicRequest`, and
  `provider.MusicResult`.
- Produces: `minimax.NewMusic(provider.ResolvedConfig)`.
- Uses: `POST /v1/music_generation` with bearer authentication.
- Defaults: `music-3.0` without a cover source; `music-cover` with a cover
  source.

- [ ] **Step 1: Write the failing supplied-contract test**

  Use `httptest` to submit:

  ```json
  {
    "model": "music-cover",
    "audio_url": "https://example.com/original-song.mp3",
    "prompt": "Jazz, smooth, late night lounge, saxophone",
    "audio_setting": {
      "sample_rate": 44100,
      "bitrate": 256000,
      "format": "mp3"
    },
    "output_format": "url"
  }
  ```

  Assert `POST /v1/music_generation`, `Authorization: Bearer test-key`, exact
  field names, URL result mapping, status, trace ID, and audio metadata.

- [ ] **Step 2: Add failing error and boundary tests**

  Cover explicit request-auth precedence, decorator-based construction,
  `MINIMAX_MUSIC_BASE_URL`, non-zero `base_resp.status_code`, non-2xx HTTP
  errors, missing audio, incomplete status, oversized success payload, and
  context cancellation.

- [ ] **Step 3: Add failing MiniMax validation tests**

  Cover prompt and lyrics rune limits, cover prompt minimum/maximum, required
  cover source, allowed output formats (`url`, `hex`), sample rates
  (`16000`, `24000`, `32000`, `44100`), bitrates
  (`32000`, `64000`, `128000`, `256000`), and audio formats
  (`mp3`, `wav`, `pcm`).

- [ ] **Step 4: Verify RED**

  Run:

  ```bash
  go test ./provider/minimax -run Music -count=1
  ```

  Expected: compile failure because `NewMusic` is not defined.

- [ ] **Step 5: Implement request mapping and validation**

  Keep MiniMax model-specific limits in `provider/minimax/music.go`. Do not
  reject unknown explicit model IDs solely because the static list may change;
  apply cover rules to `music-cover` / `music-cover-free` and general
  non-streaming limits to other models.

- [ ] **Step 6: Implement bounded HTTP transport**

  `music_client.go` owns request creation, bearer/custom headers, a
  `128 MiB` success limit, a `1 MiB` error limit, and structured
  `provider.APIError` mapping. A successful MiniMax envelope must have
  `base_resp.status_code == 0`, `data.status == 2`, and non-empty audio.

- [ ] **Step 7: Implement result mapping**

  Map `data.audio` to `MusicAsset.URL` for `output_format=url`, otherwise to
  `MusicAsset.Hex`. Map `extra_info.music_duration`,
  `music_sample_rate`, `music_channel`, `bitrate`, and `music_size` without
  preserving the full wire DTO.

- [ ] **Step 8: Verify GREEN**

  Run:

  ```bash
  go test ./provider/minimax ./provider -count=1
  ```

  Expected: PASS with no live network access.

### Task 3: Add an executable provider/sample consumer

**Files:**

- Create: `provider/sample/svc/music.go`
- Modify: `provider/sample/config/api_type.go`
- Modify: `provider/sample/config/config.go`
- Modify: `provider/sample/config/config_test.go`
- Modify: `provider/sample/cmd.go`
- Modify: `provider/sample/matrix.go`
- Modify: `provider/sample/run.go`
- Modify: `provider/sample/run_test.go`
- Modify: `provider/sample/svc/request.go`
- Modify: `provider/sample/svc/svc_test.go`
- Modify: `provider/sample/README.md`

**Interfaces:**

- Adds `--type music`; keeps `--type audio` as typed unsupported.
- Adds `--audio-url`, `--lyrics`, `--output-format`, `--sample-rate`,
  `--bitrate`, and `--audio-format`.
- Non-JSON output prints a returned URL or hex character count, never a full
  hex payload.

- [ ] **Step 1: Write failing CLI and service tests**

  Assert the supplied cover request passes through the sample, the matrix
  reports MiniMax music support, and non-JSON output prints the returned URL.
  Keep the existing generic audio unsupported test unchanged.

- [ ] **Step 2: Verify RED**

  Run:

  ```bash
  go test ./provider/sample/... -run Music -count=1
  ```

  Expected: compile failure because `API_TYPE_MUSIC` and `svc.Music` do not
  exist.

- [ ] **Step 3: Implement CLI mapping**

  Preserve the prompt positional argument. Default the sample music output to
  `url` and audio settings to `44100 / 256000 / mp3`, matching the supplied
  request. Forward `--model music-cover` and `--audio-url` without embedding a
  credential in flags.

- [ ] **Step 4: Implement safe output**

  JSON mode emits the complete folded `MusicResult`. Text mode emits
  `music.url=...` or `music.hex_chars=N` plus status, trace ID, duration,
  sample rate, bitrate, and size.

- [ ] **Step 5: Verify GREEN**

  Run:

  ```bash
  go test ./provider/sample/... -count=1
  ```

  Expected: PASS.

### Task 4: Synchronize canonical docs and verify the workspace

**Files:**

- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `README.todo`
- Modify: `docs/CHANGELOG.md`
- Modify: `docs/terminology.md`

**Interfaces:**

- `README.md` owns the getting-started example.
- `CLAUDE.md` owns package boundaries, structure, and registry decisions.
- `README.todo` records only the missing live credential smoke test and future
  streaming/preprocess work.
- `docs/CHANGELOG.md` records the completed implementation.
- `docs/terminology.md` defines `MusicGenerator` once.

- [ ] **Step 1: Update the business-facing example**

  Show the Go equivalent of the supplied `music-cover` request and state that
  returned URLs expire according to MiniMax policy.

- [ ] **Step 2: Update technical ownership**

  Add `MusicGenerator`, `Entry.NewMusic`, `provider.NewMusic`,
  `CAPABILITY_MUSIC_GENERATE`, `MINIMAX_MUSIC_BASE_URL`, and the MiniMax music
  files to the canonical architecture descriptions.

- [ ] **Step 3: Record verification boundaries**

  State that deterministic `httptest` covers the wire contract, while no live
  paid request was executed without a real operator-owned API key and source
  audio.

- [ ] **Step 4: Format and run targeted verification**

  Run:

  ```bash
  gofmt -w provider/music.go provider/music_test.go \
    provider/adapter.go provider/capability.go provider/registry.go \
    provider/registry_test.go provider/minimax/music.go \
    provider/minimax/music_client.go provider/minimax/music_dto.go \
    provider/minimax/music_test.go provider/minimax/config.go \
    provider/minimax/register.go provider/sample/config/api_type.go \
    provider/sample/config/config.go provider/sample/config/config_test.go \
    provider/sample/cmd.go provider/sample/matrix.go provider/sample/run.go \
    provider/sample/run_test.go provider/sample/svc/request.go \
    provider/sample/svc/music.go provider/sample/svc/svc_test.go
  go test ./provider/... -count=1
  ```

  Expected: formatting changes only in scoped Go files; tests PASS.

- [ ] **Step 5: Run full verification**

  Run:

  ```bash
  go test ./...
  go vet ./...
  bash scripts/verify-workspace.sh
  git diff --check
  ```

  Expected: every command exits `0`. Do not claim live MiniMax acceptance.

## Official Contract Sources

- [MiniMax Music Generation API](https://platform.minimax.io/docs/api-reference/music-generation)
- [MiniMax Music Generation Guide](https://platform.minimax.io/docs/guides/music-generation)
