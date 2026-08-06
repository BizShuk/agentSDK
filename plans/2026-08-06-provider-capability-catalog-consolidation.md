# Provider Capability 與 Model Catalog Consolidation Plan

日期：2026-08-06
狀態：Scope review，尚未開始 implementation

> 執行時依序使用 `test-driven-development` 與
> `verification-before-completion`；本計畫只定義 change scope，不授權 live API、
> commit 或 push。

## Goal

以 `provider.Capability` 作為 provider discovery、model catalog、CLI 與 benchmark
共用的 operation vocabulary，移除 `benchmark.Kind` 與 model-name inference。
同時把 catalog metadata 從 `core` 移至 `provider`，明確分開：

- `Capability`：model 可以執行哪一種 operation。
- `InputModalities` / `OutputModalities`：model 可以讀取與產生哪些 data form。
- `Entry`：provider 整體提供哪些 API surface。
- `ModelSpec`：該 provider 內的特定 model 可以服務哪些 capability。

這會讓 consumer 能以 `Provider + Capability + Model` 選擇同一 provider 下的不同
model，例如 chat、image、video 各用一個 model；實際 request 仍走既有 typed
interface。

## 1. Scope 決策

### 1.1 Canonical Capability Vocabulary

`provider` 保留唯一一組 capability vocabulary：

| Capability | 意義 | Model-level | Benchmark |
| --- | --- | --- | --- |
| `chat` | paired blocking + streaming model interaction | 是 | 是 |
| `catalog` | provider model discovery | 否 | 否 |
| `image` | image generation | 是 | 是 |
| `video` | video generation | 是 | 是 |
| `music` | music generation | 是 | 是 |
| `transcribe` | audio to text | 是 | 是 |
| `speech` | text to audio | 是 | 是 |
| `live` | realtime session | 是 | 否 |
| `translate` | translation session | 是 | 否 |

`model_generate` 與 `model_stream` 合併為 `chat`。原因是 registry 的 chat factory
已回傳 paired `Adapter`；streaming interface 仍保留，但不再是獨立 discovery
capability。

Benchmark capability 是 provider capability 的明確子集：`chat`、`image`、
`video`、`music`、`transcribe`、`speech`。只有 provider 與 model 都宣告支援，且
benchmark 有可執行 case 時，才進入自動選擇或 package generation。

### 1.2 Directional Modality

`Modality` 擴充為 `text`、`image`、`audio`、`video`，並改成 directional metadata：

- `InputModalities` 表示 model 能讀取的 data form。
- `OutputModalities` 表示 model 能產生的 data form。

`speech`、`music` 不新增為 modality：兩者是 operation capability，其 output 都是
`audio`。Capability 與 modality 不互相替代。

### 1.3 Catalog Ownership

Catalog vocabulary 全部由 `provider` 擁有：

- `provider.ModelSpec`
- `provider.Modality`
- `provider.ModelLister`

`core` 只保留 runtime request/result ports，不再持有 discovery metadata。
`provider.Entry.Catalog`、static catalog、live catalog、wizard 與 provider CLI
統一使用 `provider.ModelSpec`。

`ModelSpec` 的 catalog metadata 範圍為：identity、family、reasoning、model
capabilities、input/output modalities、context window 與 output token limit。
同一 model 可宣告多個 capability。

### 1.4 Selection Invariant

動態選擇的共同 gate 為：

```text
Entry supports capability
  ∩ ModelSpec supports capability
  ∩ consumer has a matching operation path
  ∩ request satisfies that operation's input requirements
```

`Entry` capability 仍由 factory availability 推導，避免第二份 provider-level
metadata。`ModelSpec.Capabilities` 則是 model-specific metadata，兩者不能合併。

需要 dynamic selection 的 orchestration/service layer 可以 import `provider`，用
`Capability` 與 catalog 表達需求；已選定 operation 後的 business component 仍只依賴
對應 typed interface。這次只建立可供 routing 使用的 metadata contract，不建立
router 本身。

## 2. In Scope

- 統一 capability 名稱與 values，移除 obsolete capability vocabulary。
- 將 catalog types 與 live catalog contract 從 `core` 移至 `provider`。
- 為所有 bundled model catalog 補上 model capabilities 與 directional modalities。
- 更新 static/live catalog merge，保留已知 model metadata；未知 live model 不用名稱
  猜測 capability。
