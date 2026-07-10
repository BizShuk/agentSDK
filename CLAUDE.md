# CLAUDE.md — agentsdk 技術脈絡 (Technical Context)

`agentsdk` 是 `playground/agentsdk` 子專案,Go Agentic Loop SDK + Log Doctor sample。目標導向控制迴圈 SDK,純 Go (1.26),以 `go.work` 多模組管理依賴。

## 專案結構 (Project Structure)

```text
agentsdk/
├── README.md                        # 業務範疇總覽 (本專案)
├── CLAUDE.md                        # 技術脈絡 (本檔)
├── go.work                          # 多模組: root + sample/logdoctor
├── go.mod                           # module github.com/bizshuk/agentsdk
├── core/                            # 純狀態機 (stdlib only, 連 gosdk 都不 import)
│   ├── state.go                     # State, RunStatus, Budget, AutonomyLevel L0-L4
│   ├── input.go                     # Input, InputKind, Percept, ModelResult, ToolResult
│   ├── effect.go                    # Effect (tagged union, 7 kinds)
│   ├── message.go                   # Message, Role, Chunk (multimodal text/audio/image/tool)
│   ├── thinking.go                  # ThinkingPattern 介面 + 6 ThinkingKind 常數
│   ├── tool.go                      # ToolSchema, RiskLevel
│   ├── autonomy.go                  # ApprovalPolicy 介面
│   ├── approval.go                  # PendingApproval, ApprovalDecision
│   ├── port.go                      # ModelProvider / StateStore / WAL / ToolRegistry / Notifier
│   └── step.go                      # Step func type, NewStep(patterns) 唯一 dispatch 點
├── perception/                      # 支柱 1 (M1 實作)
│   ├── source.go                    # Source 介面 + Multi fan-in (sync.WaitGroup close)
│   └── normalize.go                 # Normalizer (Percept → Message)
├── planning/                        # 6 ThinkingPattern 實作
│   ├── think_then_act.go            # ✅ ThinkThenAct (think → act → observe FSM via scratch)
│   ├── plan_then_run.go             # ✅ PlanThenRun (blueprint + step dispatch)
│   ├── do_then_review.go            # ✅ RunThenReview (execute + critique iterate)
│   ├── one_shot.go                  # 🟡 STUB OneShotReasoning (emit CALL_MODEL+DONE)
│   ├── learn_from_failure.go        # 🟡 STUB LearnFromFailure
│   ├── choose_agent.go              # 🟡 STUB ChooseAgent
│   └── helpers.go                   # scratch helpers + newID
├── memory/                          # 支柱 2 (M2 完整實作)
│   ├── window.go                    # Window (MaxMessages / MaxTokens) + CharHeuristicCounter
│   ├── compactor.go                 # Compactor 介面 + HeadlineCompactor (no-LLM fallback)
│   ├── checkpoint/checkpointer.go   # Checkpoint() / Recover() — 與 Store+WAL 配對
│   └── filestore/                   # FileStateStore (atomic write-temp+rename) + FileWAL (JSONL append)
├── middleware/                      # M2 鏈 (tracing/sandbox/approval/sanitizer 留 M3/M4)
│   ├── middleware.go                # Middleware / Next / Chain
│   ├── harness/retry.go             # Retry (N 次 + 指數 backoff,認 RetryableError interface)
│   ├── harness/budget.go            # Budget guard (state.Budget.Exceeded → BudgetExceededError)
│   ├── harness/timeout.go           # Timeout (per-effect WithTimeout)
│   └── loopguard/loopguard.go       # 指紋 (sha1+volatile strip) + 連續 CALL_TOOL → REQUEST_APPROVAL
├── config/                          # 一站式 CLI wiring (AppConfig)
│   └── app.go                       # OpenForCLI: gosdk/config init + mkdir + slog + filestore Store/WAL
├── action/                          # 支柱 3 (M1 minimal)
│   ├── tool.go                      # TypedTool[TArgs,TOut] 泛型 (json.RawMessage)
│   └── registry.go                  # Registry (記憶體靜態註冊, M3 加 ToolSource 動態)
├── runtime/                         # Shell
│   └── loop.go                      # Engine: dispatch + preStep scratch seed + short-circuit on end_turn
├── internal/testutil/               # 測試 only (FakeProvider / MemStore / MemWAL / CapturingNotifier)
└── sample/logdoctor/                # 驗證 sample (獨立 go.mod)
    ├── main.go                      # cobra entry
    ├── cmd/root.go                  # NewRoot() — 不呼叫 Execute()
    ├── cmd/run.go                   # RegisterRun(root) — --once --fixture --fake
    ├── core/listener.go             # domain layer (gosdk noun 慣例)
    ├── tool/read_log_tail.go        # TypedTool 包裝 listener
    ├── tool/notify.go               # TypedTool 包裝 io.Writer
    ├── internal/fake/fake.go        # 腳本化 ScriptedProvider (read_log_tail → notify → end_turn)
    └── testdata/error.log           # fixture
```

