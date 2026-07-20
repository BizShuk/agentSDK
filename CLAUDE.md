# CLAUDE.md — agentsdk 技術脈絡 (Technical Context)

`agentsdk` 是 Go Agentic Loop SDK、LLM protocol proxy、provider adapter、認證 CLI 與範例程式的 workspace。本文記錄目前程式碼真正採用的邊界與決策；舊的 `proxy/adaptor` one-to-one 設計已不再是 production path。

## 技術基準 (Current Baseline)

- 語言與 workspace：Go `1.26.0`、`go.work`，共 `10` 個 module entries（root、`tui`、`tools/dependency-analyzer`、7 個 sample module）。`proxy` 與 `llm_provider` 均為外部 module dependency，不再列入本 workspace。Standalone dependency analyzer repo 仍位於 `~/projects/go-dependency-analysis`；本 workspace 同時保留已合併的 in-tree prototype。`provider/*` 已於 551410d 併回 root module，不再各自帶 go.mod。2026-07-19 移除原 `cli/` + `mcp/` 兩個未對接的套件,並移除外部 `video-utils` 依賴 (`go.mod` 與 `cmd/` wiring 同步清掉)。同日落地 harness/UX skeleton：`hook`、`permission`、`session`、`contextfile`、`skill`、`subagent`、`wire` 七個 core-only package + `tui` 獨立 module + runtime steering/follow-up queue，計畫見 [`plans/2026-07-19-harness-ux-modularization.md`](plans/2026-07-19-harness-ux-modularization.md)、來源調查見 [`docs/memory/2026-07-19-agent-client-feature-catalog.md`](docs/memory/2026-07-19-agent-client-feature-catalog.md)。
- root module：`github.com/bizshuk/agentsdk`，內容為 SDK 核心群（core/planning/action/tool/memory/middleware/runtime）、harness 群（hook/permission/session/contextfile/skill/subagent/wire，全部只依賴 core）與組合層（app/config）+ root CLI 子指令（`cmd/provider.go` 的 `provider` smoke-test 子指令,直接呼叫 `core.Provider.Generate/Stream`，不走 Agent/Engine/harness）。`core/` 保持標準函式庫 only；`auth` 是 production Git submodule 且獨立 module，`proxy` 也是獨立 module；二者各有自己的 `main.go` 並可單獨 build 出獨立 binary。root `main.go` 只掛載 `cmd.NewProviderCommand()`，不再掛載 auth/proxy 指令集；root module 仍透過 `config/` 直接 import `auth/model`、`auth/svc`、`auth/utils`，因此 `auth` 仍是 root 的 direct require，`proxy` 已從 root `go.mod` 移除。SDK 核心群不依賴兩者。
- 目前 proxy 架構：`protocol → route → transform → upstream`，三種 client wire format 的 `3×3` directed pair 已接上 handler。
- 來源與規格：現行 pairwise 決策見 [`proxy/docs/specs/2026-07-16-pairwise-agent-provider-transform.md`](proxy/docs/specs/2026-07-16-pairwise-agent-provider-transform.md)；四個來源的 wire-format 盤點見 [`proxy/docs/specs/format/README.md`](proxy/docs/specs/format/README.md)。
- Git submodule：`auth` 是 production auth module（`https://github.com/BizShuk/auth.git`）；`tmp/auth2api` 與 `tmp/cliproxyapi` 僅供格式研究與規格追溯，不是 agentsdk 的 runtime dependency。

## 專案結構 (Project Structure)

