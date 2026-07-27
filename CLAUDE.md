# CLAUDE.md — agentsdk 技術脈絡 (Technical Context)

`agentsdk` 是 Go Agentic Loop SDK、LLM protocol proxy、provider adapter、認證 CLI 與範例程式的 workspace。本文記錄目前程式碼真正採用的邊界與決策；舊的 `proxy/adaptor` one-to-one 設計已不再是 production path。

## 技術基準 (Current Baseline)

- 語言與 workspace：Go `1.26.0`、`go.work`，共 `10` 個 module entries（root + `9` 個 sample module）。`proxy` 與 `llm_provider` 均為外部 module dependency，不再列入本 workspace。Standalone dependency analyzer repo 仍位於 `~/projects/go-dependency-analysis`；in-tree prototype 已於 `19f0d41` 移除。`provider/*` 已於 `551410d` 併回 root module；`tui/` 於 2026-07-21 併回 root，2026-07-26 再下沉為 `sample/code-agent/tui`（只有 agent 實作需要終端機畫面，SDK 表面不需要），全程不帶自己的 `go.mod`。2026-07-19 移除原 `cli/` + `mcp/` 兩個未對接的套件,並移除外部 `video-utils` 依賴 (`go.mod` 與 `cmd/` wiring 同步清掉)。同日落地 harness/UX skeleton：`hook`、`permission`、`session`、`contextfile`、`skill`、`subagent`、`wire` 七個 core-only package + `tui` sub-package（zero-dep） + runtime steering/follow-up queue；`contextfile` 於 2026-07-24 併入 `prompt`（固定行為、無客製化縫），計畫見 [`plans/2026-07-19-harness-ux-modularization.md`](plans/2026-07-19-harness-ux-modularization.md)、來源調查見 [`docs/memory/2026-07-19-agent-client-feature-catalog.md`](docs/memory/2026-07-19-agent-client-feature-catalog.md)。2026-07-22 落地 agent skeleton：`agent/`（含 `agent/spec` 宣告層）、`prompt/`、`provider/registry.go` 三個新 package + root `wizard` 子指令，計畫見 [`plans/2026-07-22-agent-skeleton-config-opt-in.md`](plans/2026-07-22-agent-skeleton-config-opt-in.md)。
- root module：`github.com/bizshuk/agentsdk`，內容為 SDK 核心群（core/reasoning/tool/memory/middleware/runtime）、harness 群（middleware/hook、agent/permission、agent/session、agent/wire、skill、prompt）、組合層 `agent`、process host `agent/cli` 與 root CLI 子指令。`agent/spec` 是只 import `core` 的宣告層；`agent` 才組裝 reasoning/provider/harness。`core/` 保持 stdlib only。`auth` 只被 `provider/credential` import；`proxy` 已從 root module 移除。
- `core/` 於 2026-07-26 依 domain 重分檔但維持單一 package：run state（`state.go`/`budget.go`/`run_status.go`/`autonomy.go`）、transition（`event.go`/`instruction.go`/`decision.go`）、model boundary（`message.go`/`model.go`/`provider.go`/`credential.go`）、tool/HITL（`tool.go`/`approval.go`）及 runtime ports（`observation.go`/`persistence.go`/`notification.go`/`hook.go`/`stream.go`）。`core.Decide` 留作純 transition contract；`State.ReasoningStyle` 直接使用 `string`，`REASON_*` 是無 named type 的字串常數。`DecisionRule`/`NewDecide` 歸到實際消費它們的 `reasoning/`。原本混合多個 domain 的 `input.go`/`port.go` 與沿用舊術語的 `effect.go`/`step.go`/`thinking.go` 已移除。
- `agent/` 於 2026-07-26 收斂為 `7` 個 root production 檔：`agent.go` 集中公開契約與 aliases，`options.go` 集中注入，engine/runtime seams 併入 `build.go` 與 `host.go`；production code `1885 → 1397` 行，公開 API 與行為不變。`agent/spec` 的 built-in tool vocabulary 改為純宣告值並以 drift test 對 `tool.BuiltinNames`，production dependencies `206 → 69`，恢復只依賴 `core` 的邊界。2026-07-27 再移除無完整 consumer 的 `Output` 設定，`build.go` 收斂為 7-stage composition；presentation 由 frontend 以 `WithSink` 或直接設定 `Engine.Sink` 接管。
- Vocabulary ownership（2026-07-27）：`tool/builtin.Register` 擁有 tool name → constructor 與 allowlist registration，`reasoning.NewRule` 擁有 style → `DecisionRule`，`core.ParseAutonomyLevel` 擁有 config string → runtime enum。`agent/build.go` 只呼叫 owner API 並保留 injected tool/rule 的 later-wins 覆寫順序；unknown value 不再由 composition root 猜測或默認成 `L2`。跨 package drift tests 直接驗證 `agent/spec` choices 可被三個 owner API 接受。
- Provider protocol codec（2026-07-27）：stdlib-only `provider/protocol/sse` 擁有完整 SSE frame boundary、multiline `data`、UTF-8 BOM、`event`/`id`/`retry`/comments、大小限制與 frame writer；不解讀 `[DONE]`、`message_stop`、`response.completed` 等 provider terminal semantics。Google / Ollama 的 request bytes、non-stream folding、SSE chunks 與 failure semantics 先由同一組跨 adapter golden/httptest 鎖定，再收斂到使用該 frame codec 的 `provider/protocol/openaichat`。兩個 adapter 只保留 endpoint、default model、auth header 與 vendor error prefix；Grok、Anthropic、MiniMax、Codex、Antigravity 的 DTO 未合併。
- Provider image capability（2026-07-27）：`provider.ImageGenerator` / `ImageRequest` / `ImageResult` 留在 provider layer，不放進沒有 image consumer 的 agent runtime `core.Provider`。`Entry.NewImage` 與 `provider.NewImage` 是一致建構路徑，`Entry.Supports` / `Capabilities` 負責 discovery；unsupported adapter 回 typed `ErrUnsupportedCapability`。Google / Grok 以同一組 golden/httptest 證明 `POST /images/generations` 的 request、URL/base64 result、usage、auth、structured API error 與 cancellation semantics 相容後，共用 `provider/protocol/openaiimage` codec；成功 image response 上限 `128 MiB`，error body 上限 `1 MiB`、保留的 details 上限 `16 KiB`，不外洩原始 upstream body。規格見 [`docs/specs/2026-07-27-provider-auth-image-capabilities.md`](docs/specs/2026-07-27-provider-auth-image-capabilities.md)。
- Reasoning content boundary（2026-07-28）：`core.Part` 新增 `PART_KIND_REASONING`；`Part.Text` 是可攜的 reasoning 文字，`ReasoningState` 保存 Anthropic opaque `signature` 與 Responses `id` / `encrypted_content`。這與 `State.ReasoningStyle`（agent strategy selector）正交。`ModelResult.Parts` 是有序 canonical assistant content，`Text` / `ToolCalls` 是相容投影；runtime 先 `NormalizeContent` 再把完整 Parts 寫入 transcript，memory token fallback 也計入 reasoning 文字。Anthropic / Antigravity / MiniMax 的 non-stream 與 SSE thinking、Codex Responses request/stream reasoning item、Grok `reasoning_content` 已接線；Codex 固定要求 `reasoning.encrypted_content`。無法表示 provider-specific continuation metadata 的 wire path 明確報錯，不靜默丟棄；proxy 自有的版本化 signature envelope 仍由外部 proxy 解碼。
- Provider access sample（2026-07-27）：`provider/sample` 是 root module 內的 package-local executable，不另立 `go.mod`；`--list` 從 `provider.Entry` 產生 provider × chat/image/audio × auth-env matrix，`--auth auto|api_key|oauth` 直接走 `Options.CredentialKind`，chat/image 分別走 `provider.New` / `NewImage`。audio 尚無單一 production contract 或 adapter wire consumer，故 sample 回 `UnsupportedCapabilityError{Capability:"audio"}`，不把 `core.Part.Audio` 靜默送進會忽略它的 adapter。
- 目前 proxy 架構：`protocol → route → transform → upstream`，三種 client wire format 的 `3×3` directed pair 已接上 handler。
- 來源與規格：現行 pairwise 決策見 `proxy/docs/specs/2026-07-16-pairwise-agent-provider-transform.md`（外部 repo）；四個來源的 wire-format 盤點見 `proxy/docs/specs/format/README.md`（外部 repo）。
- 外部依賴：`auth` 是外部 module（`github.com/bizshuk/auth`，go.mod require，非 submodule），只被 `provider/credential`（`model`/`svc`/`provider`）使用——這是全 repo 唯一允許 import 它的 package，由 `go list -deps` 驗收；`proxy` 已無任何殘留（無目錄、無 require、無 import）。`tmp/auth2api` 與 `tmp/cliproxyapi` 僅供格式研究，不是 runtime dependency。