## 技術棧 (Tech Stack)

| 類別 | 技術 | 備註 |
|------|------|------|
| 語言 | Go | 1.26 |
| 多模組 | go.work | root + sample/logdoctor |
| 測試 | testify | v1.11.1, table-driven + t.Run |
| CLI | spf13/cobra | sample/logdoctor |
| Provider | (M4) | anthropic-sdk-go / openaicompat / google.golang.org/genai |

## 關鍵決策 (Key Decisions)

- **`core/` 純 stdlib**:連 gosdk 都不 import,讓 SDK 可獨立發佈;gosdk wiring 只發生在 sample 組合根 (M2 後)
- **`core.Step` 是純函式**:patterns 只看 scratch + state.Messages,不能 inspect Input;runtime 在呼叫 Step 前 pre-populate scratch (例如 `react.last_call_id`)
- **End-turn short-circuit**:runtime 收到 `ModelResult.StopReason=end_turn` (無 tool_calls) 時直接 COMPLETED,跳過 Step — 避免 ReAct 等 pattern 在 act phase 對 stale scratch 發出 CALL_TOOL
- **Tagged union Effect**:Go 沒有 sum type,用 `Kind` discriminator + 7 個 optional pointer 表達
- **Notifier 結構性相容**:`core.Notifier` 介面與 `gosdk/notify.Notifier` 方法集相同,結構性滿足,無需 adapter
- **`sample/logdoctor/core` 撞名**:不同 module path 編譯安全,import 時以 `sdkcore` / `domain` 別名區分
- **Middleware 鏈組合 (M2)**:retry → timeout → budget → loopguard → base dispatch。state 在每一層都會被 mutate,但因為 Go map 是 reference type,scratch 變更會自動傳遞給下一層與下個 iteration。loopguard state 透過 scratch[loopguard.state] 持久化。
- **scratch 是 pattern 與 middleware 的通訊介面**:runtime 在 Step 前 preStep 寫入 (例如 `react.last_call_id`),pattern 透過 Decide 讀 scratch 決定 effect,middleware 把 bookkeeping 寫回 scratch 跨迭代累積。
- **WAL Replay 語意**:`Replay(runID, sinceSeq)` 回傳所有 `input.Seq > sinceSeq` 的 Inputs(State.LastInputSeq 是「已被跑過的最大 Seq」)。Caller 不重發模型呼叫,因為 WAL 已經包含原來的 ModelResult / ToolResult。

## 模組對應 (Module Mapping)

| 業務領域 | 套件 | 進入點 |
|---------|------|--------|
| 核心狀態機 | `agentsdk/core` | `core.Step`, `core.NewStep` |
| 感知 | `agentsdk/perception` | `perception.Source`, `perception.Multi` |
| 規劃 | `agentsdk/planning` | `planning.NewReAct` 等 6 個 constructor |
| 行動 | `agentsdk/action` | `action.NewRegistry`, `action.NewTypedTool` |
| 配置 | `agentsdk/config` | `config.OpenForCLI`, `config.MustOpenForCLI` → `AppConfig` |
| Shell | `agentsdk/runtime` | `runtime.NewEngine`, `Engine.Run` / `Engine.Resume` |
| Sample | `agentsdk/sample/logdoctor` | `cmd.RegisterRun` → `cobra.Command.Execute` |
| Test fixtures | `agentsdk/internal/testutil` | (production code MUST NOT import) |