```text
agentsdk/
├── README.md                         # 業務範疇與使用者導覽
├── CLAUDE.md                         # 技術脈絡與架構決策（本檔）
├── go.work                           # root + llm_provider/proxy siblings + tui + analyzer + sample/*
├── go.mod                            # github.com/bizshuk/agentsdk
├── main.go                           # cobra root binary；掛載 `provider` smoke-test 子指令（cmd/provider.go）
├── cmd/                              # root cobra subcommands（`provider` 打 core.Provider 不走 Engine）
├── app/                              # CLI agent composition/lifecycle（Agent、preflight、panic recovery）
├── config/                           # AppConfig、middleware presets、RefreshingProvider
├── core/                             # 純狀態機、Message/Part、Event、Instruction、ports（含 ObservationSource）
├── planning/                         # 6 個純函式 DecisionRule FSM
├── action/                           # tool registry、TypedTool/schema、sandbox、approval policy
├── tool/                             # Read/Write/Edit/Bash/Glob/Grep 內建工具
├── hook/                             # core.Hooks 實作：Rule matcher + Func/Command handler（exit 2 = block）
├── permission/                       # core.ApprovalPolicy 實作：Mode × allow/ask/deny specifier rules
├── session/                          # StateStore/WAL 之上的 session 管理：list/resume/fork/tree + meta sidecar
├── contextfile/                      # AGENTS.md/CLAUDE.md 階層載入 + @import 展開（read-only）
├── skill/                            # SKILL.md/commands/templates registry（progressive disclosure）
├── subagent/                         # 定義解析 + task tool（RunFunc closure DI、depth guard）
├── wire/                             # headless envelope：stream-json/RPC/print（core.EventSink adapter）
├── internal/frontmatter/             # skill/subagent 共用的 "---" key:value 解析器
├── tui/                              # 獨立 module：differential renderer、ANSI 工具、Component/Terminal 抽象
├── middleware/                       # chain、retry/timeout/budget/loopguard、安全與 OTel tracing
├── memory/                           # context window、compactor、checkpoint、JSON state/WAL
├── runtime/                          # Engine：dispatch Instruction、fold Event、Run/Resume/HITL
├── cmd/                              # root CLI 子指令；目前只有 `provider` smoke-test（Cobra 註冊進 main.go）
├── cli/                              # (已刪除) 9 種 JSONL Envelope 與 codec —— 無 production caller
├── auth/                             # Git submodule + 獨立 module；main.go 可 build 出獨立 `auth` binary
│   ├── model/                        # Credential、options 與 credential metadata
│   ├── svc/                          # Login、OAuth/PKCE、device flow、FileStore、Resolver
│   ├── utils/                        # active name、store、browser、PKCE helper
│   ├── cmd/                          # login/list/verify/refresh/logout/use 指令集（Install 掛載）
│   ├── provider/                     # 6 個 auth provider 包 + ROUTES registry
│   └── authtest/                     # 共用測試假 provider
├── proxy/                            # 獨立 module；main.go 可 build 出獨立 `proxy` binary
│   ├── handlers/                     # Gin HTTP surface：server、handler、middleware、observability
│   ├── config/                       # LoadConfig/Config（gosdk layered viper、APP_NAME=agentSDK）
│   ├── model/                        # protocol：DTO、SSE parser、ProxyError；package 改名 model
│   │   └── {anthropic,chat,responses}/  # 三個 sub-format 套件
│   ├── svc/                          # 第二層 domain
│   │   ├── route/                    # qualified model → provider family
│   │   ├── transform/                 # 9 組 pairwise request/response/stream transforms
│   │   └── upstream/                  # concrete profile、credential resolver、safe HTTP client
│   ├── docs/                         # proxy-owned plans/specs + wire-format catalog
│   └── cmd/                          # `proxy` 指令（NewCommand）
├── mcp/                              # (已刪除) 獨立 module：MCP Client → action.ToolSource —— 無 production caller
├── provider/
│   ├── anthropic/                    # 獨立 module：anthropic-sdk-go adapter
│   ├── google/                       # 獨立 module：google.golang.org/genai adapter
│   └── openaicompat/                 # 獨立 module：stdlib OpenAI-compatible HTTP/SSE adapter
├── video-utils 外部依賴              # (已移除 2026-07-19) 原 `github.com/bizshuk/video-utils`：audio、frames、subtitles/ffmpeg utilities + video CLI command。root module 不再依賴;若日後重啟,改以獨立 git repo clone 後 add 為 workspace module
├── sample/
│   ├── code-agent/                   # 全 harness 組合 CLI：tui 互動 / -p print / --json（wire）+ session flags
│   ├── file-agent/                   # 6 內建工具的檔案操作 agent
│   ├── greet-agent/                  # 內建工具 + greet tool
│   ├── logdoctor/                    # log listener、todo tools、watch/resume/approve CLI
│   ├── memory-demo/                  # StateStore/WAL/checkpoint demo
│   ├── middleware-demo/              # middleware chain demo
│   └── strategy-demo/                # 6 reasoning strategy demo
├── docs/specs/                       # root SDK architecture/spec history
├── plans/                            # 進行中的落地計畫
└── tmp/                              # submodule 與 runtime symlink，不放 production logic
```

## 技術棧 (Tech Stack)