## 專案結構 (Project Structure)

```text
agentsdk/
├── README.md                         # 業務範疇與使用者導覽
├── CLAUDE.md                         # 技術脈絡與架構決策（本檔）
├── go.work                           # root + 9 sample modules（tui/provider 皆無獨立 go.mod）
├── go.mod                            # github.com/bizshuk/agentsdk
├── main.go                           # cobra root binary；掛載 `provider` 與 `wizard` 兩個子指令
├── cmd/                              # root cobra subcommands
│   ├── agent/                        # agent 相關子指令
│   │   └── wizard/                   # `wizard`/`w` 設定產生器（逐階段 wizard prompt，產出 agent.Config）
│   └── provider.go                   # `provider` smoke-test（直接打 core.Provider.Generate / core.StreamProvider.Stream）
├── agent/                            # 組裝層：Config → 7 stage pipeline → Engine；agent.go 集中公開契約
│   ├── cli/                          # process host：OpenForCLI/Main/Run、signal、slog、os.Exit
│   ├── spec/                         # 宣告層：Config/Choice/tier 展開/驗證（只 import core，可獨立被讀取）
│   ├── permission/                   # core.ApprovalPolicy 實作：Mode × allow/ask/deny specifier rules（只 import core；2026-07-26 自 root permission/ 移入）
│   ├── session/                      # StateStore/WAL 之上的 session 管理：list/resume/fork/tree + meta sidecar（只 import core；2026-07-26 自 root session/ 移入）
│   └── wire/                         # headless envelope：stream-json/RPC/print（core.EventSink adapter；只 import core；2026-07-26 自 root wire/ 移入）
├── prompt/                           # content management：Slot(system/user/reminder)、Source、Builder、LoadContextFiles（AGENTS.md/CLAUDE.md 階層 + @import；2026-07-24 自 contextfile 併入）
├── prompt/source/                    # 內建 Source 實作：PersonaSource/ContextFileSource/EnvSource/ReminderSource + SkillSource（透過 SkillProvider interface 收 *skill.Registry，prompt/source 仍只 import prompt + stdlib）
├── core/                             # 純狀態機；單一 package，檔案依 domain 分組
│   ├── state.go / budget.go / run_status.go / autonomy.go
│   ├── event.go / observation.go / instruction.go / decision.go
│   ├── message.go / model.go / provider.go / credential.go
│   ├── tool.go / approval.go
│   └── persistence.go / notification.go / hook.go / stream.go
├── reasoning/                        # DecisionRule/NewRule/NewDecide + 6 個純函式 FSM
├── tool/                             # core.Tool alias、RawMessage converter、Registry/RegisterFunc、sandbox
│   └── builtin/                      # allowlist-aware Register + Read/Write/Edit/Bash/Glob/Grep
├── skill/                            # SKILL.md/commands/subagents registry（progressive disclosure + SubAgent/Spawner "task" tool）
├── utils/                            # 根層共用 utilities umbrella：utils/frontmatter/（adrg/frontmatter YAML/TOML/JSON wrapper,key:value 攤平為 string map）+ utils/configfile/（副檔名決定編碼、一律以 JSON 呈現給 caller,故 `json` tag 是唯一真相）+ utils/testutil/（in-process fake provider/state store/notifier）
├── middleware/                       # chain、retry/timeout/budget/loopguard、安全與 OTel tracing；preset/ 子包（Default/Secure 具名鏈，2026-07-26 自 config/default.go 併入）；hook/ 子包（core.Hooks 實作：Rule matcher + Func/Command handler，exit 2 = block；以 middleware-style handler 連續執行 + HookDecision 合併，signature 仍獨立）
├── memory/                           # context window、compactor、checkpoint、JSON state/WAL
├── runtime/                          # Engine：dispatch Instruction、fold Event、Run/Resume/HITL
├── cmd/                              # root CLI 子指令；目前只有 `provider` smoke-test（Cobra 註冊進 main.go）
├── cli/                              # (已刪除) 9 種 JSONL Envelope 與 codec —— 無 production caller
├── auth 外部依賴                     # `github.com/bizshuk/auth`：獨立 repo + go.mod require（非 in-tree 目錄）
│                                     # 唯一 importer 是 provider/credential；adapter 與 agent 都看不到它
├── proxy 外部依賴                    # 已完全脫離本 repo：無目錄、無 go.mod require、無任何 import
├── mcp/                              # (已刪除) 獨立 module：MCP Client → tool.ToolSource —— 無 production caller
├── provider/                         # package provider（2026-07-26 自 package registry 改名，目錄與 package 名終於一致，移除全部 import alias）
│   ├── capability.go                 # model/image capability discovery + typed unsupported error
│   ├── image.go / error.go           # ImageGenerator contract、request/result、auth wrapper、structured API error
│   ├── registry.go                   # Entry 是 name / metadata / static catalog / model+image factory 的唯一真相 + DEFAULT_NAME
│   ├── registry_options.go           # Options（unresolved）→ ResolvedConfig（construction input）；集中 env / credential class resolution
│   ├── adapter.go                    # Adapter = core.Provider + core.StreamProvider；discovery data 不進 runtime client
│   ├── decorator.go                  # Decorator = func(ctx) (core.Auth, error)：model/image 每次 request 共用解析規則
│   ├── credential/                   # 全 repo 唯一 import bizshuk/auth 之處：(name, kind) → auth route id 對照、Decorator 實作、Login 委派
│   ├── all/                          # meta-package：blank-import 全部 adapter 的便利入口
│   ├── sample/                       # provider/auth/chat-image-audio capability matrix + direct access CLI
│   ├── protocol/
│   │   ├── sse/                      # stdlib-only 完整 SSE frame decoder / writer；不含 provider terminal semantics
│   │   ├── openaichat/               # Google/Ollama 共用 request/response codec + Frame → ModelChunk
│   │   └── openaiimage/              # Google/Grok 共用 /images/generations JSON codec + bounded response/error
│   ├── utils/                        # provider 共用 utilities：live model catalog helper（Fetch/DecodeIDList/Merge）
│   ├── anthropic/                    # 獨立 module：anthropic-sdk-go adapter
│   ├── antigravity/                  # adapter：Google Cloud Antigravity OAuth
│   ├── codex/                        # adapter：OpenAI Codex OAuth
│   ├── google/                       # 獨立 module：google.golang.org/genai adapter
│   ├── grok/                         # adapter：xAI Grok
│   ├── minimax/                      # adapter：MiniMax stdlib HTTP/SSE
│   └── ollama/                       # adapter：本地 Ollama endpoint
├── sample/
│   │                                 # 目錄前綴即分類（2026-07-26）：demo-* 是手接單一元件的展示，*-agent 是完整 agent
│   ├── code-agent/                   # 全 harness 組合 CLI：tui 互動 / -p print / --json（wire）+ session flags；compose.go 用 agent 宣告（333→101 行）
│   │   └── tui/                      # zero-dep differential renderer、ANSI 工具、Component/Terminal 抽象（2026-07-26 自 root tui/ 移入）
│   ├── file-agent/                   # 6 內建工具的檔案操作 agent
│   ├── greet-agent/                  # 內建工具 + greet tool
│   ├── log-agent-v2/                 # agent.WithListener + MiniMax 的 serialized scheduled log analyzer
│   ├── logdoctor-agent/              # MiniMax-only log listener；單一 watch command 增量掃描 ~/.config/*/logs/*
│   ├── skeleton-agent/               # wizard --print-go 樣板:cli.Main(agent.MustNew(cfg)) + stdinAgent 包裝
│   ├── demo-memory/                  # StateStore/WAL/checkpoint demo
│   ├── demo-middleware/              # middleware chain demo
│   └── demo-strategy/                # 6 reasoning strategy demo
├── docs/specs/                       # root SDK architecture/spec history
├── plans/                            # 進行中的落地計畫
└── tmp/                              # submodule 與 runtime symlink，不放 production logic
```

## 技術棧 (Tech Stack)

