# CLAUDE.md — agentsdk 技術脈絡 (Technical Context)

`agentsdk` 是 `playground/agentsdk` 子專案,Go Agentic Loop SDK + Log Doctor sample。目標導向控制迴圈 SDK,純 Go (1.26),以 `go.work` 多模組管理依賴。

## 專案結構 (Project Structure)

```text
agentsdk/
├── README.md                        # 業務範疇總覽 (本專案)
├── CLAUDE.md                        # 技術脈絡 (本檔)
├── go.work                          # 多模組: root + sample/logdoctor
├── go.mod                           # module github.com/bizshuk/agentsdk
├── main.go                          # auth-cli binary entry (package main)
├── cmd/proxy.go                     # proxy server CLI composition root
├── cmd/auth/                        # auth-cli cobra 指令樹
│   ├── root.go                      # NewRoot() + --auth-dir / --no-browser + saveAndReport
│   ├── login.go                     # login --provider <id> — 唯一登入入口 (id 即認證路徑)
│   ├── list.go                      # list — 表格輸出,不印 secret
│   ├── verify.go                    # verify [name|--all] — 對真實 provider 驗證 + 輪替後存回
│   └── refresh.go                   # refresh <name> + logout <name>
├── auth/                            # 機制層 (mechanism,純 stdlib,不認識任何一家 provider)
│   ├── auth.go                      # Credential / Kind / Authenticator / VerifyResult
│   ├── options.go                   # Options (導出,provider 套件要讀) + Option
│   ├── oauth.go                     # OAuthClient: AuthCodeURL / Exchange / Refresh / PostToken
│   │                                #   (可選 PKCE + 可選 client_secret + single-flight + 429 退避)
│   ├── device.go                    # Device Authorization Grant (RFC 8628) + OIDC discovery
│   ├── login.go                     # RunBrowserLogin / RunDeviceLogin / VerifyByRefresh / MergeOAuthToken
│   ├── apikey.go                    # APIKeySpec + 泛用 APIKeyAuth (env chain + models 端點驗證)
│   ├── pkce.go callback.go          # RFC 7636 S256;本機 callback server
│   ├── browser.go jwt.go store.go   # 開瀏覽器;JWT claims (不驗簽);FileStore (0700/0600 + atomic)
│   ├── authtest/                    # 測試用假 provider 與 helper (給 auth 與各 provider 套件共用)
│   └── provider/                    # registry (package provider) — import 下列子包,無循環
│       ├── provider.go              #   ROUTES / New(id) / Login(ctx,id) / For(cred) / IDs()
│       ├── anthropic/               #   api key (x-api-key) + oauth2 PKCE (claude.ai)
│       ├── openai/                  #   api key (bearer) + oauth2 PKCE (auth.openai.com, id_token claims)
│       ├── google/                  #   api key (x-goog-api-key, Gemini)
│       ├── xai/                     #   api key (bearer) + device code RFC 8628 (OIDC discovery)
│       ├── antigravity/             #   oauth2 (accounts.google.com, client_secret 無 PKCE, userinfo)
│       └── vertex/                  #   service account: RS256 JWT assertion → Google STS
├── proxy/                           # LLM protocol proxy
│   ├── handler.go                   # bounded generic request/response/stream pipeline
│   ├── observability.go             # redacted transform warnings/loss metrics
│   ├── middleware.go                # API key、rate limit、CORS middleware
│   ├── server.go                    # Gin routes + runtime lifecycle
│   ├── protocol/                    # envelopes、native errors、SSE framing
│   │   ├── anthropic/               # Anthropic Messages typed DTO
│   │   ├── chat/                    # OpenAI Chat Completions typed DTO
│   │   └── responses/               # OpenAI Responses typed DTO
│   ├── transform/                   # explicit 3×3 pairwise transforms + collectors
│   ├── route/                       # qualified model → provider family/forced format
│   └── upstream/                    # profiles、credential resolver、safe HTTP client
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
│   ├── one_shot.go                  # ✅ OneShotReasoning (think→done 雙相 FSM)
│   ├── learn_from_failure.go        # ✅ LearnFromFailure (act→reflect→retry→done,critique OK: prefix + reflection 累積)
│   ├── choose_agent.go              # ✅ ChooseAgent (select→delegate→done,inline system-msg delegate)
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
- **`auth/` 分兩層**:`auth/` 只有機制 (Credential / OAuthClient / device flow / PKCE / callback / FileStore),不認識任何一家 provider;`auth/provider/<name>/` 一家一包,只 import `auth`;`auth/provider` 本身是 registry,import 所有子包。依賴單向,無循環。代價是 `auth.Options` 必須導出 (provider 套件住在別的套件,要讀得到它)。
- **`provider.ROUTES` 是唯一真相來源**:provider id → (provider, kind, 建構子)。`New` / `Login` / `For` / CLI 的旗標說明全部從它推導,新增一家 provider 只要加一列。測試 `TestEveryRouteHasAnAuthenticator` 確保表格不會與實作脫節。
- **provider id 即認證路徑**:`--provider anthropic` 是 API key,`--provider anthropic_oauth` 是瀏覽器流程 — 認證方式編碼在 id 裡,不另設 `--kind`。OAuth 的 id 帶 `_oauth` 後綴,與憑證檔名的後綴一致 (`anthropic-<email>_oauth.json`)。同一個 email 可以同時有 API key 與 OAuth 憑證,後綴讓兩者不互相覆蓋。
- **三種 OAuth 變體共用一個 `OAuthClient`**:差異用設定表達 — `UsePKCE` (public client 開,Google installed-app 關)、`ClientSecret` (只有 antigravity 有)、`Encoding` (Anthropic 收 JSON,其餘 form)、`SendState` (只有 Anthropic 開)。device flow (xAI) 另走 `RunDeviceLogin`,不開本機埠。
- **`SendState` 預設關**:token exchange 的 body 不帶 `state`。CSRF 比對在 callback 收回來的當下就做完了 (`RunBrowserLogin` 比對後才換 token),token 端點不需要它;OpenAI 更會對多出來的 state 直接回 `400 unknown_parameter`。只有 Anthropic 要求 exchange 帶 state。這條是真實 provider 打回來的教訓,別「順手統一」把它加回所有 provider。
- **Verify 是真的打網路,而且誠實回報做了什麼**:`models_endpoint` (api_key,無副作用) / `userinfo_endpoint` (antigravity,無副作用) / `token_refresh` (其餘 oauth,provider 可能輪替 token) / `sts_exchange` (vertex)。會輪替 token 的那幾條,`VerifyResult.Credential` 會帶回新憑證,**呼叫端必須存回磁碟** (OpenAI 會讓舊 refresh token 立刻失效)。
- **Login 內建驗證**:所有流程都在存檔前先對 provider 驗一次,不讓一把打不通的憑證安靜落地、把失敗延後到第一次推論才爆。
- **`Credential.BaseURL`**:對 gateway/proxy 發的 API key 會把 base URL 一起存進憑證,後續 `verify` 必須打回同一個端點,而不是 provider 官方端點。
- `Proxy registry 明確完整`: `Anthropic Messages`、`OpenAI Chat Completions`、`OpenAI Responses` 以九個 directed pair 明確註冊；每一組都有 request、non-stream response、stream factory，不建立單一 canonical IR。
- `Provider 與 protocol format 分離`: client format 只決定輸入/輸出 shape；qualified model、credential kind 與 concrete profile capability 才決定 provider endpoint 和 upstream format。unknown model 回 `unknown_model`，不得 fallback 到 Anthropic。
- `SSE 以完整 frame 轉換`: decoder 保留 event、id、retry 與多行 data；每個 request 建立獨立 stateful transformer，EOF 前未出現 terminal event 一律視為 `unexpected EOF` protocol failure。
- `xAI Responses 優先`: `xai/<model>` 預設 `/v1/responses`，只有 `xai-chat/<model>` 明確強制 `/v1/chat/completions`。

## 模組對應 (Module Mapping)

| 業務領域 | 套件 | 進入點 |
|---------|------|--------|
| 核心狀態機 | `agentsdk/core` | `core.Step`, `core.NewStep` |
| 感知 | `agentsdk/perception` | `perception.Source`, `perception.Multi` |
| 規劃 | `agentsdk/planning` | `planning.NewReAct` 等 6 個 constructor |
| 行動 | `agentsdk/action` | `action.NewRegistry`, `action.NewTypedTool` |
| 認證機制 | `agentsdk/auth` | `auth.OAuthClient`, `auth.RunBrowserLogin` / `RunDeviceLogin`, `auth.NewAPIKey(spec)`, `auth.NewFileStore` |
| 認證 provider | `agentsdk/auth/provider` | `provider.Login(ctx, id)`, `provider.New(id)`, `provider.For(cred)`, `provider.IDs()` |
| 認證 CLI | `agentsdk/cmd/auth` | `auth.NewRoot` → `main.go` (binary `auth-cli`) |
| Protocol proxy | `agentsdk/proxy` | `proxy.New` → `cmd/proxy.go`; `route → transform → upstream → reverse transform` |
| 配置 | `agentsdk/config` | `config.OpenForCLI`, `config.MustOpenForCLI` → `AppConfig`;`config.OpenAuthStore` |
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

### auth-cli (M6)

```bash
# 單一 login 入口: --provider 的 id 就是整條認證路徑 (不需要 --kind)
go run . login --provider anthropic                  # api key,讀 ANTHROPIC_API_KEY
go run . login --provider anthropic_oauth            # oauth2 + pkce,本機 :54545 收 callback
go run . login --provider openai_oauth --no-browser  # 無頭機器: 印 URL,貼回 code
go run . login --provider xai_oauth                  # device code (RFC 8628),顯示 user code + 輪詢
go run . login --provider antigravity_oauth          # google oauth (client secret,無 PKCE)
go run . login --provider google --key AIza...       # api key
go run . login --provider openai --api-base http://localhost:8317   # 對 gateway 發的金鑰
go run . login --provider vertex --sa-file sa.json --location us-central1