| 類別                      | 技術                                                | 現況                                                                            |
| ------------------------- | --------------------------------------------------- | ------------------------------------------------------------------------------- |
| Language                  | Go `1.26.0`                                         | `go.work` 管理 10 個 module entries                                              |
| Root runtime              | Go stdlib、`github.com/bizshuk/gosdk v1.1.0`        | config/log/notify 等組合點在 root 或 sample                                     |
| Auth module               | `viper` + stdlib                                    | Git submodule + 獨立 module；credential 機制、Resolver、active.json             |
| HTTP proxy                | `gin-gonic/gin v1.11.0`、`gosdk/mw`、`gosdk/router` | 獨立 module；`/v1` API、health/ping、localhost CORS、API key、per-IP rate limit |
| CLI/config                | `spf13/cobra v1.10.2`、`spf13/viper v1.20.1`        | auth/proxy module CLI、samples、root `provider` 子指令                       |
| State/schema              | `testify v1.11.1`、`invopop/jsonschema v0.14.0`     | table-driven tests、TypedTool JSON Schema                                       |
| IDs/telemetry             | `google/uuid v1.6.0`、OpenTelemetry `v1.44.0`       | request ID、transform warning/loss metrics                                      |
| Anthropic adapter         | `anthropics/anthropic-sdk-go v1.50.2`               | 只在 `provider/anthropic` module 引入                                           |
| Google adapter            | `google.golang.org/genai v1.62.0`                   | 只在 `provider/google` module 引入                                              |
| OpenAI-compatible adapter | `net/http` + JSON + SSE                             | `provider/openaicompat` 不依賴 vendor SDK                                       |
| MCP                       | `modelcontextprotocol/go-sdk v1.6.1`                | `mcp` 獨立 module，轉成 `action.ToolSource`                                     |
| Terminal UI               | Go stdlib only                                      | `tui` 獨立 module；differential rendering、CSI 2026、不用 alternate screen      |

`core/` 不 import `gosdk`、Gin、任何 provider SDK；auth、proxy、provider 與 MCP 的重依賴藉由獨立 `go.mod` 隔離。root module 的直接外部依賴縮減為 `gosdk`、OTel(trace)、`cobra`/`viper`、`jsonschema`、`uuid` 已隨 proxy 遷出；root 仍非 stdlib-only（組合層在此）。

## 核心架構決策 (Core Decisions)

- `core` 是純狀態與 transition contract。`core.Decide(state, event)` 不做 I/O，只回傳下一個 `State` 與 `[]Instruction`；runtime 才執行 model、tool、approval、notify、checkpoint。
- `State` 的對話模型是 `Message{Role, Parts}`；`Part` 支援 text、audio、image、tool use、tool result。JSON tags `scratch` 與 `thinking_kind` 保留以相容舊 state。
- `Instruction` 是 tagged union，透過 `Kind` + optional payload 表示 `call_model`、`call_tool`、`request_approval`、`notify`、`checkpoint`、`emit`、`done`；不建立 vendor-specific effect type。
- `runtime.Engine` 是 shell，維護 `ModelProvider`、`ToolRegistry`、`StateStore`、`WriteAheadLog`、`Notifier` 與 middleware。`Middleware == nil` 代表 no-op；`config.DefaultMiddleware()` 與 `config.SecureMiddleware()` 由 composition root 明確選用。
- `config.DefaultMiddleware()` 順序為 `retry → timeout → budget → loopguard`；安全版本再加入 `sandbox → approval → spotlight → sanitizer`。工具輸出會被 spotlight 標為 untrusted，prompt injection 命中會被 sanitizer 改寫。
- WAL recovery 先載入 snapshot，再依 `LastInputSeq` replay Event；replay 不重新呼叫模型，避免 crash recovery 產生重複副作用。`memory/filestore` 使用 atomic state write + JSONL append。
- `planning/` 的六個 strategy（ThinkThenAct、PlanThenRun、RunThenReview、OneShotReasoning、LearnFromFailure、ChooseAgent）都是 phase FSM；working memory 是 rule 與 runtime/middleware 的通訊介面。
- `action.TypedTool` 用 `invopop/jsonschema` 反射 args schema，呼叫前做 required-field validation；`tool.RegisterDefaults` 集中註冊 Read/Write/Edit/Bash/Glob/Grep。高風險工具由 `DefaultApprovalPolicy` 依 L0–L4 gate。
- `mcp.Client` 只做 MCP `ListTools`/`CallTool` 與 `core.ToolSpec`/`ToolResult` 轉換，不把 MCP transport 混入 core 或 runtime。
- Harness ports（2026-07-19，借鏡 claude-code harness + pi 模組化）：`core` 新增 `Hooks`（`HookEvent`/`HookDecision`）與 `EventSink`（`StreamEvent`）兩個純資料 port；`runtime.Engine` 持有 nil 即 no-op 的 `Hooks`/`Sink` 欄位，`PreToolUse` block 會折成失敗 `ToolResult` 讓 model 看到拒絕、`PostToolUse` 的 `SystemNote` 追加為 system message。所有 harness package 只依賴 `core`，由 composition root 注入。
- `permission.Engine` 是兩軸決策（codex 式）：`Mode`（`default`/`acceptEdits`/`plan`/`bypassPermissions`）× rule specifier（`Bash(git:*)`、`Edit(src/**)`；自寫 `**` matcher，因 `filepath.Match` 的 `*` 不跨 `/`），優先序 `deny > ask > allow`；無 rule 命中時可注入 `Fallback`（如 `action.DefaultApprovalPolicy` 的 autonomy grid）。
- Steering / follow-up queue（pi 式）：`Engine.Steer` 在下一次 Decide 前插入 user message；`Engine.FollowUp` 把「本應完成」轉為續跑（每次一則）。follow-up 續跑會清掉 `think_then_act.phase` 讓 FSM 回到 reason（與 loop 既有 pending_call seeding 同一 seam）；其他五個 rule 的通用 reset 慣例待補。
- `session.Manager` 只加 lineage（meta sidecar：`ID`/`Parent`/`Title`/`Cwd`）與 fork（複製 state+WAL、改 RunID）；transcript 真相仍是 WAL JSONL。
- `wire` 是 `cli/` 的復活但有真 caller path：`runtime → core.EventSink → wire.Sink → JSONL`；Envelope 欄位是對外 API，需維持 JSON round-trip 穩定。
- `tui` 獨立 module 零依賴且不 import agentsdk：caller 把 `core.StreamEvent` 轉成 component 更新；不用 alternate screen、CSI 2026 synchronized output、frame 未變零輸出。