| 類別                      | 技術                                                | 現況                                                                                                                                   |
| ------------------------- | --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Language                  | Go `1.26.0`                                         | `go.work` 管理 10 個 module entries                                                                                                    |
| Root runtime              | Go stdlib、`github.com/bizshuk/gosdk v1.2.1`        | config/log/notify 等組合點在 root 或 sample                                                                                            |
| Auth module               | `viper` + stdlib                                    | Git submodule + 獨立 module；credential 機制、Resolver、active.json                                                                    |
| HTTP proxy                | `gin-gonic/gin v1.11.0`、`gosdk/mw`、`gosdk/router` | 獨立 module；`/v1` API、health/ping、localhost CORS、API key、per-IP rate limit                                                        |
| CLI/config                | `spf13/cobra v1.10.2`、`spf13/viper v1.20.1`        | auth/proxy module CLI、samples、root `provider` / `wizard` 子指令                                                                      |
| Config 序列化             | stdlib `encoding/json`、`gopkg.in/yaml.v3 v3.0.1`   | `agent/spec` 只用 JSON；YAML 在 `agent` 走 JSON tag 轉譯，不另立 tag                                                                   |
| Markdown frontmatter      | `github.com/adrg/frontmatter v0.2.0`                | `utils/frontmatter` wrapper：自動偵測 YAML/TOML/JSON delimiter,解碼後攤平為 `map[string]string`,序列值以 `,` join 保留舊 `List()` 行為 |
| State/schema              | `testify v1.11.1`、`invopop/jsonschema v0.14.0`     | table-driven tests、`core.Tool` RawMessage Call、反射式 JSON Schema                                                                    |
| IDs/telemetry             | `google/uuid v1.6.0`、OpenTelemetry `v1.44.0`       | request ID、transform warning/loss metrics                                                                                             |
| Anthropic adapter         | `anthropics/anthropic-sdk-go v1.50.2`               | 只在 `provider/anthropic` module 引入                                                                                                  |
| Google adapter            | `google.golang.org/genai v1.62.0`                   | 只在 `provider/google` module 引入                                                                                                     |
| OpenAI-compatible adapter | `net/http` + JSON + SSE                             | `provider/openaicompat` 不依賴 vendor SDK                                                                                              |
| MCP                       | `modelcontextprotocol/go-sdk v1.6.1`                | `mcp` 獨立 module，轉成 `tool.ToolSource`                                                                                              |
| Terminal UI               | Go stdlib only                                      | `sample/code-agent/tui`（zero-dep）；differential rendering、CSI 2026、不用 alternate screen；不屬 SDK 表面                            |

`core/` 不 import `gosdk`、Gin、任何 provider SDK；auth、proxy、provider 與 MCP 的重依賴藉由獨立 `go.mod` 隔離。root module 的直接外部依賴縮減為 `gosdk`、OTel(trace)、`cobra`/`viper`、`jsonschema`、`uuid` 已隨 proxy 遷出；root 仍非 stdlib-only（組合層在此）。

## 核心架構決策 (Core Decisions)

