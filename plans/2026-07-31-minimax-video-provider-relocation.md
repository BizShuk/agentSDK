# MiniMax Video Provider Relocation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: execute inline with test-driven-development; do not delegate or create commits in this session.

**Goal:** Move the MiniMax video-generation provider and transport from `ip-incubation/svc/video` into AgentSDK while preserving the four-mode CLI behavior.

**Architecture:** AgentSDK owns a provider-neutral `VideoGenerator` optional capability and the MiniMax implementation under `provider/minimax`. `ip-incubation` remains the application-level Cobra consumer, blank-imports the MiniMax adapter for registration, and keeps a versioned AgentSDK requirement. Local integration of the unpublished capability uses an ignored `go.work`, never a `go.mod` replace.

**Tech Stack:** Go 1.26, stdlib `net/http`, AgentSDK provider registry, Cobra, `testify`.

## Global Constraints

- Preserve unrelated `ip-incubation/pkg/seedance-2.0` and `pkg/one-degree-north/` work.
- Keep `core/` stdlib-only and keep video generation outside the agent runtime.
- Keep all four modes: text, image, start/end frame, and subject reference.
- Keep context cancellation, polling, authenticated download, and MP4 `ftyp` verification.
- Do not call the live MiniMax API during tests.
- Do not commit, push, or tag.

---

### Task 1: Add the provider-neutral video capability

**Files:**

- Create: `provider/video.go`
- Modify: `provider/registry.go`
- Modify: `provider/capability.go`
- Modify: `provider/registry_test.go`
- Test: `provider/video_test.go`

**Interfaces:**

- Produces: `VideoGenerator.GenerateVideo(context.Context, VideoRequest) (VideoResult, error)`.
- Produces: `VideoFactory`, `Entry.NewVideo`, `NewVideo`, `WithVideoDecorator`.
- Produces: `VIDEO_MODE_TEXT`, `VIDEO_MODE_IMAGE`, `VIDEO_MODE_START_END`, `VIDEO_MODE_SUBJECT`.
- Produces: `CAPABILITY_VIDEO_GENERATE`.

- [x] **Step 1: Write failing contract and registry tests**

Cover request validation, per-request credential decoration, explicit-auth precedence,
capability discovery, unsupported capability errors, and MiniMax construction.

- [x] **Step 2: Verify RED**

Run:

```bash
go test ./provider -run 'Test(Video|NewVideo)' -count=1
```

Expected: compile failure because the video capability symbols do not exist.

- [x] **Step 3: Implement the minimal provider contract**

Keep `VideoRequest` provider-neutral. Put vendor prompt limits, model defaults, wire
payloads, polling behavior, and file verification in the adapter.

- [x] **Step 4: Verify GREEN**

Run:

```bash
go test ./provider -count=1
```

Expected: PASS.

### Task 2: Move the MiniMax implementation into AgentSDK

**Files:**

- Create: `provider/minimax/video.go`
- Create: `provider/minimax/video_client.go`
- Create: `provider/minimax/video_dto.go`
- Create: `provider/minimax/video_file.go`
- Create: `provider/minimax/video_test.go`
- Modify: `provider/minimax/config.go`
- Modify: `provider/minimax/register.go`

**Interfaces:**

- Consumes: `provider.VideoGenerator`, `provider.VideoRequest`, `provider.VideoResult`.
- Produces: `minimax.NewVideo(provider.ResolvedConfig)`.
- Produces: a MiniMax generator with a 3000-rune prompt limit.

- [x] **Step 1: Write failing adapter tests**

Use `httptest` to cover every mode payload, bearer auth, queued-to-success polling,
API failure, download, and MP4 verification.

- [x] **Step 2: Verify RED**

Run:

```bash
go test ./provider/minimax -run Video -count=1
```

Expected: compile failure because `NewVideo` is not defined.

- [x] **Step 3: Implement the adapter and transport**

Use `/v1/video_generation`, `/v1/query/video_generation`, and
`/v1/files/retrieve`; download the returned asset to `VideoRequest.OutputPath`.
Use `MINIMAX_VIDEO_BASE_URL` for a video-specific endpoint override.

- [x] **Step 4: Verify GREEN**

Run:

```bash
go test ./provider/minimax ./provider -count=1
```

Expected: PASS with no live network access.

### Task 3: Rewire ip-incubation and remove duplicate ownership

**Files:**

- Create: `cmd/video/provider.go`
- Create: `cmd/video/provider_test.go`
- Modify: `cmd/video/video.go`
- Modify: `cmd/video/text.go`
- Modify: `cmd/video/image.go`
- Modify: `cmd/video/startend.go`
- Modify: `cmd/video/subject.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Delete: `svc/video/`
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: AgentSDK `README.md`
- Modify: AgentSDK `CLAUDE.md`
- Modify: AgentSDK `docs/CHANGELOG.md`

**Interfaces:**

- Consumes: `provider.NewVideo`, `provider.VideoRequest`, and `provider.VideoResult`.
- Produces: unchanged `ip-incubation video text|image|startend|subject` CLI surface.

- [x] **Step 1: Write the failing CLI provider integration test**

Assert that the linked MiniMax adapter resolves through AgentSDK and reports the
3000-rune limit.

- [x] **Step 2: Verify RED**

Run:

```bash
go test ./cmd/video -run VideoProvider -count=1
```

Expected: compile failure because the local AgentSDK-backed lookup helper is absent.

- [x] **Step 3: Rewire the commands and delete `svc/video`**

Keep signal handling and output naming in the CLI. Construct one
`provider.VideoRequest` per command without storing mode-specific data in shared
global flag state.

- [x] **Step 4: Synchronize canonical docs and module metadata**

Describe AgentSDK as the provider owner and `ip-incubation` as the consumer. Add a
versioned AgentSDK requirement without a local replace. Use an ignored `go.work`
to compose both checkouts until the next AgentSDK release contains the new capability;
after release, bump the requirement and verify once with `GOWORK=off`.

- [x] **Step 5: Verify both repositories**

Run:

```bash
go test ./...
go build ./...
```

in `ip-incubation`, then:

```bash
go test ./...
bash scripts/verify-workspace.sh
```

in AgentSDK. Also run `git diff --check` and confirm no stale
`github.com/bizshuk/ip-incubation/svc/video` imports remain.

### Task 4: Publish the cross-repo dependency

- [ ] Publish an AgentSDK version containing the new video capability.
- [ ] Bump `ip-incubation` from `v0.0.29` to that version.
- [ ] Remove the temporary ignored `ip-incubation/go.work` and verify with
  `GOWORK=off`.