## 認證與 provider 決策 (Auth and Providers)

`auth/` 只提供 mechanism：`Credential`、API key、OAuth authorization-code（PKCE 或 client secret）、device code（RFC 8628 + OIDC discovery）、service-account JWT → Google STS、callback、refresh、FileStore。`auth/provider/<name>/` 封裝單一家 provider，依賴 `auth` 與必要的 provider-specific config 套件；`auth/provider` registry 再統一掛載實作。

`auth/provider.ROUTES` 是 provider id、credential kind、constructor 與 CLI 說明的唯一真相來源，目前 9 個 id：

```text
anthropic          anthropic_oauth
openai             openai_oauth
google             xai                  xai_oauth
antigravity_oauth  vertex
```

重要規則：

- 憑證目錄與 JSON 檔案權限固定 `0700` / `0600`，寫入使用 temp + rename。
- login 在存檔前先 Verify；OAuth/service-account verify 可能輪替 token，呼叫端必須把 `VerifyResult.Credential` 存回。
- `Credential.BaseURL` 隨 API key 保存，gateway/proxy 憑證 verify 與後續 request 必須回到同一端點。
- `auth.Resolver` 是共用的 credential 解析機制：active.json 選取 → 同 provider 字母序 fallback → 環境變數 fallback（`DefaultEnvironmentNames`，含 ollama/llmbox）→ 過期自動 refresh 並持久化。active.json 讀寫慣例由 `auth.LoadActiveNames`/`SaveActiveName` 統一。
- proxy 的 `upstream.CredentialResolver` 是 `auth.Resolver` 的 thin adapter，只把 `auth.UnavailableError` 映射為 `credential_unavailable` ProxyError；runtime 路徑由 `config.NewRefreshingProvider` 包裝任意 `core.ModelProvider`，每次呼叫前 resolve/refresh，token rotation 時重建 inner provider。
- auth CLI 的預設 namespace 是 `~/.config/agentsdk/data/auth`；proxy config 目前以 `agentSDK` namespace 載入（`proxy.APP_NAME`），若兩者要共用憑證，使用 `auth-dir` 明確指定同一目錄。

## Proxy pairwise 決策 (Current Proxy Architecture)

Proxy 不使用 canonical request/response IR，也不保留 legacy `proxy/adaptor`。client protocol 與 concrete provider profile 分離：

```text
HTTP route/source format
  → route.Router（qualified、exact、anchored prefix）
  → transform.Registry（source → target directed pair）
  → upstream.Profile.NormalizeRequest（provider-specific mutation）
  → CredentialResolver + safe upstream.Client
  → reverse response/stream transform
```

支援的 protocol format：

```text
anthropic-messages
openai-chat
openai-responses
```