- `core` 是純狀態與 transition contract。`core.Decide(state, event)` 不做 I/O，只回傳下一個 `State` 與 `[]Instruction`；runtime 才執行 model、tool、approval、notify，並經 StateStore/WAL 自動持久化。
- `core` 不拆成 domain 子套件：`State`、`Event`、`Instruction`、model/tool contracts 會彼此交叉引用，硬拆只會製造 import cycle 或 root alias facade。domain 邊界改由一檔一職責表達；檔名直接對應公開詞彙，不再用 `input`、`effect`、`step`、`thinking`、`port` 等需先讀內容才能理解的名稱。
- `State` 的對話模型是 `Message{Role, Parts}`；`Part` 支援 text、reasoning、audio、image、tool use、tool result。reasoning 文字沿用 `Part.Text`，opaque continuation metadata 放 `ReasoningState`，不與 agent strategy 的 `ReasoningStyle` 混用。JSON tags `scratch` 與 `thinking_kind` 保留以相容舊 state。
- `Instruction` 是 tagged union，只保留有 production producer/consumer 的 `call_model`、`call_tool`、`request_approval`、`notify`、`done`；持久化由 runtime 的 StateStore/WAL lifecycle 負責，presentation 由 `core.EventSink` / `Engine.Emitter` 負責，不另立 `checkpoint` / `emit` command。
- 宣告式 `agent/spec` 只暴露 composition root 真正消費的設定；未接線的 `limits.max_wall_time`、`memory.compaction` 與只讓 JSON 生效的 `output.format` 已移除。Process deadline 以 `agent.WithTimeout` 注入，`memory/compaction` mechanism 由需要它的 caller 明確組裝，presentation 則由 frontend 以 `agent.WithSink` 或 `Engine.Sink` 接管。
- `runtime.Engine` 是 shell，維護 `core.Provider`、`ToolRegistry`、`StateStore`、`WriteAheadLog`、`Notifier` 與 middleware。`Middleware == nil` 代表 no-op；`preset.Default()` 與 `preset.Secure()` 由 composition root 明確選用。
- `config.DefaultMiddleware()` 順序為 `retry → timeout → budget → loopguard`；安全版本再加入 `sandbox → approval → spotlight → sanitizer`。工具輸出會被 spotlight 標為 untrusted，prompt injection 命中會被 sanitizer 改寫。
- WAL recovery 先載入 snapshot，再依 `LastInputSeq` replay Event；replay 不重新呼叫模型，避免 crash recovery 產生重複副作用。`memory/filestore` 使用 atomic state write + JSONL append。
- `reasoning/` 擁有 `DecisionRule`、built-in factory `NewRule` 與 registry-backed `NewDecide`，因為 rule construction 與 dispatcher 都是 strategy 詞彙的 consumer；`DecisionRule.Kind()` 與 registry key 直接用 `string`。六個 strategy（ThinkThenAct、PlanThenRun、RunThenReview、OneShotReasoning、LearnFromFailure、ChooseAgent）都是 phase FSM。`core` 只保留 `Decide` function type，所以 `runtime` 不必反向依賴具體 reasoning package；working memory 是 rule 與 runtime/middleware 的通訊介面。
- `core.Tool` 是唯一可執行工具契約（`Name`/`Spec`/`Call(json.RawMessage)`），`tool.Tool` 只是 alias，不另立平行介面。`ToolCall`/`ToolResult` 同時是 model chunk、transcript part、instruction/event 的 canonical payload，不再另立串流或訊息專用衍生 struct。`tool.CallWithRawMessage` 在每個 concrete tool 的 `Call` 內把 raw JSON 轉成 typed args、驗證 required fields、執行 typed business function，再把輸出轉回 `ToolResult`；`RegisterFunc` 沿用同一 converter。`builtin.Register` 接受 allowlist（空 = 全部）並以 all-or-nothing 語意註冊 Read/Write/Edit/Bash/Glob/Grep；`RegisterDefaults` 是全選 convenience wrapper。
- `core.ModelRequest` 同時是 provider request 與 `Instruction.CallModel` 的 canonical payload；`RequestID` 供 middleware/tracing 辨識，runtime 只在 request 未帶 tools 時補 registry list，其餘 `MaxTokens`/`StopReasons`/`Auth` 原樣轉送。`agent/spec.Subagents.MaxDepth` 直接注入 `skill.Spawner.MaxDepth`；未接 composition 的 telemetry config block 已移除，OTel 仍是可組合的 `middleware/observability`。
- `core.ModelResult.Parts` 是 provider 回應與 transcript 之間的 canonical ordered content；`NormalizeContent` 讓舊 provider 的 `Text` / `ToolCalls` 可合成 Parts，也讓新 provider 的 Parts 回填相容投影。reasoning 不併進 `Text`，避免 frontend 無意顯示 chain-of-thought；runtime 仍保留它供下一輪 provider continuation 使用。
- Provider capability boundary（2026-07-27）：`core.Provider` 是 runtime 消費端定義的最小 port，只含 blocking `Generate`；串流與 live catalog 分別是 optional `core.StreamProvider` / `core.ModelLister`。`provider.Adapter` 只組合 generate + stream；沒有 runtime consumer 的圖片生成留在 sibling optional `provider.ImageGenerator`，不反向擴肥 core。`provider.Entry` 單獨擁有 name、credential metadata、static catalog 與 model/image factories；`ID`/`Name`/`Models`/`AuthSchemes`/`CountTokens`/`Metadata` 不再是 constructed client 的 required methods。若日後有真實 token-count consumer，同樣以 optional capability 接回。
- Provider config pipeline（2026-07-27）：`provider.Options` 是 unresolved live input，唯一在 `Resolve(Entry.Metadata)` 查 env 並依 credential class 投影到 `ResolvedConfig.Auth`（OAuth → `Bearer`、API key → `APIKey`）；`ResolvedConfig{Model, BaseURL, Auth}` 是七個 adapter model/image factory 的唯一 construction input。adapter-local functional options/viper/env resolver 與 `register.go` converter 全移除；原本只放 env/endpoint 常數卻名為 `auth_api.go` 的七個檔案統一改為 `config.go`。`core.Auth` 不再含 `BaseURL`：endpoint 是 construction config，不允許透過 request credential 偽裝成可輪替認證。六個遠端 adapter 以 `Metadata.CredentialRequired` 在 registry boundary fail fast；Ollama 與外部 custom entry 維持預設 credential-optional。認證是跨 model/image API 的 request policy，不是獨立 capability；credential 優先序固定為`單次 request Auth → 明示 Options.APIKey → Decorator → env`。
- Provider wire sharing（2026-07-27）：共用的條件是 observable contract 相同，不是 struct 欄位相似。`provider/protocol/sse` 只共享通用 framing；Google / Ollama 由 `provider/openaichat_compat_test.go` 驗證 Chat Completions bytes/result/stream/failure semantics後使用 `provider/protocol/openaichat`；Google / Grok 由 `provider/openaiimage_compat_test.go` 驗證 image request bytes、URL/base64 response、usage、auth、HTTP/decode error 與 cancellation後使用 `provider/protocol/openaiimage`。其他 vendor wire DTO 保持 local。
- `mcp.Client` 只做 MCP `ListTools`/`CallTool` 與 `core.ToolSpec`/`ToolResult` 轉換，不把 MCP transport 混入 core 或 runtime。
- Harness ports（2026-07-19，借鏡 claude-code harness + pi 模組化）：`core` 新增 `Hooks`（`HookEvent`/`HookDecision`）與 `EventSink`（`StreamEvent`）兩個純資料 port；`runtime.Engine` 持有 nil 即 no-op 的 `Hooks`/`Sink` 欄位，`PreToolUse` block 會折成失敗 `ToolResult` 讓 model 看到拒絕、`PostToolUse` 的 `SystemNote` 追加為 system message。所有 harness package 只依賴 `core`，由 composition root 注入。
- `permission.Engine` 是兩軸決策（codex 式）：`Mode`（`default`/`acceptEdits`/`plan`/`bypassPermissions`）× rule specifier（`Bash(git:*)`、`Edit(src/**)`；自寫 `**` matcher，因 `filepath.Match` 的 `*` 不跨 `/`），優先序 `deny > ask > allow`；無 rule 命中時可注入 `Fallback`（如 `permission.DefaultApprovalPolicy` 的 autonomy grid）。
- Agent skeleton（2026-07-22）：`spec.Config` 是宣告式資料，`agent` 是組裝層。opt-in 有兩層——層 `1` feature 開關（block 是 pointer，`nil` = 關、`{}` = 開且用預設，剛好對應 JSON 的「缺 key / 空物件」），層 `2` variant 選擇（block 內具名字串）。`reasoning` 再多一層正交軸：`Reasoning.Enable` 決定註冊哪些 rule 進 `reasoning.NewDecide` 的 map，`Reasoning.Style` 決定這次 seed 哪個——未註冊的 style 會讓 `reasoning.NewDecide` 回 `NOTIFY error`，故 `spec.Validate` 早報。
- Tier 階梯（`oneshot`/`basic`/`standard`/`full`）是 block 集合的單調展開簡寫，展開後顯式 block 覆蓋（explicit wins）。`tier` 與 `reasoning` 正交，組合`不`視為衝突：`T0` 無工具 → provider 收不到 tool spec → `ModelResult.ToolCalls` 恆空 → `runtime/loop.go` 的 short-circuit 在 `Decide` 前收成 `COMPLETED`，任何 strategy 到 `T0` 都退化成一次 model call。`T0` 預設關持久化，否則 `agent/cli.OpenForCLI` 的空 appName 檢查會讓 `Name` 從「`T1+` 必填」變成「永遠必填」。
- `agent.Once` 不繞過 Engine：它用 `reasoning.OneShotReasoning` + 全 nil port 走同一條路。不得改寫成「每次回 `[CALL_MODEL, DONE]`」的 no-op `Decide`——那會破壞 `core.Decide` 的純函式不變式，retry/WAL replay 會重發 model call（理由見 `reasoning/one_shot.go` 型別註解）。
- Scheduled listener sample（2026-07-27）：`sample/log-agent-v2` 的 process 持續運作，但每個非空 batch 都建立新的 `TIER_ONESHOT` `agent.Agent`，讓 transcript、budget 與 `RunID` 不跨 batch 累積。排程器先等待 interval，才呼叫 `Reader.Next`；batch 經 `agent.WithListener` 在第一個 model request 前進入 steering queue，`agent.WithSink` 將 Markdown 寫 stdout、stream events 寫 stderr。只有 `agent.Run` 與輸出都成功才 atomic commit cursor；失敗 batch 在下一個 interval 重試。
- 注入用 `agent.Option`（`func(*builder) error`），不用 `Deps` struct：沿 repo 既有慣例（`app` + 7 個 provider adapter 共 `8` 處 `type Option func(*x)`）。`Option` 是 closure、不可列舉、只活在本 process；`spec.Choice` 是資料、可列舉、要跨序列化邊界寫進設定檔——兩者不可合併，只在 wizard 的 `--print-go` 輸出端相遇。
- `prompt` 擁有「這一輪送什麼進 context window」，`memory` 擁有「放不下時砍什麼」：前者是 policy、後者是 mechanism，合併會讓注入與裁切互相遞迴。`SLOT_SYSTEM` 內順序由不變到易變（persona `10` → context files `20` → skill 索引 `30` → env `40`），因為 prompt caching 要求前綴穩定；預算超限時從尾端整段丟，不切半。`skill` 不知道 `prompt` 存在，adapter 住在 `prompt/source`（sub-package，透過 `SkillProvider` interface 而非型別耦合）；context-file 載入已併入 `prompt.LoadContextFiles`（固定行為、無客製化縫）。
- Steering / follow-up queue（pi 式）：`Engine.Steer` 在下一次 Decide 前插入 user message；`Engine.FollowUp` 把「本應完成」轉為續跑（每次一則）。follow-up 續跑會清掉 `think_then_act.phase` 讓 FSM 回到 reason（與 loop 既有 pending_call seeding 同一 seam）；其他五個 rule 的通用 reset 慣例待補。
- Tool-call batch settlement（2026-07-24）：一個 assistant message 的 `N` 個 `tool_use` part，必須在下一次 `CALL_MODEL` 前對應到 `N` 個 `tool_result`，否則 Anthropic-format provider 回 `400` 且 model 把缺結果讀成「還在跑」。`runtime` 播種整批 `ToolCalls` 進 `think_then_act.pending_calls`（`ThinkThenAct` 的 dispatch phase 一次發 `N` 個 `CALL_TOOL`），未執行者（pause / hook block / budget skip）由 `settleSkipped`/`settleUnrun` 補上失敗 `tool_result`。`planning.decodeCalls` 依形狀解碼 working memory，避免 JSON round-trip 後 pending call 消失（crash 後 Resume 靜默完成的 bug）。
- Round / MaxToolCalls budget（2026-07-24）：`round` = 一次 `CALL_MODEL` 及其 tool call（使用者面），`Budget.MaxRounds` 上限、`UsedRounds` 在 `CALL_MODEL` 遞增；`turn` = `Decide` 次數（loopguard 用），兩者不混。`MaxToolCalls` 限單 round 批量：超限`整批 skip + settle` 並暫停於 `continue-gate`（`ToolCall == nil` 的 `PendingApproval`），approve → resume 讓 model 重讀重新規劃（不重發原批次），reject → `COMPLETED`。
- `agent.Interactive`（2026-07-24）：`NextRound(ctx, Pause) (Resume, error)` 是唯一互動縫，`PAUSE_APPROVAL`（含 continue-gate）與 `PAUSE_ROUND_END`（follow-up）共用。`agent.Run` 在 `safeRun` 後 loop 呼叫，`advance` 只走 `SubmitHumanDecision`（不再 `Resume`，避免 WAL replay 重複執行）、follow-up 走 `Steer`+重開。不實作 `Interactive` = 維持外部 verb 語意。收斂自三介面草案（`PauseHandler`/`ApprovalResolver`/`RejectionHandler`），計畫見 [`plans/2026-07-24-round-batch-and-interactive-seam.md`](plans/2026-07-24-round-batch-and-interactive-seam.md)。
- Agent lifecycle ownership（2026-07-27）：`Bootstrap` 是 engine 與 opening state 的唯一組裝 owner；`Run` 先驗證 `Runner` / `Host` / context，只呼叫一次 `Bootstrap`，不再以 `bindHost` 猜測並補回 persistence。原 optional `Preflighter` 已移除，避免 registry provider 每次 run 建構兩次；建構錯誤直接由 `Bootstrap` 回報。`agent.Run` 回傳 `error`，process exit code 僅由 `agent/cli` 擁有與轉譯。
- Agent public surface（2026-07-27）：`Parts` 只回傳 driver 真正消費的 `Engine` / `Sessions` / `Skills` / `Host` / `Cwd`；無 production consumer 的 `Prompt` 與展開後 `Config` 不再外洩。`Host` 是 process/runtime 資源的唯一名稱，過渡用 `AppConfig` alias 已移除。`spec.Output`、tier output default、wizard output stage 與 `buildSink` 一併移除；`agent/wire` 保留為可選的 `core.EventSink` adapter，不反向成為序列化設定的一部分。
- `session.Manager` 只加 lineage（meta sidecar：`ID`/`Parent`/`Title`/`Cwd`）與 fork（複製 state+WAL、改 RunID）；transcript 真相仍是 WAL JSONL。
- `wire` 是 `cli/` 的復活但有真 caller path：`runtime → core.EventSink → wire.Sink → JSONL`；Envelope 欄位是對外 API，需維持 JSON round-trip 穩定。
- `permission`/`session`/`wire` 收為 `agent/` 子套件（2026-07-26）：三者都只被 `agent` 組裝（`build.go` 的 approval / sessions / sink 三個 stage），流程由 `agent` 掌控——permission 決定「這次工具能不能跑」、session 決定「這次從哪個對話接續」、wire 決定「這次的回應怎麼吐出去」。採`子套件`而非併入 `agent` 檔案：三者各自帶完整詞彙（`Mode`/`Behavior`/`Rule`、`Meta`/`Node`/`Manager`、`Envelope`/`Sink`），攤平會得到 `agent.Mode`、`agent.Rule`、`agent.Meta` 這種在組裝層毫無指涉的通名，且與 `hook.Rule` 概念撞名。位置改變、`package` 名與符號皆不變，呼叫端只換 import path。三者仍`只 import core`，由 `go list -deps` 驗收——這是它們能待在 `agent/` 之下而不被組裝層污染的前提。
- `tui` 住在 `sample/code-agent/tui`（2026-07-26 自 root 移入，zero-dep 且不 import agentsdk）：caller 把 `core.StreamEvent` 轉成 component 更新；不用 alternate screen、CSI 2026 synchronized output、frame 未變零輸出。移動的理由是`誰需要它`——SDK 與 `agent` 組裝層都不需要終端機畫面，只有具體的 agent 實作需要，而 `code-agent` 是唯一 caller。留在 root 會讓 SDK 表面暗示「用這個 SDK 就得用這套 TUI」，實際上 renderer 是應用自己的選擇（另一個 agent 大可用 bubbletea 或什麼都不用）。zero-dep 鐵則不變，只是現在它是 sample 內部的一個 package。
- `provider.Entry` 是 provider discovery/config 的唯一 owner（2026-07-27）：每個 `register.go` 直接在 `Entry` literal 內宣告 `Name`、`Metadata`、`Catalog`、`New` 與 optional `NewImage`，constructed adapter 不再複製 identity 或 metadata。`var _ provider.Adapter = (*Provider)(nil)` 只證明 Generate + Stream；Google/Grok 另以 `var _ provider.ImageGenerator` 證明 image capability。CLI 的 static fallback 直接讀 `Entry.Catalog`，live path 才 type-assert `core.ModelLister`。這取代 2026-07-24 的 `Adapter.Metadata()` 雙真相設計；原設計背景仍見 [`plans/modular-frolicking-key.md`](plans/modular-frolicking-key.md)。
- auth 沉入 provider 之下 + `config/` 解體 (2026-07-26，Phase A/B/C1/C2 全落地)：計畫見 [`plans/2026-07-26-auth-under-provider-and-config-dissolution.md`](plans/2026-07-26-auth-under-provider-and-config-dissolution.md)。
    - `Phase A`：`config/provider.go` 的 `RefreshingProvider`（零呼叫者）先搬到 `provider/credential`，`AppConfig` 移除同樣零呼叫者的 `AuthStore`/`AuthDir`。`agent` 的傳遞依賴中 `auth` 自此消失；該零呼叫者 wrapper 於 2026-07-27 被 per-request decorator 完全取代並刪除。
    - `Phase B`：`config/` 三個檔案各有各的歸屬,目錄解散。`config/app.go` → `agent.AppConfig`/`OpenForCLI`（`agent.Run` 的 step 1 本來就在做這件事）;`config/default.go` → `middleware/preset`（preset 是 middleware 的組合,`preset → middleware` 的子套件方向不成環,原註解擔心的循環只存在於 `runtime → middleware`）。破壞性變更是 `Runner.Bootstrap` 的 `*config.AppConfig` → `*agent.AppConfig`,5 個 sample 同步。順帶解掉「五個東西都叫 config」的命名衝突。
    - `Phase C1`：`core.Auth` 補上 `Merge`/`Token`/`IsZero`,7 個 adapter 的裸 `apiKey`/`bearer`/`accountID` 欄位全部收斂為單一 `auth core.Auth`（`codex` 的 `accountID` 併入 `Headers`,因為它本來就是個 header,獨立欄位會讓 per-request 覆寫碰不到它）。新增 `provider.Decorator = func(ctx) (core.Auth, error)` 與 `WithDecorator`,經既有的 `core.ModelRequest.Auth` 逐次覆寫縫下傳。`型別定義在 provider 而非 auth` 是硬性約束:定義在 auth 會讓每個接受它的 adapter 都得 import auth,「adapter 不帶憑證機制也能編譯與測試」這個性質就沒了,而那正是當初複製 967 行的理由。`解析必須每 request 一次`而不是建構時一次:OAuth token 會中途過期,順帶涵蓋 Generate retry 與 SSE 重連,且輪替時不重建 adapter。
    - `Phase C2`：`provider/credential` 持有 `(name, kind) → auth route id` 對照表並委派 `auth/provider`,四個 adapter 的 in-tree OAuth acquisition 流程先由 `967` 收斂；2026-07-27 再移除零 production caller 的 `OAuthCredentials/NewWithOAuth` transport DTO/constructor，request-time 只剩 `core.Auth` 與 Anthropic vendor beta header。對照表無法省略,因為兩套詞彙連拼字都不同（`codex`→`openai`、`grok`→`xai`）,而且 agentsdk 的兩軸模型（`Model.Provider` + `Model.CredentialKind`）比 auth 的扁平 id 更適合設定檔——`anthropic_oauth` 這種 id 到此為止,不冒到 `spec.Model.Provider`。
    - 分層由 `go list -deps` 驗收:`agent` 與 `provider` registry 本體對 `auth` 皆為 `0`,只有 `provider/credential` 非零。
    - 未驗證項:兩份 anthropic OAuth 常數（in-tree 與 auth）曾在 `TOKEN_URL`/`REDIRECT_URI`/`SCOPES` 三處漂移,C2 一律以 `auth` 為準。in-tree 版本零呼叫者,故此決定不構成行為回歸,但 OAuth 登入流程本身`仍未經實跑驗證`。

