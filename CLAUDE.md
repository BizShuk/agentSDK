# CLAUDE.md — agentsdk 技術脈絡 (Technical Context)

`agentsdk` 是 Go Agentic Loop SDK、LLM protocol proxy、provider adapter、認證 CLI 與範例程式的 workspace。本文記錄目前程式碼真正採用的邊界與決策；舊的 `proxy/adaptor` one-to-one 設計已不再是 production path。

## 技術基準 (Current Baseline)

- 語言與 workspace：Go `1.26.0`、`go.work`，共 `12` 個 module（root、`mcp`、3 個 provider module、6 個 sample module、`utils/video`）。
- root module：`github.com/bizshuk/agentsdk`。`core/` 保持標準函式庫 only；root 的 runtime、CLI、proxy 與 wiring 會使用外部套件。
- 目前 proxy 架構：`protocol → route → transform → upstream`，三種 client wire format 的 `3×3` directed pair 已接上 handler。
- 來源與規格：現行 pairwise 決策見 [`docs/specs/2026-07-16-pairwise-agent-provider-transform.md`](docs/specs/2026-07-16-pairwise-agent-provider-transform.md)；四個來源的 wire-format 盤點見 [`docs/specs/format/README.md`](docs/specs/format/README.md)。
- 參考 submodule：`tmp/auth2api` 與 `tmp/cliproxyapi` 僅供格式研究與規格追溯，不是 agentsdk 的 runtime dependency。

## 專案結構 (Project Structure)

```text
agentsdk/
├── README.md                         # 業務範疇與使用者導覽
├── CLAUDE.md                         # 技術脈絡與架構決策（本檔）
├── go.work                           # root + mcp + provider/* + sample/* + utils/video
├── go.mod                            # github.com/bizshuk/agentsdk
├── main.go                           # auth-cli binary entry
├── cmd/                              # auth、proxy、credential use 指令樹
├── app/                              # CLI agent composition/lifecycle（Agent、preflight、panic recovery）
├── config/                           # AppConfig、middleware presets、proxy config
├── core/                             # 純狀態機、Message/Part、Event、Instruction、ports
├── perception/                       # ObservationSource 與 normalize helper
├── planning/                         # 6 個純函式 DecisionRule FSM
├── action/                           # tool registry、TypedTool/schema、sandbox、approval policy
├── tool/                             # Read/Write/Edit/Bash/Glob/Grep 內建工具
├── middleware/                       # chain、retry/timeout/budget/loopguard、安全與 OTel tracing
├── memory/                           # context window、compactor、checkpoint、JSON state/WAL
├── runtime/                          # Engine：dispatch Instruction、fold Event、Run/Resume/HITL
├── cli/                              # 9 種 JSONL Envelope 與 codec
├── auth/                             # credential、API key、OAuth/PKCE、device flow、FileStore
├── proxy/                            # Gin HTTP proxy 與 pairwise protocol bridge
│   ├── protocol/                     # Format、typed DTO、ProxyError、完整 SSE frame parser
│   ├── route/                        # qualified/exact/prefix model routing
│   ├── transform/                    # 9 組 request/response/stream pairwise transforms
│   └── upstream/                     # concrete profile、credential resolver、safe HTTP client
├── mcp/                              # 獨立 module：MCP Client → action.ToolSource
├── provider/
│   ├── anthropic/                    # 獨立 module：anthropic-sdk-go adapter
│   ├── google/                       # 獨立 module：google.golang.org/genai adapter
│   └── openaicompat/                 # 獨立 module：stdlib OpenAI-compatible HTTP/SSE adapter
├── utils/video/                      # 獨立 module：audio、frames、subtitles/ffmpeg utilities + video CLI command
├── sample/
│   ├── file-agent/                   # 6 內建工具的檔案操作 agent
│   ├── greet-agent/                  # 內建工具 + greet tool
│   ├── logdoctor/                    # log listener、todo tools、watch/resume/approve CLI
│   ├── memory-demo/                  # StateStore/WAL/checkpoint demo
│   ├── middleware-demo/              # middleware chain demo
│   └── strategy-demo/                # 6 reasoning strategy demo
├── docs/specs/                       # architecture/spec history
│   └── format/                       # 37 個 client/provider directed wire entities
├── plans/                            # 進行中的落地計畫
└── tmp/                              # submodule 與 runtime symlink，不放 production logic
```

## 技術棧 (Tech Stack)