go run . list                                       # 表格輸出,不印 secret
go run . verify --all                               # 對真實 provider 驗證 (輪替後的憑證會存回)
go run . refresh <name>                             # api_key 會回 ErrRefreshUnsupported
go run . logout <name>
```

`ROUTES` (`auth/provider/provider.go`) 是 provider id 的唯一真相來源,目前 9 個 id:
`anthropic` / `anthropic_oauth` / `openai` / `openai_oauth` / `google` / `xai` / `xai_oauth` /
`antigravity_oauth` / `vertex`。

憑證預設落在 `~/.config/agentsdk/data/auth/<provider>-<account>.json`(0700 目錄 / 0600 檔),
可用 `--auth-dir` 覆寫。離線 e2e 的做法: 起一個假 provider,SA JSON 的 `token_uri` 指向它,
`--api-base` 指向它的 models 端點。

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
| M3 | 工具生態 + 執行期安全 | ✅ 完成 | `invopop/jsonschema` 反射 / sandbox allow-deny / spotlight + sanitizer / MCP discover / `runtime/m3_e2e_test.go` runtime 層 e2e 驗證 sanitizer 命中 + spotlight 包覆 + transcript 同步 / `perception/` 去留決策見下方「perception 去留」段 |
| M4 | 架構解耦 + HITL + 三 provider | ✅ 完成 | mid-run approval State round-trip / `config.SecureMiddleware` 含 approval + spotlight + sanitizer 完整鏈 / `consumeApprovedPendingCall` 讓 `SubmitHumanDecision` 與 `Resume` 真正消費 out-of-band decision / `runtime/m4_hitl_e2e_test.go` 跑通 approve + reject 故事 / 三 provider mock 測試齊(anthropic httptest / google 內部 fixture)/ `--provider=anthropic\|openaicompat\|google` flag 接通 sample |
| M5 | 內建工具 + sample wiring helper | ✅ 完成 | `tool/` 套件 6 工具(Read/Write/Edit/Bash/Glob/Grep)+ `RegisterDefaults` 一次註冊 / `action.Policy` sandbox re-check / Risk 對應 HITL(Write/Edit/Bash HIGH 觸發 ApprovalGate)/ `config.MustOpenForCLI` 樣板削減 / `sample/greet-agent` 接入 6 內建工具 + SecureMiddleware |
| M6 | Provider 認證 (`auth/`) | ✅ 完成 | 機制層 (`auth/`) + 6 個 provider 包 + registry,9 個 provider id / 4 種認證機制:api key、OAuth2 authorization-code (PKCE 或 client_secret)、device code (RFC 8628 + OIDC discovery)、service account (RS256 JWT → STS) / PKCE S256 + state CSRF 檢查 + 本機 callback server / FileStore 0700 目錄 + 0600 檔 + atomic write / refresh single-flight + 429 Retry-After 退避 / `verify` 對真實 provider 驗證並存回輪替後的憑證 / e2e: 假 provider(models 端點 + 驗簽的 Google STS)跑通 login → list → verify → refresh → logout;xAI device flow 對真實 `auth.x.ai` 跑通 discovery + device code |

詳細規格見 `docs/specs/`(M3 `2026-07-04-tool-ecosystem-and-runtime-security.md`、M4 `2026-07-04-architecture-decoupling-hitl-and-providers.md`、M5 `2026-07-07-m5-built-in-tools.md`、wiring helper `2026-07-07-appname-wiring-helper.md`)。歷史計畫剩 `plans/plan-only-and-plan-breezy-pike.md` 主綱與已被取代的 `docs/specs/` 對應;`plans/` 內 2026-07-07-m3 / 2026-07-07-m4 plan 已併入對應 spec 並刪除。

### perception 去留(來自 M3 carry-over)

**結論: 保留套件,延後決策**。`perception/` 套件(perception/source.go、normalize.go)目前無 consumer,屬無用 scaffolding。理想路徑是**選項 C: 刪除** 整個套件(`core.Observation` / `core.ObservationSource` shim 仍保留供 `testutil` 與 runtime 使用),但 M3 收尾階段未實際刪除,本檔據實記錄為「保留待後續評估」。若 sample 出現多 source fan-in 需求,優先在 sample 端 inline(`sample/logdoctor/core/listener.go` 的 `LogFileListener` 即典型用法),而非重新啟用 `perception/`。刪除 TODO 見 `README.todo`。

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