- 分層修正三則 (2026-07-26)：`provider` 目錄下的 package 由 `registry` 改名為 `provider`,目錄與 package 名一致,移除全部 `registry "…/provider"` import alias。`core.DefaultProvider`（vendor 名 `"minimax"`）移出 `core` 成為 `provider.DEFAULT_NAME` —— 純狀態機不該因為換預設廠商而改動,而宣告層 `agent/spec` 本來就看不到哪些 adapter 被 link 進來,所以 `spec.Expand` 不再填 `Model.Provider`,空字串一路留到 `provider.Lookup` 才解析。`core.CredentialKind*` 保留在 `core` 但改為 `CREDENTIAL_KIND_*` 以符合全 repo 常數慣例；它們是 `agent/spec` 與 provider credential resolution 共用的 core-only vocabulary，不代表 runtime provider capability。
- prompt 內建 Source 歸位 + SkillSource 下降 (2026-07-26)：`PersonaSource`/`ContextFileSource`/`EnvSource`/`ReminderSource`（含 `gitBranch` shell-out）自 `agent/sources.go` 移入 `prompt/sources.go`。判準是「寫這個東西需不需要同時知道兩個 package 存在」——只需要 `prompt` + stdlib 的是 content 職責,屬於 `prompt`;`agent/sources.go` 因此只剩 `SkillSource`（唯一真正的跨套件配接,因為 `skill` 與 `prompt` 互不相見）與 `BuildSources` 的 name→Source dispatch,214 → 128 行。`prompt` 仍只 import `core`。同時把 skills / commands / agents 三處各自複製的「userDir 優先、projectDir 覆蓋」目錄走訪收斂成 `discoveryRoots(cfg, userDir, cwd, kind)`。接著把 `prompt/sources.go` 五個 Source（含 `agent.SkillSource` 與新增的 `SkillProvider` interface）一同移入 `prompt/source/` sub-package，每個 Source 一檔；`prompt/source` 不 import `skill` 也不 import `agent`，只透過 `SkillProvider` interface 收 `*skill.Registry`。`agent/sources.go` 因此只剩 `BuildSources` 的 name→Source dispatch（含 typed-nil `*skill.Registry` → true-nil interface 的轉換）、`promptUserDir`、`discoveryRoots`、`discoverSkills`。`prompt/source` 仍只 import `prompt` 與 stdlib。
- sample 兩類命名 (2026-07-26)：頂層 `sample/` 的目錄前綴即分類——`demo-*` 是手接單一 SDK 元件的展示（`demo-memory`/`demo-middleware`/`demo-strategy`,不經 `agent/`）,`*-agent` 是完整 agent。分類看「是什麼」不看「怎麼建的」,所以繞過 `agent/` 的 `logdoctor-agent` 仍歸完整 agent 一類。`provider/sample` 是 package-local provider API example，不屬於頂層 sample 分類，也不另立 module。