`transform.NewDefaultRegistry()` 強制驗證完整 `3×3 = 9` 組 `(from, to)`，每組必須有 request transform、non-stream response transform 與 stream factory；identity pair 仍會 decode/normalize，不是 raw passthrough。每個 request 的 stream transformer 都是獨立 state，禁止跨 request 污染。

目前 concrete profile：

| Profile              | Provider family | Target formats     | 特殊規則                                                                                          |
| -------------------- | --------------- | ------------------ | ------------------------------------------------------------------------------------------------- |
| `anthropic`          | `anthropic`     | Anthropic Messages | `/v1/messages`、`anthropic-version`、`/v1/messages/count_tokens`                                  |
| `minimax`            | `minimax`       | Anthropic Messages | Anthropic-compatible `/v1/messages`                                                               |
| `openai-api`         | `openai`        | Responses、Chat    | preferred Responses；`openai-chat/<model>` 強制 Chat                                              |
| `openai-codex-oauth` | `openai`        | Codex Responses    | `/codex/responses`；normalizer 強制 `stream=true`、`store=false`，non-stream client 用 SSE bridge |
| `xai`                | `xai`           | Responses、Chat    | preferred Responses；`xai-chat/<model>` 強制 Chat；只允許 function tools                          |

路由無法唯一解析 model/provider 時回 `400 unknown_model`，不得 fallback 到 Anthropic。credential kind 會參與 profile selection（例如 `openai` API key 與 OAuth 對應不同 profile）。

### Proxy HTTP surface

- 公開 operational endpoints：`/health`、gosdk 提供的 `/healthz` 與 `/ping`。
- 受 API key + per-IP `60 requests/minute` 保護的 `/v1`：`GET /models`、`POST /chat/completions`、`POST /responses`、`POST /messages`、`POST /messages/count_tokens`。
- `/admin/accounts`、`/admin/stats`、`/admin/reload` 目前明確回 `501`，不是已完成的管理 API。
- downstream 只轉送 profile allowlist headers；`Authorization`、`x-api-key`、`Host`、cookie、forwarded headers 等敏感欄位不 passthrough。upstream redirect 被拒絕並映射為 gateway error。
- body、upstream error、SSE frame/line、stream collector 都有上限；server 設定 read-header/idle timeout，stream 不設固定 write timeout。request context 會一路傳到 refresh、HTTP、stream，client disconnect 時取消 upstream。
- SSE parser 以空行作完整 frame boundary，保留 `event`、`id`、`retry`、comments 與 multiline `data`；terminal event 前 EOF 是 protocol failure。stream 已開始後依 source format 發出 native error frame。
- transform warnings/losses 只進 redacted structured log/OTel metrics，不把 prompt、tool output、credential 或 upstream body 寫入一般 log。
- `/v1/messages/count_tokens` 只走 provider native capability；不以固定常數冒充 token count，profile 不支援時回 `501`。

## CLI、設定與持久化 (CLI, Config, Persistence)

`main.go` 直接建立 Cobra root（binary 名稱 `agentsdk`，版本 `0.1.0`），目前只掛載 `cmd.NewProviderCommand()` 提供的 `provider` 子指令：直接呼叫 `core.Provider.Generate`/`Stream`，不走 Agent/Engine/harness，用於 provider adapter 的 wire-format smoke test。Root 不再掛載 `auth/cmd.Install` 或 `proxycmd.NewCommand()`——auth CLI 與 proxy CLI 都從各自 module root 各自 `go build .` 建出獨立 binary，與 root binary 共用相同設定與憑證目錄。auth 函式庫在 `auth/model`、`auth/svc`、`auth/utils`、`auth/provider`，proxy 函式庫在 `proxy/handlers`、`proxy/config`、`proxy/model`、`proxy/svc`。`config.OpenForCLI(appName, level)` 為 sample 建立：

```text
~/.config/<appName>/
├── data/
│   ├── states/<runID>.json
│   ├── wal/<runID>.jsonl
│   └── auth/*.json
└── log/<runID>.log
```

`app.Run` 是 CLI agent 的共同 lifecycle：config → optional preflight → wall-clock deadline → Bootstrap → panic-safe Engine.Run → optional OnComplete。`app.Main` 只負責 signal binding 與 exit code。

Proxy defaults（`proxy/config.go`）：port `8317`、body limit `200 MB`、model timeout `120s`、stream timeout `600s`、count-tokens timeout `30s`、stats enabled、debug off；未設定 API key 時在記憶體產生 `sk-...`，設定持久化由上層 command 負責。