- 讓 agent wizard 只列出能執行 `chat` 的 model。
- 讓 provider CLI 顯示 provider-level capability 與 model-level metadata。
- 讓 benchmark case、selection、result metadata 與 generator 直接使用
  `provider.Capability`。
- 移除 `benchmark.Kind`、`KIND_*`、`KindsOf` 與 provider/model-name capability
  inference。
- Benchmark 自己保留「case 是否足以驅動 model」的 applicability policy，例如缺少
  subject/reference audio 的特殊 model 不進 generic case。
- 重新產生 generator-owned `benchmark/pkg/*/main.go`。
- 同步 canonical docs、CLI reference、terminology 與 changelog。

## 3. Out of Scope

- 不新增 generic `Generate(any)`、untyped request/result 或單一 universal client。
- 不建立 service router、model load balancer、client cache、fallback policy 或 runtime
  hot-swap manager。
- 不改變既有 chat/image/video/music/speech/transcribe/live/translate typed contracts。
- 不改 provider endpoint、authentication、wire payload 或 default model policy。
- 不用 model ID naming convention 推導 capability。
- 不為舊 capability constants、`benchmark.Kind`、`-kinds` flag 或 result JSON schema
  建 compatibility alias、fallback 或 migration。
- 不執行 paid/live provider calls；驗證只使用 local tests、fixtures 與 generated output。
- 不修改 benchmark session 下既有 `tmp/` result history。

## 4. Dependency Boundary

### Current

```text
core.ModelSpec / Modality / ModelLister
        ↓
provider.Entry.Catalog + adapter catalogs
        ↓
benchmark.Kind ↔ capability mapping
        ↓
KindsOf(provider name, model ID heuristic)
```

### Target

```text
provider.Capability
        ├── Entry factories → provider-level support
        ├── ModelSpec → model-level support + input/output modalities
        ├── wizard / provider CLI → discovery and filtering
        └── benchmark → predefined capability subset + applicability policy

core → runtime model request/result only
```

`provider` 可以 import `core` 以實作 runtime interfaces；`core` 不反向 import
`provider`。Catalog metadata 因此不再污染 runtime domain。

## 5. Change Map

### Provider Catalog Domain

**Create**

- `provider/model.go`
- `provider/model_test.go`

**Modify**

- `provider/capability.go`
- `provider/capability_test.go`
- `provider/registry.go`
- `provider/registry_test.go`
- `provider/decorator.go`
- `provider/utils/utils.go`
- `provider/utils/utils_test.go`

**Remove obsolete ownership from**

- `core/model.go`
- `core/provider.go`

### Adapter Catalogs

下列 catalog 全部改用同一個 provider-owned schema，並以 upstream contract 明確
標記 capability 與 directional modality：

- `provider/anthropic/models.go`
- `provider/antigravity/models.go`
- `provider/codex/models.go`
- `provider/elevenlabs/models.go`
- `provider/google/models.go`
- `provider/grok/models.go`
- `provider/minimax/models.go`
- `provider/ollama/models.go`

同步調整各 adapter 現有 model/catalog tests，特別涵蓋 live merge、multi-capability
model、catalog-only model，以及 SDK 尚無 factory 的 upstream model。

### Consumers

- `cmd/agent/wizard/notes.go`
- `cmd/agent/wizard/notes_test.go`
- `cmd/agent/wizard/providers_test.go`
- `cmd/provider/catalog.go`
- `cmd/provider/matrix.go`
- `cmd/provider/provider_test.go`
- `cmd/provider.go`

### Benchmark

**Modify**

- `benchmark/case.go`
- `benchmark/benchmark.go`
- `benchmark/benchmark_test.go`
- `benchmark/store.go`
- `benchmark/cmd/main.go`
- `benchmark/gen/main.go`

**Replace obsolete responsibility**

- Delete `benchmark/catalog.go` 的 model-name capability inference。
- Create `benchmark/applicability.go`，只擁有 benchmark case applicability。
- Add focused tests under `benchmark/cmd/` 與 `benchmark/gen/`。
- Regenerate all generator-owned `benchmark/pkg/*/main.go` through
  `go generate ./benchmark`；不手動編輯 generated files。