- `provider.Metadata` 拆 OAuthEnv / APIKeyEnv (2026-07-26)：原本以 `APIKeyEnv []string` 隱含 OAuth-first precedence,現拆為兩個明確欄位讓 `Options.Resolve` 區分 strict 模式。新增 `Options.CredentialKind` (`""` / `"auto"` / `"api_key"` / `"oauth"`)：空字串保留 OAuth-env-first precedence,strict 模式只查對應 env,缺失 → `provider.Options.Resolve` 回 error 並由 `provider.New` 包進既有錯誤格式。三層 (`provider cmd --credential-kind` flag / `agent/spec.Model.CredentialKind` YAML 欄位 / `provider.Options.CredentialKind` 內部選項) 共用同一組常數 `core.CREDENTIAL_KIND_AUTO/_APIKEY/_OAUTH`,確保 doc、schema、cmd、registry 不會各自漂移；常數留在 `core` 讓 `agent/spec` 維持只 import `core`。2026-07-27 起 `Resolve` 明確回 `ResolvedConfig`：OAuth env 進 `Auth.Bearer`、API key env 進 `Auth.APIKey`；Codex stored OAuth 由 `provider/credential.Source.Decorator()` 提供，不再有平行 constructor。設計見 [`plans/validated-meandering-rabbit.md`](plans/validated-meandering-rabbit.md) 與 [`docs/specs/2026-07-27-agent-sdk-contract-alignment.md`](docs/specs/2026-07-27-agent-sdk-contract-alignment.md)。

## 認證與 provider 決策 (Auth and Providers)