JSONL 對外 envelope (`cli/`) 於 2026-07-19 移除：原 9 種 type (`observation`、`assistant`、`tool_call`、`tool_result`、`approval_request`、`human_decision`、`checkpoint`、`result`、`error`) 無 production caller；外部 wire-format 需求改由 sample 自訂 codec 承接，或待新需求再決定是否以獨立 module 重啟。不得把內部 `State` 欄位直接當成穩定外部 API，新增欄位需維持 JSON round-trip。

## 模組對應 (Module Mapping)

| 領域              | 套件 / 進入點                                                                                                                                                                                                                                    |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 狀態與 ports      | `agentsdk/core`：`NewDecide`、`State`、`Instruction`、`ModelProvider`                                                                                                                                                                            |
| 推理策略          | `agentsdk/planning`：6 個 `New*` DecisionRule constructor                                                                                                                                                                                        |
| runtime           | `agentsdk/runtime`：`NewEngine`、`Run`、`RunWithEvent`、`Resume`、`SubmitHumanDecision`                                                                                                                                                          |
| tools/safety      | `agentsdk/action`、`agentsdk/tool`：`NewRegistry`、`NewTypedTool`、`RegisterDefaults`                                                                                                                                                            |
| memory            | `agentsdk/memory`、`memory/checkpoint`、`memory/filestore`                                                                                                                                                                                       |
| lifecycle hooks   | `agentsdk/hook`：`NewRunner`、`Rule`、`Func`、`Command`（實作 `core.Hooks`）                                                                                                                                                                     |
| permission        | `agentsdk/permission`：`Engine`、`Rule`、`MatchSpec`（實作 `core.ApprovalPolicy`）                                                                                                                                                               |
| session 管理      | `agentsdk/session`：`NewManager`、`Begin`、`List`、`Latest`、`Fork`、`Tree`                                                                                                                                                                      |
| context files     | `agentsdk/contextfile`：`Loader.Load`（AGENTS.md/CLAUDE.md 階層 + `@import`）                                                                                                                                                                    |
| skills/commands   | `agentsdk/skill`：`NewRegistry`、`DiscoverSkills`、`Body`、`ExpandCommand`、`RenderTemplate`                                                                                                                                                     |
| subagents         | `agentsdk/subagent`：`ParseDef`、`DiscoverDefs`、`NewSpawner`（`task` tool）、`Depth`                                                                                                                                                            |
| headless wire     | `agentsdk/wire`：`Envelope`、`NewEncoder`/`NewDecoder`、`NewSink`、`ReadRequest`/`WriteResponse`、`FormatStream`                                                                                                                                 |
| terminal UI       | `agentsdk/tui`（獨立 module）：`Renderer`、`Component`、`Terminal`、`VisibleWidth`/`WrapText`                                                                                                                                                    |
| dependency graph  | external CLI `github.com/bizshuk/go-dependency-analysis`：Go tooling facts + JSON policy heuristics；不加入本 workspace、不被本 repo import                                                                                                        |
| middleware        | `agentsdk/middleware`、`harness`、`loopguard`、`security`、`observability`                                                                                                                                                                       |
| app/config        | `agentsdk/app`、`agentsdk/config`：`app.Run`、`OpenForCLI`、`SecureMiddleware`、`NewRefreshingProvider`                                                                                                                                          |
| root CLI subcommands | `agentsdk/cmd`：`NewProviderCommand`（root cobra `provider` smoke-test CLI；打 `core.Provider.Generate`/`Stream` 不走 Engine；只掛 minimax + anthropic adapter）                                                                       |
| authentication    | `github.com/bizshuk/auth/{model,svc,utils,provider}`（Git submodule + 獨立 module，root 有 `auth` binary main）：`Login`、`For`、`FileStore`、`NewResolver`                                                                                      |
| proxy             | `agentsdk/proxy/handlers`、`agentsdk/proxy/config`、`agentsdk/proxy/model`、`agentsdk/proxy/svc/{route,transform,upstream}`（獨立 module，root 有 `proxy` binary main）：`handlers.New`、`config.LoadConfig`、`model.Format`、`svc/route.Router` |
| JSONL             | (已移除 2026-07-19) 原 `agentsdk/cli`：`Envelope`、`JSONLCodec`                                                                                                                                                                                  |
| MCP               | (已移除 2026-07-19) 原 `agentsdk/mcp` 獨立 module：`mcp.NewClient` 實作 `action.ToolSource`，目前無 caller；若日後需要 MCP，重新以獨立 module 落地並補上 wiring                                                                                  |
| provider adapters | `agentsdk/provider/anthropic`、`google`、`openaicompat`（各自獨立 module）                                                                                                                                                                       |
| video utilities   | (已移除 2026-07-19) 原 `github.com/bizshuk/video-utils`（外部 module，獨立 git repo）：`audio`、`frames`、`subtitles`、`ffmpegutil`、`cmd.NewCommand`。root 不再依賴,不再列入模組對應表                                                          |