| 類別 | 技術 | 現況 |
| --- | --- | --- |
| Language | Go `1.26.0` | `go.work` 管理 12 個 module |
| Root runtime | Go stdlib、`github.com/bizshuk/gosdk v1.1.0` | config/log/notify 等組合點在 root 或 sample |
| HTTP proxy | `gin-gonic/gin v1.11.0`、`gosdk/mw`、`gosdk/router` | `/v1` API、health/ping、localhost CORS、API key、per-IP rate limit |
| CLI/config | `spf13/cobra v1.10.2`、`spf13/viper v1.20.1` | auth-cli、proxy、samples 與 layered settings |
| State/schema | `testify v1.11.1`、`invopop/jsonschema v0.14.0` | table-driven tests、TypedTool JSON Schema |
| IDs/telemetry | `google/uuid v1.6.0`、OpenTelemetry `v1.44.0` | request ID、transform warning/loss metrics |
| Anthropic adapter | `anthropics/anthropic-sdk-go v1.50.2` | 只在 `provider/anthropic` module 引入 |
| Google adapter | `google.golang.org/genai v1.62.0` | 只在 `provider/google` module 引入 |
| OpenAI-compatible adapter | `net/http` + JSON + SSE | `provider/openaicompat` 不依賴 vendor SDK |
| MCP | `modelcontextprotocol/go-sdk v1.6.1` | `mcp` 獨立 module，轉成 `action.ToolSource` |

`core/` 不 import `gosdk`、Gin、任何 provider SDK；provider 與 MCP 的重依賴藉由獨立 `go.mod` 隔離。root module 仍包含 proxy、CLI、config、OTel 等應用層依賴，不能再描述成整個 root 都是 stdlib-only。

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
- proxy 的 `CredentialResolver` 會在 request 前選 active credential、檢查 expiry、refresh 並持久化新 token；一般 provider `ModelProvider` 呼叫前的自動 refresh 仍列在 [`README.todo`](README.todo)。
- auth CLI 的預設 namespace 是 `~/.config/agentsdk/data/auth`；proxy config 目前以 `agentSDK` namespace 載入，若兩者要共用憑證，使用 `auth-dir` 明確指定同一目錄。

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

| Profile | Provider family | Target formats | 特殊規則 |
| --- | --- | --- | --- |
| `anthropic` | `anthropic` | Anthropic Messages | `/v1/messages`、`anthropic-version`、`/v1/messages/count_tokens` |
| `minimax` | `minimax` | Anthropic Messages | Anthropic-compatible `/v1/messages` |
| `openai-api` | `openai` | Responses、Chat | preferred Responses；`openai-chat/<model>` 強制 Chat |
| `openai-codex-oauth` | `openai` | Codex Responses | `/codex/responses`；normalizer 強制 `stream=true`、`store=false`，non-stream client 用 SSE bridge |
| `xai` | `xai` | Responses、Chat | preferred Responses；`xai-chat/<model>` 強制 Chat；只允許 function tools |

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

`main.go` 建立 Cobra root（binary 名稱 `auth-cli`，版本 `0.1.0`），目前指令包含 `login`、`list`、`verify`、`refresh`、`logout`、`use`、`proxy`、`video`。`video` 由獨立 module 的 `utils/video/cmd.NewCommand()` 組合回 root CLI。`config.OpenForCLI(appName, level)` 為 sample 建立：

```text
~/.config/<appName>/
├── data/
│   ├── states/<runID>.json
│   ├── wal/<runID>.jsonl
│   └── auth/*.json
└── log/<runID>.log
```

`app.Run` 是 CLI agent 的共同 lifecycle：config → optional preflight → wall-clock deadline → Bootstrap → panic-safe Engine.Run → optional OnComplete。`app.Main` 只負責 signal binding 與 exit code。

Proxy defaults（`config/proxy.go`）：port `8317`、body limit `200 MB`、model timeout `120s`、stream timeout `600s`、count-tokens timeout `30s`、stats enabled、debug off；未設定 API key 時在記憶體產生 `sk-...`，設定持久化由上層 command 負責。

JSONL 對外 envelope 在 `cli/` 定義 9 種 type：`observation`、`assistant`、`tool_call`、`tool_result`、`approval_request`、`human_decision`、`checkpoint`、`result`、`error`。不得把內部 `State` 欄位直接當成穩定外部 API，新增欄位需維持 JSON round-trip。

## 模組對應 (Module Mapping)

| 領域 | 套件 / 進入點 |
| --- | --- |
| 狀態與 ports | `agentsdk/core`：`NewDecide`、`State`、`Instruction`、`ModelProvider` |
| 推理策略 | `agentsdk/planning`：6 個 `New*` DecisionRule constructor |
| runtime | `agentsdk/runtime`：`NewEngine`、`Run`、`RunWithEvent`、`Resume`、`SubmitHumanDecision` |
| tools/safety | `agentsdk/action`、`agentsdk/tool`：`NewRegistry`、`NewTypedTool`、`RegisterDefaults` |
| memory | `agentsdk/memory`、`memory/checkpoint`、`memory/filestore` |
| middleware | `agentsdk/middleware`、`harness`、`loopguard`、`security`、`observability` |
| app/config | `agentsdk/app`、`agentsdk/config`：`app.Run`、`OpenForCLI`、`SecureMiddleware` |
| authentication | `agentsdk/auth`、`agentsdk/auth/provider`：`Login`、`For`、`FileStore` |
| proxy | `agentsdk/proxy`：`proxy.New`、`protocol`、`route`、`transform`、`upstream` |
| JSONL | `agentsdk/cli`：`Envelope`、`JSONLCodec` |
| MCP | `agentsdk/mcp`（獨立 module）：`mcp.NewClient` |
| provider adapters | `agentsdk/provider/anthropic`、`google`、`openaicompat`（各自獨立 module） |
| video utilities | `agentsdk/utils/video`（獨立 module）：`audio`、`frames`、`subtitles`、`ffmpegutil`、`cmd.NewCommand` |

