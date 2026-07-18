# agentsdk

Go Agentic Loop SDK、LLM protocol proxy 與 Log Doctor sample，提供目標導向控制迴圈 (Goal-directed Control Loop) 及跨提供者協定橋接。

## 範疇 (Scope)

四大支柱對應到頂層 package,架構即文件:

| 支柱        | 套件                       | 角色                                                                                          |
| ----------- | -------------------------- | --------------------------------------------------------------------------------------------- |
| 1. 認知架構 | `core` (ObservationSource) | 觀察來源 port (Percepts channel);原 `perception/` 套件無 consumer 已移除                      |
| 2. 系統韌性 | `memory/`                  | Window / Compactor / Checkpoint (M2)                                                          |
| 3. 工具生態 | `action/`                  | TypedTool / Registry / Sandbox / ApprovalPolicy                                               |
| 4. 規劃     | `planning/`                | 6 種 ThinkingPattern (ReAct / Planner-Executor / Executor-Critic / CoT / Reflexion / Router)  |

`core/` 是純狀態機 (state + event + instruction + step),只依賴 stdlib,連 gosdk 都不 import。root module 的 `runtime/loop.go` 是 shell,負責 dispatch instructions 到綁定的 port (model / tools / store / notifier)。

## 模組結構

```tree
agentsdk/
├── go.work                    # 多模組: root + auth submodule + proxy + mcp + provider/* + sample/* + utils/video
├── go.mod                     # module github.com/bizshuk/agentsdk
├── cmd/proxy.go               # proxy server CLI composition root
├── core/                      # 純狀態機 (stdlib only, 含 ObservationSource port)
├── memory/                    # 支柱 2 (M2)
├── planning/                  # 6 thinking patterns
├── action/                    # TypedTool + Registry
├── middleware/                # (M2 鏈)
├── runtime/                   # Loop: dispatch + checkpoint + WAL
├── cli/                       # JSONL envelope/codec
├── app/                       # CLI agent lifecycle/composition root
├── config/                    # AppConfig、middleware presets、RefreshingProvider
├── tool/                      # 6 個內建工具
├── auth/                      # Git submodule + 獨立 module：credential mechanism + provider registry + Resolver
├── proxy/                     # 獨立 module：LLM protocol bridge + provider routing/upstream
│   ├── protocol/              # Anthropic Messages / OpenAI Chat / Responses DTO + SSE
│   ├── transform/             # 明確的 3×3 pairwise request/response/stream transforms
│   ├── route/                 # qualified model → provider family
│   ├── upstream/              # concrete profiles、credentials、safe HTTP client
│   ├── config.go              # proxy 設定 (gosdk layered viper, APP_NAME=agentSDK)
│   ├── handler.go             # bounded generic request pipeline
│   └── server.go              # Gin route composition
├── mcp/                       # 獨立 module：MCP ToolSource adapter
├── provider/                  # 獨立 module：anthropic/google/openaicompat adapters
├── utils/video/               # 獨立 module：audio/frames/subtitles/ffmpeg preprocessing
├── internal/testutil/         # FakeProvider / MemStore / CapturingNotifier
└── sample/logdoctor/          # 驗證 sample (cobra CLI + 兩個 tool)
```

`auth` 是 production Git submodule（`https://github.com/BizShuk/auth.git`），`utils/video`、`auth`、`proxy` 都是獨立 Go module，且各自攜帶自己的 CLI surface：`auth/cmd.Install` 掛載憑證指令集（login/list/verify/refresh/logout/use）、`proxy/cmd.NewCommand()` 提供 `proxy` 指令、`utils/video/cmd.NewCommand()` 提供 `video` 指令。root `cmd/` 只是聚合殼，把三者組合成 `auth-cli`；任何 cobra root 都能單獨掛載其中一組。`auth` 與 `proxy` module root 另有 `main.go`（auth 函式庫位於 `auth/model`、`auth/svc`、`auth/utils`、`auth/provider`，proxy 函式庫位於 `proxy/handlers`、`proxy/config`、`proxy/model`、`proxy/svc`），可各自 build 出獨立同名 binary，與 `auth-cli` 共用設定與憑證目錄。`auth` 依賴 viper/cobra + stdlib；`proxy` 依賴 `auth` 與 gin/gosdk；SDK 核心群（core/runtime/...）不依賴兩者。依賴方向固定為 `binary(root cmd) → proxy → auth`。

## Proxy protocol bridge

Proxy 將 agent 使用的 wire protocol 與 LLM provider 的 concrete API profile 分開處理：

```text
client route → directed pair transform → provider normalization → upstream
                                                       ↓
client response ← reverse directed pair transform ← provider response
```

- 支援 `Anthropic Messages`、`OpenAI Chat Completions`、`OpenAI Responses` 的完整 `3×3` request、non-stream response 與 SSE stream matrix。
- concrete profiles 包含 `anthropic`、`minimax`、`openai-api`、`openai-codex-oauth`、`xai`。
- xAI 預設走 `OpenAI Responses`；qualified model `xai-chat/<model>` 可明確選擇 `OpenAI Chat Completions`。
- provider selection 由 qualified model、credential kind 與 profile capability 決定，不以 client protocol 綁定 provider。
- 四個參考來源的 `37` 個 directed wire-format entity 與雙向 payload 範例見 [format catalog](docs/specs/format/README.md)。

## 設計原則

- **核心純粹**:`core/` 零 vendor 依賴,可獨立發佈;所有 I/O 都在 `runtime/` 與 ports adapter
- **六種 ThinkingPattern**:透過 `core.NewDecide` 與純函式 DecisionRule dispatch;working memory 作為 pattern 與 runtime 間的通訊介面
- **Tagged union Instruction**:7 種 instruction kind 透過 Kind discriminator + optional pointer 欄位表達,JSON round-trip 透過 `omitempty` 精簡
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
| M5        | built-in tools、sample wiring、`app` lifecycle                                | ✅ 完成 |
| M6        | auth mechanism、9 provider ids、auth CLI                                      | ✅ 完成 |
| Proxy     | 3×3 pairwise protocol transform、provider profile routing、SSE hardening        | ✅ 完成 |
| Format    | 四來源 `37` 個 client/provider wire-format entity catalog                       | ✅ 完成 |

詳細規格見 `docs/specs/` 與 [`docs/specs/format/README.md`](docs/specs/format/README.md),各 milestone 實作完成後會轉為 `docs/specs/YYYY-MM-DD-<feature>.md`:

- `2026-07-04-core-paradigm-and-sample-skeleton.md` (M1)
- `2026-07-04-system-resilience-and-loop-defense.md` (M2)
- `2026-07-04-tool-ecosystem-and-runtime-security.md` (M3)
- `2026-07-04-architecture-decoupling-hitl-and-providers.md` (M4)

## 慣例衝突 (Naming Collision)

`agentsdk/core` (純狀態機) 與 `sample/logdoctor/core` (gosdk noun 層領域邏輯) 撞名,屬不同 module path (`github.com/bizshuk/agentsdk/core` vs `github.com/bizshuk/agentsdk/sample/logdoctor/core`),編譯安全。Sample 端 import 時以 `sdkcore` / `domain` 別名區分。

## 慣例

- 常數一律 `SCREAMING_SNAKE_CASE` (含 unexported、block-scoped),與 gosdk 一致
- `go.work` 多模組:每子模組各自 `go.mod`;`core/` 維持 stdlib-only,root config/app 與獨立 module (auth/proxy) 可使用應用層依賴
- 測試:table-driven + `t.Run` + `testify`
- 中文註解 + 英文關鍵字,遵循 `playground/CLAUDE.md` 慣例