### Documentation

- `README.md`：只在 public feature summary 需要時更新。
- `CLAUDE.md`：更新 ownership、dependency boundary 與 benchmark invariant。
- `docs/providers.md`：更新 capability/catalog truth table。
- `docs/cli.md`：將 `-kinds` 改為 `-capabilities`，更新 list/sweep 行為。
- `docs/terminology.md`：定義 Capability、Modality、ModelSpec 與 applicability。
- `benchmark/README.md`：更新使用方式與 result schema。
- `docs/CHANGELOG.md`：implementation 全部完成後記錄 breaking change。

## 6. Implementation Sequence

### Task 0: Preflight 與 Baseline

- [x] 確認仍在 `master`，不建立 branch 或 worktree。
- [x] 盤點 staged、unstaged、untracked changes 與 active writer。
- [x] 保留目前 `provider/antigravity/models.go`、
  `provider/antigravity/provider_test.go`、`provider/codex/models.go`、
  `provider/grok/models.go` 的既有變更；未穩定前不修改 overlap files。
- [x] 記錄 targeted baseline 與 full workspace baseline，將 pre-existing failure 與本次
  regression 分開。

驗證：

```bash
git status --short
git diff --check
go test ./provider/... ./benchmark/... ./cmd/... -count=1
```

### Task 1: Canonical Capability Vocabulary

- [x] 先以 tests 鎖定 capability values、stable order 與 `Entry.Supports` 行為。
- [x] 將 provider capability 收斂為本計畫的九個 canonical values。
- [x] 將 paired chat adapter 視為單一 `chat` capability；保留 streaming runtime
  interface，但移除獨立 discovery 名稱。
- [x] 更新所有 capability consumer 與 typed unsupported error expectation。
- [x] 確認 repository 不再引用 obsolete values。

驗證：

```bash
go test ./provider -run 'Capability|Unsupported' -count=1
rg -n 'model_generate|model_stream|model_catalog|image_generate|video_generate|music_generate|audio_transcribe|audio_speech' core provider benchmark cmd README.md CLAUDE.md docs/providers.md docs/cli.md docs/terminology.md benchmark/README.md
```

第二個 command 預期為零 current-code / canonical-doc match；historical plans/specs
不列入此 gate。

### Task 2: Provider-owned Model Catalog

- [x] 先以 contract tests 鎖定 `ModelSpec`、directional modality、capability lookup、
  JSON round-trip 與 catalog copy semantics。
- [x] 把 `ModelSpec`、`Modality`、`ModelLister` 移至 `provider`。
- [x] 移除 `core` 的 catalog/discovery ownership，不留下 alias。
- [x] 更新 registry、decorator、live listing、catalog merge、wizard 與 provider CLI 的
  dependency direction。
- [x] 確認 `core` 仍不依賴 provider 或 catalog metadata。

驗證：

```bash
go test ./core ./provider ./provider/utils -count=1
rg -n 'core\.(ModelSpec|Modality|ModelLister)' --glob '*.go'
```

第二個 command 預期為零 current-code match。

### Task 3: Catalog Annotation 與 Invariants

- [x] 逐一分類所有 bundled model：可執行 capability、accepted input 與 produced
  output。
- [x] 允許同一 model 擁有多個 capability；禁止 model 宣告 provider 沒有 factory 的
  executable capability。
- [x] `catalog` 只屬於 provider entry，不放入 individual model capability。
- [x] 對 SDK 尚無 typed surface 的 upstream model 保留 catalog identity，但不虛構
  executable capability。
- [x] 將既有 typed capability 使用、但尚未列入 catalog 的 default live/translate
  model 納入 catalog，避免 provider 宣告 capability 卻沒有可選 model。
- [x] Live catalog 對已知 ID 保留 static metadata；未知 ID 只保留 upstream 真正提供
  的 metadata，不使用 ID heuristic 猜測。
- [x] 加入跨 provider catalog invariant tests：ID 非空且唯一、capability 是 entry
  support 的子集、directional modalities 使用 canonical vocabulary。

驗證：

```bash
go test ./provider/... -count=1
```

### Task 4: Discovery Consumers

- [x] Agent wizard 的 model picker 只顯示 `chat` model，避免 image/video/audio model
  被當成 agent runtime model。
