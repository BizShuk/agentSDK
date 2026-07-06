# agentsdk

Go Agentic Loop SDK + Log Doctor sample,目標導向控制迴圈 (Goal-directed Control Loop) 的多模組 SDK。

## 範疇 (Scope)

四大支柱對應到頂層 package,架構即文件:

| 支柱        | 套件          | 角色                                                                                         |
| ----------- | ------------- | -------------------------------------------------------------------------------------------- |
| 1. 認知架構 | `perception/` | Source 介面 (Percepts channel) + NormalizeFunc                                               |
| 2. 系統韌性 | `memory/`     | Window / Compactor / Checkpoint (M2)                                                         |
| 3. 工具生態 | `action/`     | TypedTool / Registry / Sandbox / ApprovalPolicy                                              |
| 4. 規劃     | `planning/`   | 6 種 ThinkingPattern (ReAct / Planner-Executor / Executor-Critic / CoT / Reflexion / Router) |

`core/` 是純狀態機 (state + input + effect + step),只依賴 stdlib,連 gosdk 都不 import。`runtime/loop.go` 是 shell,負責 dispatch effects 到綁定的 port (model / tools / store / notifier)。

## 模組結構

```tree
agentsdk/
├── go.work                    # 多模組: root + sample/logdoctor
├── go.mod                     # module github.com/bizshuk/agentsdk
├── core/                      # 純狀態機 (stdlib only)
├── perception/                # 支柱 1
├── memory/                    # 支柱 2 (M2)
├── planning/                  # 6 thinking patterns
├── action/                    # TypedTool + Registry
├── middleware/                # (M2 鏈)
├── runtime/                   # Loop: dispatch + checkpoint + WAL
├── cli/                       # (M2 JSONL codec)
├── internal/testutil/         # FakeProvider / MemStore / CapturingNotifier
└── sample/logdoctor/          # 驗證 sample (cobra CLI + 兩個 tool)
```

## 設計原則

- **核心純粹**:`core/` 零 vendor 依賴,可獨立發佈;所有 I/O 都在 `runtime/` 與 ports adapter
- **六種 ThinkingPattern**:透過 `core.Step` 純函式 dispatch;scratch map 作為 pattern 與 runtime 間的純函式通訊介面
- **Tagged union Effect**:7 種 effect kind 透過 Kind discriminator + optional pointer 欄位表達,JSON round-trip 透過 `omitempty` 精簡
- **Notifier 結構性相容**: `core.Notifier` 介面方法集與 `gosdk/notify.Notifier` 完全相同,gosdk 的 Multi / Stdout / Slack 直接傳入,無需 adapter

## 執行範例 (M1 e2e)

```bash
cd sample/logdoctor
go run . --fake --max-turns=10 run --once --fixture testdata/error.log
```

JSONL 輸出:

```
effect call_model      ← 第一次思考
effect call_tool       ← read_log_tail n=5
effect call_model      ← 觀察結果
effect call_tool       ← notify
effect call_model      ← 最終回應
effect done            ← end_turn
```

## 開發狀態 (Milestones)

| Milestone | 範疇                                                                             | 狀態    |
| --------- | -------------------------------------------------------------------------------- | ------- |
| M1        | 核心範式 + sample 骨架 (無 provider / 無 middleware / 無 dedupe)                 | ✅ 完成 |
| M2        | 系統韌性 + 循環防禦 (memory / checkpoint / WAL / loopguard / retry)              | ✅ 完成 |
| M3        | 工具生態 + 執行期安全 (schema / sandbox / spotlight / sanitizer / MCP / tracing) | ✅ 完成 |
| M4        | 架構解耦 + HITL 完整 + 三個 LLM provider (anthropic / openaicompat / google)     | ✅ 完成 |

詳細規格見 `plans/plan-only-and-plan-breezy-pike.md`,各 milestone 實作完成後會轉為 `docs/specs/YYYY-MM-DD-<feature>.md`:

- `2026-07-04-core-paradigm-and-sample-skeleton.md` (M1)
- `2026-07-04-system-resilience-and-loop-defense.md` (M2)
- `2026-07-04-tool-ecosystem-and-runtime-security.md` (M3)
- `2026-07-04-architecture-decoupling-hitl-and-providers.md` (M4)

## 慣例衝突 (Naming Collision)

`agentsdk/core` (純狀態機) 與 `sample/logdoctor/core` (gosdk noun 層領域邏輯) 撞名,屬不同 module path (`github.com/bizshuk/agentsdk/core` vs `github.com/bizshuk/agentsdk/sample/logdoctor/core`),編譯安全。Sample 端 import 時以 `sdkcore` / `domain` 別名區分。

## 慣例

- 常數一律 `SCREAMING_SNAKE_CASE` (含 unexported、block-scoped),與 gosdk 一致
- `go.work` 多模組:每子模組各自 `go.mod`,root SDK 維持 stdlib-only
- 測試:table-driven + `t.Run` + `testify`
- 中文註解 + 英文關鍵字,遵循 `playground/CLAUDE.md` 慣例