## 開發與驗證 (Development and Verification)

前置需求：Go `1.26+`；使用 provider adapter 時依該 module 的 API key/environment；`video` module 的部分指令另需系統 `ffmpeg`、`ffprobe` 或對應 Python/ML runtime。

```bash
cd /Users/shuk/projects/agentSDK
go work sync
go mod download
go build ./...
go test ./... -count=1 -timeout=120s
```

驗證所有 workspace modules：

```bash
for mod in utils/video mcp provider/anthropic provider/google provider/openaicompat \
  sample/file-agent sample/greet-agent sample/logdoctor \
  sample/memory-demo sample/middleware-demo sample/strategy-demo; do
  (cd "$mod" && go test ./... -count=1 -timeout=120s)
done
```

常用本地流程：

```bash
go run . login --provider anthropic
go run . login --provider anthropic_oauth --no-browser
go run . list
go run . verify --all
go run . proxy

cd sample/logdoctor
go run . --fake --max-turns=10 run --once --fixture testdata/error.log
```

`sample/logdoctor` 的 real provider 旗標為 `anthropic`、`openaicompat`、`google`；`--fake` 與 `--provider` 互斥。`sample/file-agent` 與 `sample/greet-agent` 使用 Anthropic-compatible adapter、`SecureMiddleware` 與 JSONL effect output。

## 目前狀態與待辦 (Status and Open Items)

| 項目 | 狀態 |
| --- | --- |
| M1 核心範式與 sample | 完成 |
| M2 state/WAL/checkpoint/retry/timeout/loopguard | 完成 |
| M3 tool schema、sandbox、MCP、spotlight、sanitizer、tracing | 完成 |
| M4 HITL、security wiring、三個 provider adapter | 完成 |
| M5 built-in tools、sample wiring、`app` lifecycle | 完成 |
| M6 auth mechanism、9 provider ids、auth CLI | 完成；一般 ModelProvider 呼叫前自動 refresh 仍待補 |
| Proxy 3×3 pairwise cutover 與安全 hardening | 完成（現行 branch） |
| 四來源 37 entity wire-format catalog | 完成，見 [`docs/specs/format/README.md`](docs/specs/format/README.md) |

目前明確未完成或刻意保留：

- `perception/` 尚無 production consumer；先保留，後續再決定刪除或重新定位，見 [`README.todo`](README.todo)。
- 一般 `core.ModelProvider` 呼叫前的 expiry/refresh wiring 尚未統一；proxy request path 已有 `CredentialResolver` refresh。
- Anthropic/Google provider 的 `Stream` 目前以 `Generate` 結果折成 chunk；只有 `openaicompat` 與 proxy path 使用原生/完整 SSE 轉換。
- `/admin/*` 仍是未實作 placeholder；不要在文件或 client 中當成穩定 API。

## 慣例與注意事項 (Conventions and Caveats)

- 常數使用 `SCREAMING_SNAKE_CASE`，變數、函式、型別使用 Go `MixedCaps`；package 名稱使用單字。
- 錯誤以 `fmt.Errorf("...: %w", err)` wrap；公開 error 不帶 credential、authorization、prompt 或未清理 upstream body。
- 測試採 table-driven + `t.Run`，`testify/assert` 與 `testify/require` 並用；`internal/testutil` 只可被測試使用。
- `core.Decide`、planning rules、transform pair 不得直接做 I/O、讀 credential 或建立 HTTP request；這些責任分別屬於 runtime、upstream 與 auth。
- `sample/logdoctor/core` 與 `agentsdk/core` 是不同 module path；import 時使用 `domain` / `sdkcore` alias。
- `docs/specs/2026-07-16-client-llm-adaptor.md` 是 legacy historical design；修改 proxy 時以 pairwise spec、現行 `proxy/` code 與測試為準。
- 修改 package tree、module、路由或 protocol contract 後，必須同步本檔；業務範疇變更才同步 `README.md`。格式 catalog 的 entity/來源異動則同步 [`docs/specs/format/README.md`](docs/specs/format/README.md)。