## 開發指南 (Development Guide)

### 前置需求

- Go 1.26+
- `bizshuk/gosdk@v1.0.2` 在 module cache (M2 開始用)

### 安裝

```bash
cd agentsdk
go work sync
go mod download
```

### 建置

```bash
# root SDK + sample
go build ./...
```

### 測試

```bash
# 全套 (root SDK)
go test ./... -count=1 -timeout=30s

# sample 模組
cd sample/logdoctor
go test ./... -count=1 -timeout=30s
```

### E2E (M1)

```bash
cd sample/logdoctor
go run . --fake --max-turns=10 run --once --fixture testdata/error.log
```

預期 JSONL 序列:`call_model → call_tool(read_log_tail) → call_model → call_tool(notify) → call_model → done`

## Milestone 進度 (Roadmap)

| ID | 範疇 | 狀態 | 驗收 |
|----|------|------|------|
| M1 | 核心範式 + sample 骨架 | ✅ 完成 | `go test ./...` 全綠 + E2E JSONL 符合預期 |
| M2 | 系統韌性 + 循環防禦 | ✅ 完成 | Budget 到頂即停 / Retry N 次後 surface / FileStateStore round-trip / Checkpointer.Recover JSON 與原 State 等價,Recover 期間 FakeProvider.CallCount 不變 (不重呼叫 LLM) / loopguard 第 5 次連續 CALL_TOOL 觸發 REQUEST_APPROVAL{Reason:"loop_detected"} / sample logdoctor run + resume CLI 驗證 |
| M3 | 工具生態 + 執行期安全 | ⏳ | invopop/jsonschema 反射 / sandbox allow-deny / spotlight + sanitizer / MCP discover / **perception/ 去留決策 (見 plans/2026-07-07-m3-tooling-security.md carry-over)** |
| M4 | 架構解耦 + HITL + 三 provider | ⏳ | 含 mid-run approval State round-trip / 三 provider 實測 / 完整 watch→fix story |

詳細規格見 `plans/plan-only-and-plan-breezy-pike.md`;可執行 todo 見 `README.todo`(索引) → `plans/2026-07-07-m3-tooling-security.md` 與 `plans/2026-07-07-m4-hitl-providers.md`。

## 慣例 (Conventions)

- **命名 (Naming)**
  - 常數一律 `SCREAMING_SNAKE_CASE` (含 unexported、block-scoped),與 gosdk 一致
  - `var` / `func` / `type` 仍用 `MixedCaps`
  - Package 名為單字 (`core`, `action`, `planning`)
- **錯誤處理 (Error Handling)**
  - `error` 回傳為主;`ToolResult.Error` 字串欄位是 tool 內部錯誤的載體 (與 panic 解耦)
  - runtime loop 用 `fmt.Errorf("...: %w", err)` wrap
- **測試 (Testing)**
  - table-driven + `t.Run`
  - `testify/assert` 與 `testify/require` 並用 (assert 為非致命檢查,require 為致命)
  - testutil 套件為內部,production code 不得 import
- **文件 (Docs)**
  - Package docstring 在每個目錄的主要檔案
  - 中文註解 + 英文關鍵字,遵循 `playground/CLAUDE.md` 全域慣例

## 注意事項 (Caveats)

- **`perception.Multi` close 行為**:用 `sync.WaitGroup` 等所有 source goroutine 完成才 close merged channel (M1 修正重點 — 不能讓子 goroutine 提早關閉導致 race)
- **runtime preStep scratch seed**:`react.last_call_id` 等 scratch key 由 runtime 寫入,在 Step 呼叫前完成,讓純函式 pattern 能讀到
- **`Sample/logdoctor/core` 與 `agentsdk/core` 撞名**:sample 端 import 必須用 `sdkcore` / `domain` 別名
- **`go.work.sum` 已 commit**:workspace lock 檔案進入版控,讓 CI 可離線重建
- **M2 將引入 gosdk**:`config` (viper) / `log` (slog) / `notify` (Multi/Stdout/Slack) / `metric` (mimir),wiring 點都在 sample 組合根,SDK 核心不變