- [x] Provider CLI list/catalog 顯示 provider capability、model capability、input/output
  modalities，讓 consumer 能選擇 `Provider + Capability + Model`。
- [x] Matrix 與 unsupported reporting 全部使用 canonical capability names。
- [x] 保持各 operation 的 typed construction 與 request/result path 不變。

驗證：

```bash
go test ./cmd/agent/wizard ./cmd/provider ./cmd -count=1
```

### Task 5: Benchmark Consolidation

- [x] 先加入 tests，鎖定 capability selection、explicit unsupported request、catalog
  sweep、applicability exclusion、result JSON 與 generator determinism。
- [x] `Case`、timeouts、dispatch 與 persisted `Record` 直接使用
  `provider.Capability`。
- [x] 將 persisted key `kind` 改為 `capability`；不讀取或轉換舊 session schema。
- [x] 將 CLI `-kinds` 改為 `-capabilities`；不保留 alias。
- [x] 移除 Kind-to-Capability mapping 與 `KindsOf`。
- [x] Catalog sweep 與 generator 使用
  `Entry.Supports ∩ ModelSpec.Capabilities ∩ benchmark case set ∩ applicability`。
- [x] 特殊 model 的 generic-case exclusion 留在 benchmark domain，不回寫成錯誤的
  provider capability metadata。
- [x] 透過 generator 更新 runnable packages；離開 runnable set 的 generated
  `main.go` 依既有 marker policy 清除，歷史 `tmp/` 不動。
- [x] 連續執行 generator 兩次，第二次不得產生額外 diff。

驗證：

```bash
go test ./benchmark/... -count=1
go generate ./benchmark
git diff --check
go generate ./benchmark
git diff --check
```

### Task 6: Canonical Documentation

- [x] 更新 user-facing CLI、benchmark usage 與 result schema。
- [x] 更新 technical ownership 與 selection invariant。
- [x] 更新 terminology，讓 capability、modality、provider support、model support、
  applicability 不再混用。
- [x] 只在 implementation 與 verification 完成後寫入 changelog。
- [x] 搜尋 current docs 中的 obsolete vocabulary；historical specs 保持歷史原貌。

驗證：

```bash
rg -n '\-kinds|KindsOf|benchmark\.Kind|KIND_|model_generate|model_stream' README.md CLAUDE.md docs benchmark/README.md
git diff --check
```

### Task 7: Workspace Verification

- [x] 執行 targeted test suites，確認 provider、catalog consumers、benchmark 與
  generator 各自通過。
- [x] 執行 full root-module tests、vet 與 workspace verification。
- [x] 檢查 generated diff、JSON schema breaking change 與 docs 一致性。
- [x] 再次確認未覆蓋 pre-existing user changes、未修改 benchmark `tmp/`、未觸發
  live API。

驗證：

```bash
go test ./provider/... -count=1
go test ./benchmark/... ./cmd/... ./agent/... -count=1
go test ./... -count=1
go vet ./...
bash scripts/verify-workspace.sh
git diff --check
git status --short
```

## 7. Acceptance Criteria

- Repository current code 只有一個 capability type：`provider.Capability`。
- `benchmark.Kind`、`KIND_*`、Kind-to-Capability mapping 與 `KindsOf` 已移除。
- Provider/model/benchmark 三層關係可表達：provider 有 capability、特定 model 有
  capability、benchmark 只測其中可驅動的子集。
- `ModelSpec` 可以區分讀取能力與產生能力，且 catalog vocabulary 不再位於 `core`。
- 同一 provider 的 chat/image/video model 能被 metadata 正確區分與篩選。
- Agent wizard 不顯示不能作為 chat runtime model 的 catalog entry。
- Benchmark CLI、generated packages 與 result JSON 全部使用 canonical capability
  vocabulary。
- Generated output deterministic，existing benchmark result history 保留。
- 所有 local verification 通過；live/paid acceptance 明確未執行。

## 8. Execution Boundary

本 plan 核准後才開始 implementation。執行時直接在 `master` 進行，不建立 branch
或 worktree；但必須先確認目前 catalog edits 已穩定，避免覆蓋其他工作。每完成一個
task 先通過 targeted verification，再進下一層。除非另行要求，不 commit、不 push。