## 開發與驗證 (Development and Verification)

前置需求：Go `1.26+`；使用 provider adapter 時依該 module 的 API key/environment。

```bash
cd /Users/shuk/projects/agentSDK
go work sync
go mod download
go build ./...
go test ./... -count=1 -timeout=120s
go-dependency-analysis --workspace /Users/shuk/projects/agentSDK/go.work --format text
go-dependency-analysis --workspace /Users/shuk/projects/agentSDK/go.work --format json --json-indent='  '
go-dependency-analysis --workspace /Users/shuk/projects/agentSDK/go.work --format mermaid
go-dependency-analysis --workspace /Users/shuk/projects/agentSDK/go.work \
  --policy /Users/shuk/projects/go-dependency-analysis/examples/agentsdk.json
```

Analyzer 的 `go-tool-fact` 來自當次 Go toolchain/build context；`policy-heuristic` 才是 layer/heavy dependency 建議。`unused-direct-candidate` 必須先檢查 tests、build tags、platform files、generated code 與 tools，不能直接刪 require。完整 flags 與限制見獨立 repo `/Users/shuk/projects/go-dependency-analysis/README.md`。

驗證所有 workspace modules：

```bash
for mod in auth proxy provider/anthropic provider/google provider/openaicompat \
  sample/code-agent sample/file-agent sample/greet-agent sample/logdoctor \
  sample/memory-demo sample/middleware-demo sample/strategy-demo; do
  (cd "$mod" && go test ./... -count=1 -timeout=120s)
done
```

`provider` 子指令 smoke-test（不打 Agent，直接打 core.Provider）：

```bash
cd /Users/shuk/projects/agentSDK
go run . provider --list-providers
go run . provider --list-models --provider minimax
go run . provider "ping" --provider minimax
go run . provider --stream "say hi in one word" --provider minimax
go run . provider "summarize this repo" --provider anthropic --model claude-sonnet-5
go run . provider "ping" --provider minimax --json | jq
```

常用本地流程：

```bash
# Root binary (go run .) 只掛載 `provider` smoke-test 子指令
go run . provider --list-providers                          # 列出已註冊 provider
go run . provider --list-models --provider minimax          # 列出 provider catalog
go run . provider "ping" --provider minimax                  # 單輪 prompt,直接打 core.Provider
go run . provider --stream "stream me a haiku" --provider anthropic
go run . provider "summarize X" --provider anthropic --model claude-sonnet-5

# auth 與 proxy CLI 從各自 module root 啟動
cd auth && go run . login --provider anthropic
cd auth && go run . login --provider anthropic_oauth --no-browser
cd auth && go run . list
cd auth && go run . verify --all
cd proxy && go run .                         # 啟動 LLM protocol proxy server

cd sample/logdoctor
go run . --fake --max-turns=10 run --once --fixture testdata/error.log

cd sample/code-agent
go run . --fake -p "看看這個專案"        # print 模式（進度走 stderr）
go run . --fake --json -p "test"        # stream-json envelope（wire）
go run . --fake                          # 互動 TUI（tui module；執行中輸入 = Steer）
go run . --fake --sessions               # 列出本目錄 sessions；-c / -r / --fork 續跑

# 真實 provider：--provider 選 adapter，各自讀自己的 env（--fake 拿掉）
export MINIMAX_API_KEY=...                # minimax adapter 預設就讀這個
go run . -p "explore this repo"           # 預設 --provider minimax、model=MiniMax-M3
go run .                                   # 互動 TUI 打真實 model
go run . --provider anthropic -p "..."    # 改讀 ANTHROPIC_API_KEY（含 minimax gateway：--base-url）
```

`code-agent` 的 provider 選擇：`--provider minimax`（預設，讀 `MINIMAX_API_KEY`/`MINIMAX_BASE_URL`，`provider/minimax` stdlib adapter）或 `--provider anthropic`（讀 `ANTHROPIC_API_KEY`）；`--model` 留空用 adapter flagship 預設；`--api-key`/`--base-url` 為顯式覆寫。