> 範圍：本節描述`外部 repo` [`github.com/bizshuk/auth`](https://github.com/BizShuk/auth) 的設計，不是本 repo 的目錄。本 repo 只透過 `provider/credential` 消費它的 `model`/`svc`/`provider`。

`auth` 只提供 mechanism：`Credential`、API key、OAuth authorization-code（PKCE 或 client secret）、device code（RFC 8628 + OIDC discovery）、service-account JWT → Google STS、callback、refresh、FileStore。`auth/provider/<name>/` 封裝單一家 provider，依賴 `auth` 與必要的 provider-specific config 套件；`auth/provider` registry 再統一掛載實作。

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
- `Credential.BaseURL` 可供 auth verify / proxy 使用；agentsdk adapter runtime 不把 endpoint 放進 `core.Auth`，必須由 `provider.Options.BaseURL` 在 construction time 明確指定。
- `auth.Resolver` 是共用的 credential 解析機制：active.json 選取 → 同 provider 字母序 fallback → 環境變數 fallback（`DefaultEnvironmentNames`，含 ollama/llmbox）→ 過期自動 refresh 並持久化。active.json 讀寫慣例由 `auth.LoadActiveNames`/`SaveActiveName` 統一。
- proxy 的 `upstream.CredentialResolver` 是 `auth.Resolver` 的 thin adapter，只把 `auth.UnavailableError` 映射為 `credential_unavailable` ProxyError；runtime 路徑由 `provider.Decorator` 承擔（`credential.Source.Decorator()`），每次 request 前 resolve/refresh 且`不`重建 adapter。舊的 rebuild-provider wrapper 已移除。
- auth CLI 的預設 namespace 是 `~/.config/agentsdk/data/auth`；proxy config 目前以 `agentSDK` namespace 載入（`proxy.APP_NAME`），若兩者要共用憑證，使用 `auth-dir` 明確指定同一目錄。

## Proxy pairwise 決策 (Current Proxy Architecture)

> 範圍：本節描述`外部 repo` `bizshuk/proxy` 的設計。本 repo 已無 `proxy/` 目錄、無 go.mod require、無任何 import；下列路徑與指令需在該 repo 內執行。

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

`main.go` 直接建立 Cobra root（binary 名稱 `agentsdk`，版本 `0.1.0`），掛載兩個子指令：`cmd.NewProviderCommand()` 的 `provider`（直接呼叫 `core.Provider.Generate` / `core.StreamProvider.Stream`，不走 Agent/Engine/harness），與 `cmd.NewWizardCommand()` 的 `wizard`（alias `w`）。Wizard 的設定詞彙來自 `spec`，provider 資料直接來自 `provider.Entries`/`Catalog`。`agent/cli.OpenForCLI(appName, level)` 為 sample 建立：

```text
~/.config/<appName>/
├── data/
│   ├── states/<runID>.json
│   ├── wal/<runID>.jsonl
│   └── auth/*.json
└── logs/<runID>.log
```

`agent.Run` 是可嵌入 lifecycle：input validation → wall-clock deadline → 單次 Bootstrap → panic-safe Engine.Run → optional OnComplete，失敗直接回傳 `error`。`agent/cli.Main` 負責 signal binding，`agent/cli.Run` 才將錯誤轉成 exit code。

Proxy defaults（`proxy/config.go`）：port `8317`、body limit `200 MB`、model timeout `120s`、stream timeout `600s`、count-tokens timeout `30s`、stats enabled、debug off；未設定 API key 時在記憶體產生 `sk-...`，設定持久化由上層 command 負責。

JSONL 對外 envelope (`cli/`) 於 2026-07-19 移除：原 9 種 type (`observation`、`assistant`、`tool_call`、`tool_result`、`approval_request`、`human_decision`、`checkpoint`、`result`、`error`) 無 production caller；外部 wire-format 需求改由 sample 自訂 codec 承接，或待新需求再決定是否以獨立 module 重啟。不得把內部 `State` 欄位直接當成穩定外部 API，新增欄位需維持 JSON round-trip。

## 模組對應 (Module Mapping)

| 領域                      | 套件 / 進入點                                                                                                                                                                                                                                                                                       |
| ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 宣告式設定                | `agentsdk/agent/spec`：`Config`、`Choice`、`Expand`（tier 展開）、`Validate`、`Prepare`（只 import `core` + 純 stdlib，2026-07-26 起連 `encoding/json` 都不 import）                                                                                                                                |
| agent 設定檔 I/O          | `agentsdk/utils/agentconfig`：`Decode`/`DecodeBytes`/`Encode`/`EncodeBytes`（2026-07-26 自 `agent/spec/load.go` 移入）、`LoadFile`/`SaveFile`/`Marshal`/`FormatOf`、`Format`/`FORMAT_YAML`/`FORMAT_JSON`（re-export 自 `utils/configfile`）；`LoadFile` = `configfile.ReadJSON` → `DecodeBytes`、`SaveFile` = `EncodeBytes` → `configfile.Write`                                                              |
| agent 組裝                | `agentsdk/agent`：`New`/`MustNew`/`Bootstrap`（實作 `agent.Runner`）、`Once`/`OnceStream`、`Option` 全部 `With*`、`BuildSources`、`Run`、`Interactive` |
| prompt 內容管理           | `agentsdk/prompt`：`Slot`（system/user/reminder）、`Section`、`Source`、`Builder.Seed`/`Turn`、`Static`、`PersonaSource`/`ContextFileSource`/`EnvSource`/`ReminderSource`                                                                                                                                                                                             |
| 狀態與 ports              | `agentsdk/core`：`State`/`Budget`、`Event`/`Observation`、`Instruction`/`Decide`、最小 `Provider`、optional `StreamProvider`/`ModelLister`、`Tool` 與 persistence/presentation ports；檔案依 domain 分組，package/API 不拆                                                                                                                                             |
| 推理策略                  | `agentsdk/reasoning`：`DecisionRule`、`NewRule` built-in factory、`NewDecide` dispatcher + 6 個 `New*` rule constructor                                                                                                                                                                                            |
| runtime                   | `agentsdk/runtime`：`NewEngine`、`Run`、`RunWithEvent`、`Resume`、`SubmitHumanDecision`                                                                                                                                                                                                             |
| tools/safety              | `agentsdk/tool`、`agentsdk/tool/builtin`：`Tool`、`CallWithRawMessage`、`NewRegistry`、`RegisterFunc`、allowlist-aware `Register`、`RegisterDefaults` |
| memory                    | `agentsdk/memory`、`memory/checkpoint`、`memory/filestore`                                                                                                                                                                                                                                          |
| lifecycle hooks           | `agentsdk/middleware/hook`：`NewRunner`、`Rule`、`Func`、`Command`（實作 `core.Hooks`；lifecycle event 為 fan-out + decision merge，handler 串以 middleware-style 連續執行，但 signature 仍獨立）                                                                                                                                                                       |
| permission                | `agentsdk/agent/permission`：`Engine`、`Rule`、`MatchSpec`（實作 `core.ApprovalPolicy`；只 import `core`）                                                                                                                                                                                          |
| session 管理              | `agentsdk/agent/session`：`NewManager`、`Begin`、`List`、`Latest`、`Fork`、`Tree`（只 import `core`）                                                                                                                                                                                               |
| context files             | `agentsdk/prompt`：`LoadContextFiles(cwd, userDir)`（AGENTS.md/CLAUDE.md 階層 + `@import`；2026-07-24 自 contextfile 併入，無 Loader 結構、無 config knob）                                                                                                                                                       |
| skills/commands/subagents | `agentsdk/skill`：`NewRegistry` 統一索引三類定義；`DiscoverSkills`／`DiscoverCommands`／`DiscoverSubagents` 採相同的 later-wins 覆寫規則，並以 `Skills`／`Commands`／`Subagents` 回傳排序結果；另提供 `SubAgent`、`Body`、`ExpandCommand`、`RenderTemplate`、`ParseDef`、`NewSpawner`、`Depth`／`WithDepth`。源碼分為 `skill.go`／`command.go`／`registry.go`／`subagent.go` 四檔 |
| headless wire             | `agentsdk/agent/wire`：`Envelope`、`NewEncoder`/`NewDecoder`、`NewSink`、`ReadRequest`/`WriteResponse`、`FormatStream`                                                                                                                                                                                    |
| terminal UI               | `agentsdk/sample/code-agent/tui`（zero-dep，非 SDK 表面）：`Renderer`、`Component`、`Terminal`、`VisibleWidth`/`WrapText`                                                                                                                                                                                             |
| dependency graph          | external CLI `github.com/bizshuk/go-dependency-analysis`：Go tooling facts + JSON policy heuristics；不加入本 workspace、不被本 repo import                                                                                                                                                         |
| middleware                | `agentsdk/middleware`、`harness`、`loopguard`、`security`、`observability`                                                                                                                                                                                                                          |
| agent lifecycle           | `agentsdk/agent`：`Run`、`Host`、`Interactive`、`Pause`/`Resume`、`WithRoundTimeout`；`agentsdk/agent/cli`：`Main`/`Run`、`OpenForCLI`/`MustOpenForCLI` |
| middleware preset         | `agentsdk/middleware/preset`：`Default()`（retry→timeout→budget→loopguard）、`Secure(sandbox, approval)`（再加 sandbox→approval→spotlight→sanitizer） |
| credential                | `agentsdk/provider/credential`：`RouteID`/`Kinds`/`Names`、`NewSource`/`NewAutoSource`/`Source.Decorator()`、`Login`；唯一 import `bizshuk/auth` 之處 |
| provider registry         | `agentsdk/provider`（package `provider`，非 `registry`）：`Entry` 單獨擁有 name / metadata / static catalog / model+image factories；`Names`/`Entries`/`Lookup`/`Catalog`/`Capabilities`/`New`/`NewImage`/`Options.Resolve`/`ResolvedConfig`/`DEFAULT_NAME`；`env` 查詢以 `LookupEnv` 注入 |
| root CLI subcommands      | `agentsdk/cmd`：`NewWizardCommand`（`wizard`/`w` 設定產生器）、`NewProviderCommand`（root cobra `provider` smoke-test CLI；打 `core.Provider.Generate` / `core.StreamProvider.Stream` 不走 Engine；`--list-models` 優先打 live `core.ModelLister`,失敗 fallback `Entry.Catalog`） |
| authentication            | `github.com/bizshuk/auth/{model,svc,utils,provider}`（Git submodule + 獨立 module，root 有 `auth` binary main）：`Login`、`For`、`FileStore`、`NewResolver`                                                                                                                                         |
| proxy                     | `agentsdk/proxy/handlers`、`agentsdk/proxy/config`、`agentsdk/proxy/model`、`agentsdk/proxy/svc/{route,transform,upstream}`（獨立 module，root 有 `proxy` binary main）：`handlers.New`、`config.LoadConfig`、`model.Format`、`svc/route.Router`                                                    |
| JSONL                     | (已移除 2026-07-19) 原 `agentsdk/cli`：`Envelope`、`JSONLCodec`                                                                                                                                                                                                                                     |
| MCP                       | (已移除 2026-07-19) 原 `agentsdk/mcp` 獨立 module：`mcp.NewClient` 實作 `tool.ToolSource`，目前無 caller；若日後需要 MCP，重新以獨立 module 落地並補上 wiring                                                                                                                                       |
| provider adapters         | `agentsdk/provider/{anthropic,google,minimax,grok,ollama,codex,antigravity}`：各實作 `provider.Adapter`（`core.Provider` + `core.StreamProvider`）；Google/Grok 另實作 `provider.ImageGenerator`（`POST /images/generations`）；除 codex/antigravity 外皆另實作 optional `core.ModelLister`。identity / credential metadata / factories / static catalog 只存在於各自 `register.go` 的 `Entry` literal |
| video utilities           | (已移除 2026-07-19) 原 `github.com/bizshuk/video-utils`（外部 module，獨立 git repo）：`audio`、`frames`、`subtitles`、`ffmpegutil`、`cmd.NewCommand`。root 不再依賴,不再列入模組對應表                                                                                                             |

## 開發與驗證 (Development and Verification)

前置需求：Go `1.26+`；使用 provider adapter 時依該 module 的 API key/environment。

```bash
cd /Users/shuk/projects/ai/agentSDK
go work sync
go mod download
go build ./...
go test ./... -count=1 -timeout=120s
go-dependency-analysis --workspace /Users/shuk/projects/ai/agentSDK/go.work --format text
go-dependency-analysis --workspace /Users/shuk/projects/ai/agentSDK/go.work --format json --json-indent='  '
go-dependency-analysis --workspace /Users/shuk/projects/ai/agentSDK/go.work --format mermaid
go-dependency-analysis --workspace /Users/shuk/projects/ai/agentSDK/go.work \
  --policy /Users/shuk/projects/go-dependency-analysis/examples/agentsdk.json
```

Analyzer 的 `go-tool-fact` 來自當次 Go toolchain/build context；`policy-heuristic` 才是 layer/heavy dependency 建議。`unused-direct-candidate` 必須先檢查 tests、build tags、platform files、generated code 與 tools，不能直接刪 require。完整 flags 與限制見獨立 repo `/Users/shuk/projects/go-dependency-analysis/README.md`。

驗證所有 workspace modules：

```bash
# provider/* 已併回 root module；auth/proxy 是獨立 repo，不在 go.work 內。
for mod in . sample/code-agent sample/file-agent sample/greet-agent sample/log-agent-v2 sample/logdoctor-agent \
  sample/demo-memory sample/demo-middleware sample/skeleton-agent sample/demo-strategy; do
  (cd "$mod" && go build ./... && go test ./... -count=1 -timeout=120s)
done

# 依賴紀律：兩個宣告層 package 只准看到 core
go list -deps ./agent/spec | grep agentsdk   # 只該有 core 與 agent/spec
go list -deps ./prompt     | grep agentsdk   # 只該有 core 與 prompt 與 prompt/source
go list -deps ./prompt/source | grep agentsdk # 只該有 prompt 與 prompt/source（不該出現 skill/agent/core 在 production code）

# agent/ 之下的三個 harness 子套件仍只准看到 core（位置在 agent/ 底下不代表可以往回依賴 agent）
for p in permission session wire; do go list -deps ./agent/$p | grep agentsdk; done  # 每個只該有 core 與自己

# core 不得知道任何 vendor 名稱（預設 provider 住在 provider.DEFAULT_NAME）
grep -rn 'minimax\|anthropic\|openai\|google\|ollama\|grok' core/*.go

# auth 只准被 provider/credential 看見（2026-07-26 起）
go list -deps ./agent               | grep bizshuk/auth   # 必須為空
go list -deps ./provider            | grep bizshuk/auth   # 必須為空
go list -deps ./provider/anthropic  | grep bizshuk/auth   # 必須為空（任一 adapter 皆同）
go list -deps ./provider/credential | grep bizshuk/auth   # 必須非空——唯一允許處

# config/ 已解體
test ! -d config && grep -rn "agentsdk/config" --include=*.go .   # 兩者皆須無輸出
```

`provider` 子指令 smoke-test（不打 Agent，直接打 core.Provider）：

```bash
cd /Users/shuk/projects/ai/agentSDK
go run . provider --list-providers
go run . provider --list-models --provider minimax
go run . provider "ping" --provider minimax
go run . provider --stream "say hi in one word" --provider minimax
go run . provider "summarize this repo" --provider anthropic --model claude-sonnet-5
go run . provider "ping" --provider minimax --json | jq
```

`wizard` 子指令（產生 `agent.Config` 設定檔，不打 provider、不驗憑證）：

```bash
cd /Users/shuk/projects/ai/agentSDK
go run . w                                  # 互動：逐階段問，Enter 收預設，寫 ./agent.yaml
go run . w -y --tier full -o -              # 非互動：全採預設，輸出 stdout
go run . w -y --tier oneshot -o agent.json  # 副檔名決定格式（.json → JSON，其餘 → YAML）
go run . w --edit agent.yaml                # 以既有設定當預設值逐階段確認（round-trip 無損）
go run . w -o - --print-go                  # 額外印出等價的 Go literal
go run . w --list reasoning.style           # 列出單一欄位的選項（來自 spec/choice.go）
```

常用本地流程：

```bash
# Root binary (go run .) 掛載 `provider` smoke-test 與 `wizard` 設定產生器
go run . provider --list-providers                          # 列出已註冊 provider
go run . provider --list-models --provider minimax          # 列出 provider catalog
go run . provider "ping" --provider minimax                  # 單輪 prompt,直接打 core.Provider
go run . provider --stream "stream me a haiku" --provider anthropic
go run . provider "summarize X" --provider anthropic --model claude-sonnet-5

# auth 與 proxy CLI 已移出本 repo，需在各自的 repo clone 內執行
#   (bizshuk/auth)  go run . login --provider anthropic / list / verify --all
#   (bizshuk/proxy) go run .                 # 啟動 LLM protocol proxy server

export MINIMAX_API_KEY=...
go run ./sample/log-agent-v2 --interval 1m

cd sample/logdoctor-agent
export MINIMAX_API_KEY=...
go run . watch

cd sample/code-agent
go run . --fake -p "看看這個專案"        # print 模式（進度走 stderr）
go run . --fake --json -p "test"        # stream-json envelope（wire）
go run . --fake                          # 互動 TUI（sample/code-agent/tui；執行中輸入 = Steer）
go run . --fake --sessions               # 列出本目錄 sessions；-c / -r / --fork 續跑

# 真實 provider：--provider 選 adapter，各自讀自己的 env（--fake 拿掉）
export MINIMAX_API_KEY=...                # minimax adapter 預設就讀這個
go run . -p "explore this repo"           # 預設 --provider minimax、model=MiniMax-M3
go run .                                   # 互動 TUI 打真實 model
go run . --provider anthropic -p "..."    # 改讀 ANTHROPIC_API_KEY（含 minimax gateway：--base-url）
```

`code-agent` 的 provider 選擇：`--provider minimax`（預設，讀 `MINIMAX_API_KEY`/`MINIMAX_BASE_URL`，`provider/minimax` stdlib adapter）或 `--provider anthropic`（讀 `ANTHROPIC_API_KEY`）；`--model` 留空用 adapter flagship 預設；`--api-key`/`--base-url` 為顯式覆寫。

`sample/log-agent-v2` 固定使用 MiniMax（`MINIMAX_API_KEY`），沒有 provider selector、fake mode、tools、approval 或 session UI。它先等待 interval 再掃描，並以 `agent.New` / `agent.Run` / `agent.WithListener` / `agent.WithSink` 展示完整 agent lifecycle；每個 batch 最多 `1 MiB`，cursor 位於 `~/.config/log-agent-v2/data/log-cursor.json`。`sample/logdoctor-agent` 則是比較用的單一 `watch` command，以 `agent.OnceStream` 直接分析 chunk；啟動時立即掃描，之後預設每分鐘執行。兩者皆將 Markdown 寫 stdout、`core.StreamEvent` 寫 stderr。`sample/file-agent` 與 `sample/greet-agent` 使用 Anthropic-compatible adapter 與 `preset.Secure`。

`sample/skeleton-agent` 是 `cmd/agent/wizard.go::goLiteral` 輸出範本逐字對應的單檔 sample:沒有 cobra、沒有四種 dispatch mode、不需要 `*Parts.Sessions`/`*Parts.Skills`。差別在 main 比 code-agent 少 `~260` 行,只為了展示 wizard `--print-go` 模板可直接落地;唯一的 12 行 `stdinAgent` 包裝是為了把 stdin 內容塞進 Bootstrap 回傳的 opening state。對比 shape 見 `sample/skeleton-agent/README.md`。

## 目前狀態與待辦 (Status and Open Items)

| 項目                                                                                                                           | 狀態                                                                                                                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| M1 核心範式與 sample                                                                                                           | 完成                                                                                                                                                                   |
| M2 state/WAL/checkpoint/retry/timeout/loopguard                                                                                | 完成                                                                                                                                                                   |
| M3 tool schema、sandbox、MCP、spotlight、sanitizer、tracing                                                                    | 完成                                                                                                                                                                   |
| M4 HITL、security wiring、三個 provider adapter                                                                                | 完成                                                                                                                                                                   |
| M5 built-in tools、sample wiring、`app` lifecycle                                                                              | 完成                                                                                                                                                                   |
| M6 auth mechanism、9 provider ids、auth CLI                                                                                    | 完成；憑證解析走 `provider.Decorator`（每 request 解析、不重建 adapter），舊 rebuild-provider 路徑已移除                                                             |
| Proxy 3×3 pairwise cutover 與安全 hardening                                                                                    | 完成（現行 branch）                                                                                                                                                    |
| 四來源 37 entity wire-format catalog                                                                                           | 完成，見 `proxy/docs/specs/format/README.md`（外部 repo）                                                                                                              |
| module 拆分：`auth`、`proxy` 獨立 module；`config` 解體；`perception/` 刪除                                                    | 完成，見 [`plans/2026-07-18-architecture-module-split-roadmap.md`](plans/2026-07-18-architecture-module-split-roadmap.md)                                              |
| Agent skeleton：`agent/spec`（宣告）+ `agent`（組裝）+ `prompt`（content management）+ `provider`（registry）+ `wizard` 子指令 | 完成（`M1`–`M7` 全落地，`compose.go` `333` → `101` 行）；計畫見 [`plans/2026-07-22-agent-skeleton-config-opt-in.md`](plans/2026-07-22-agent-skeleton-config-opt-in.md) |
| Harness/UX skeleton：`middleware/hook`/`agent/permission`/`agent/session`/`agent/wire`/`skill` + `sample/code-agent/tui` + steering queue（`contextfile` 於 2026-07-24 併入 `prompt`；permission/session/wire 於 2026-07-26 收為 `agent/` 子套件）           | skeleton 完成（全部含測試），細節項見 `README.todo`；計畫見 [`plans/2026-07-19-harness-ux-modularization.md`](plans/2026-07-19-harness-ux-modularization.md)           |

目前明確未完成或刻意保留：

- Anthropic/Google provider 的 `Stream` 目前以 `Generate` 結果折成 chunk；只有 `openaicompat` 與 proxy path 使用原生/完整 SSE 轉換。
- `/admin/*` 仍是未實作 placeholder；不要在文件或 client 中當成穩定 API。
- release tag 順序：`auth` → `proxy` → root（replace 指向相對路徑，只在本 repo workspace 生效；samples 於 repo 外單獨 build 需等 tag）。
- credential 儲存改 env placeholder（`README.todo` 新項）尚未設計，動工前先開 plan。
- Harness skeleton 的刻意保留：`tui` 尚無 Editor/raw-mode 輸入與 Markdown component（`ProcessTerminal.Size` 暫讀 `COLUMNS`，互動輸入走 cooked-mode line input）；streaming 仍是 folded events（`STREAM_MESSAGE_DELTA` 保留位）；hook/permission 的 settings 檔載入層未做。`sample/code-agent` 已落地為 tui 的第一個真 caller（互動 TUI + `-p` print + `--json` wire + session flags + `.agentsdk/` skills/commands/agents 探索）；composition 已於 2026-07-22 改由 `agent` 承擔，`cmd/compose.go` 只剩 flag → `agent.Config` 映射（`101` 行），應用自身的政策與 session 選取分別移到 `cmd/safety.go` 與 `cmd/session.go`。完整清單見 `README.todo` 的 Harness/UX 段。

## 慣例與注意事項 (Conventions and Caveats)

- 常數使用 `SCREAMING_SNAKE_CASE`，變數、函式、型別使用 Go `MixedCaps`；package 名稱使用單字。
- 錯誤以 `fmt.Errorf("...: %w", err)` wrap；公開 error 不帶 credential、authorization、prompt 或未清理 upstream body。
- 測試採 table-driven + `t.Run`，`testify/assert` 與 `testify/require` 並用；`utils/testutil` 只可被測試使用。
- `core.Decide`、reasoning rules、transform pair 不得直接做 I/O、讀 credential 或建立 HTTP request；這些責任分別屬於 runtime、upstream 與 auth。
- `sample/logdoctor-agent/core` 與 `agentsdk/core` 是不同 module path；import 時使用 `domain` / `sdkcore` alias。
- `proxy/docs/specs/2026-07-16-client-llm-adaptor.md` 是 legacy historical design；修改 proxy 時以 pairwise spec、現行 `proxy/` code 與測試為準。
- 修改 package tree、module、路由或 protocol contract 後，必須同步本檔；業務範疇變更才同步 `README.md`。格式 catalog 的 entity/來源異動則同步 `proxy/docs/specs/format/README.md`（外部 repo）。