`sample/logdoctor` 的 real provider 旗標為 `anthropic`、`openaicompat`、`google`；`--fake` 與 `--provider` 互斥。`sample/file-agent` 與 `sample/greet-agent` 使用 Anthropic-compatible adapter 與 `SecureMiddleware`。

## 目前狀態與待辦 (Status and Open Items)

| 項目                                                                                                                       | 狀態                                                                                                                                                         |
| -------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| M1 核心範式與 sample                                                                                                       | 完成                                                                                                                                                         |
| M2 state/WAL/checkpoint/retry/timeout/loopguard                                                                            | 完成                                                                                                                                                         |
| M3 tool schema、sandbox、MCP、spotlight、sanitizer、tracing                                                                | 完成                                                                                                                                                         |
| M4 HITL、security wiring、三個 provider adapter                                                                            | 完成                                                                                                                                                         |
| M5 built-in tools、sample wiring、`app` lifecycle                                                                          | 完成                                                                                                                                                         |
| M6 auth mechanism、9 provider ids、auth CLI                                                                                | 完成；`config.NewRefreshingProvider` 已補上呼叫前自動 refresh                                                                                                |
| Proxy 3×3 pairwise cutover 與安全 hardening                                                                                | 完成（現行 branch）                                                                                                                                          |
| 四來源 37 entity wire-format catalog                                                                                       | 完成，見 [`proxy/docs/specs/format/README.md`](proxy/docs/specs/format/README.md)                                                                            |
| module 拆分：`auth`、`proxy` 獨立 module；`config` 解體；`perception/` 刪除                                                | 完成，見 [`plans/2026-07-18-architecture-module-split-roadmap.md`](plans/2026-07-18-architecture-module-split-roadmap.md)                                    |
| Harness/UX skeleton：`hook`/`permission`/`session`/`contextfile`/`skill`/`subagent`/`wire` + `tui` module + steering queue | skeleton 完成（全部含測試），細節項見 `README.todo`；計畫見 [`plans/2026-07-19-harness-ux-modularization.md`](plans/2026-07-19-harness-ux-modularization.md) |

目前明確未完成或刻意保留：

- Anthropic/Google provider 的 `Stream` 目前以 `Generate` 結果折成 chunk；只有 `openaicompat` 與 proxy path 使用原生/完整 SSE 轉換。
- `/admin/*` 仍是未實作 placeholder；不要在文件或 client 中當成穩定 API。
- release tag 順序：`auth` → `proxy` → root（replace 指向相對路徑，只在本 repo workspace 生效；samples 於 repo 外單獨 build 需等 tag）。
- credential 儲存改 env placeholder（`README.todo` 新項）尚未設計，動工前先開 plan。
- Harness skeleton 的刻意保留：`tui` 尚無 Editor/raw-mode 輸入與 Markdown component（`ProcessTerminal.Size` 暫讀 `COLUMNS`，互動輸入走 cooked-mode line input）；streaming 仍是 folded events（`STREAM_MESSAGE_DELTA` 保留位）；hook/permission 的 settings 檔載入層未做。`sample/code-agent` 已落地為 tui 的第一個真 caller（互動 TUI + `-p` print + `--json` wire + session flags + `.agentsdk/` skills/commands/agents 探索），composition 全在 `sample/code-agent/cmd/compose.go`。完整清單見 `README.todo` 的 Harness/UX 段。

## 慣例與注意事項 (Conventions and Caveats)

- 常數使用 `SCREAMING_SNAKE_CASE`，變數、函式、型別使用 Go `MixedCaps`；package 名稱使用單字。
- 錯誤以 `fmt.Errorf("...: %w", err)` wrap；公開 error 不帶 credential、authorization、prompt 或未清理 upstream body。
- 測試採 table-driven + `t.Run`，`testify/assert` 與 `testify/require` 並用；`internal/testutil` 只可被測試使用。
- `core.Decide`、planning rules、transform pair 不得直接做 I/O、讀 credential 或建立 HTTP request；這些責任分別屬於 runtime、upstream 與 auth。
- `sample/logdoctor/core` 與 `agentsdk/core` 是不同 module path；import 時使用 `domain` / `sdkcore` alias。
- `proxy/docs/specs/2026-07-16-client-llm-adaptor.md` 是 legacy historical design；修改 proxy 時以 pairwise spec、現行 `proxy/` code 與測試為準。
- 修改 package tree、module、路由或 protocol contract 後，必須同步本檔；業務範疇變更才同步 `README.md`。格式 catalog 的 entity/來源異動則同步 [`proxy/docs/specs/format/README.md`](proxy/docs/specs/format/README.md)。
