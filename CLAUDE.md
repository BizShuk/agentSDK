# CLAUDE.md — agentsdk 技術脈絡 (Technical Context)

`agentsdk` 是 Go Agentic Loop SDK、provider adapter、認證整合與範例程式的 workspace。
本文只記錄目前程式碼採用的邊界與決策；歷史變更見
[`docs/CHANGELOG.md`](docs/CHANGELOG.md)，未完成工作見 [`README.todo`](README.todo)。

## 技術基準 (Current Baseline)

- 語言與 workspace：Go `1.26.0`、`go.work`，共 `10` 個 module entries（root + `9` 個
  sample modules）。`auth` 與 `proxy` 是外部 modules，不列入本 workspace。
  Standalone dependency analyzer 位於 `~/projects/go-dependency-analysis`。
- root module：`github.com/bizshuk/agentsdk`，內容為 SDK 核心群（core/reasoning/tool/memory/middleware/runtime）、harness 群（middleware/hook、agent/permission、agent/session、agent/wire、skill、prompt）、組合層 `agent`、process host `agent/cli` 與 root CLI 子指令。`agent/spec` 是只 import `core` 的宣告層；`agent` 才組裝 reasoning/provider/harness。`core/` 保持 stdlib only。`auth` 只被 `provider/credential` import；`proxy` 不在 root module。
- `core/` 依 domain 分檔但維持單一 package：run state、transition、model boundary、
  tool/HITL 與 runtime ports。`core.Decide` 是純 transition contract；
  `DecisionRule` / `NewDecide` 由 `reasoning/` 擁有。
- `agent/` 以七階段 composition 組裝 Engine；`agent.go` 集中公開契約，
  `options.go` 集中注入。`agent/spec` 只依賴 `core`；presentation 由 frontend 透過
  `WithSink` 或 `Engine.Sink` 接管。
- Vocabulary ownership：`tool/builtin.Register` 擁有 tool registration，
  `reasoning.NewRule` 擁有 reasoning styles，`core.ParseAutonomyLevel` 擁有 autonomy
  parsing。`agent/build.go` 只組裝 owner API，injected tool/rule 維持 later-wins。
- Provider protocol codecs：`provider/protocol/sse` 只處理完整 SSE framing，不解讀
  provider terminal semantics；Google／Ollama 共用 `openaichat`，Google／Grok 共用
  `openaiimage`，其餘 vendor payload DTO 保持 local。
- Provider image capability：`provider.ImageGenerator` 留在 provider layer；
  `Entry.NewImage` / `provider.NewImage` 是一致建構路徑，unsupported adapter 回 typed
  `ErrUnsupportedCapability`。成功 response 上限 `128 MiB`，error body 上限 `1 MiB`，
  details 上限 `16 KiB`。
- Reasoning content boundary：`core.Part` 以 `PART_KIND_REASONING` 表示可攜 reasoning
  文字，`ReasoningState` 保存 opaque continuation metadata。`ModelResult.Parts` 是有序
  canonical assistant content；無法表示 metadata 的 wire path 必須明確報錯。
- `provider/sample` 是 root module 內的 package-local executable；`--list` 產生
  provider × chat/image/audio × auth-env matrix。Chat/image 分別走 `provider.New` /
  `NewImage`；audio 在沒有 production contract 時回 typed unsupported error。
- 目前 proxy 架構：`protocol → route → transform → upstream`，三種 client wire format 的 `3×3` directed pair 已接上 handler。
- 來源與規格：現行 pairwise 決策見 `proxy/docs/specs/2026-07-16-pairwise-agent-provider-transform.md`（外部 repo）；四個來源的 wire-format 盤點見 `proxy/docs/specs/format/README.md`（外部 repo）。
- 外部依賴：`auth` 是外部 module（`github.com/bizshuk/auth`，go.mod require，非 submodule），只被 `provider/credential`（`model`/`svc`/`provider`）使用——這是全 repo 唯一允許 import 它的 package，由 `go list -deps` 驗收；`proxy` 已無任何殘留（無目錄、無 require、無 import）。`tmp/auth2api` 與 `tmp/cliproxyapi` 僅供格式研究，不是 runtime dependency。

## 專案結構 (Project Structure)

```text
agentsdk/
├── README.md                         # 業務範疇與使用者導覽
├── CLAUDE.md                         # 技術脈絡與架構決策（本檔）
├── README.todo                       # 尚未完成的工作
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
│   ├── permission/                   # core.ApprovalPolicy 實作：Mode × allow/ask/deny specifier rules（只 import core）
│   ├── session/                      # StateStore/WAL 之上的 session 管理：list/resume/fork/tree + meta sidecar（只 import core）
│   └── wire/                         # headless envelope：stream-json/RPC/print（core.EventSink adapter；只 import core）
├── prompt/                           # content management：Slot(system/user/reminder)、Source、Builder、LoadContextFiles
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
├── middleware/                       # chain、retry/timeout/budget/loopguard、安全與 OTel tracing；preset/ 與 hook/ 子包
├── memory/                           # context window、compactor、checkpoint、JSON state/WAL
├── runtime/                          # Engine：dispatch Instruction、fold Event、Run/Resume/HITL
├── provider/                         # registry、credential bridge、protocol codecs 與七個 adapters
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
│   ├── anthropic/                    # anthropic-sdk-go adapter
│   ├── antigravity/                  # adapter：Google Cloud Antigravity OAuth
│   ├── codex/                        # adapter：OpenAI Codex OAuth
│   ├── google/                       # stdlib HTTP adapter
│   ├── grok/                         # adapter：xAI Grok
│   ├── minimax/                      # adapter：MiniMax stdlib HTTP/SSE
│   └── ollama/                       # adapter：本地 Ollama endpoint
├── sample/                           # demo-* 是單一元件展示，*-agent 是完整 agent
│   ├── code-agent/                   # 全 harness 組合 CLI：tui 互動 / -p print / --json（wire）+ session flags；compose.go 用 agent 宣告（333→101 行）
│   │   └── tui/                      # zero-dep differential renderer、ANSI 工具、Component/Terminal 抽象
│   ├── file-agent/                   # 6 內建工具的檔案操作 agent
│   ├── greet-agent/                  # 內建工具 + greet tool
│   ├── log-agent-v2/                 # agent.WithListener + MiniMax 的 serialized scheduled log analyzer
│   ├── logdoctor-agent/              # MiniMax-only log listener；單一 watch command 增量掃描 ~/.config/*/logs/*
│   ├── skeleton-agent/               # wizard --print-go 樣板:cli.Main(agent.MustNew(cfg)) + stdinAgent 包裝
│   ├── demo-memory/                  # StateStore/WAL/checkpoint demo
│   ├── demo-middleware/              # middleware chain demo
│   └── demo-strategy/                # 6 reasoning strategy demo
├── docs/
│   ├── CHANGELOG.md                  # 歷史變更與完成項目
│   ├── specs/                        # root SDK architecture/spec history
│   ├── memory/                       # retrospective 與來源研究
│   └── tutorials/                    # 教學文件
├── plans/                            # 進行中的落地計畫
└── tmp/                              # submodule 與 runtime symlink，不放 production logic
```

## 技術棧 (Tech Stack)

| 類別                      | 技術                                                | 現況                                                                                                                                   |
| ------------------------- | --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Language                  | Go `1.26.0`                                         | `go.work` 管理 10 個 module entries                                                                                                    |
| Root runtime              | Go stdlib、`github.com/bizshuk/gosdk v1.2.1`        | config/log/notify 等組合點在 root 或 sample                                                                                            |
| External auth module      | `github.com/bizshuk/auth`                           | `go.mod` dependency；只由 `provider/credential` import                                                                                 |
| External HTTP proxy       | `gin-gonic/gin`、`gosdk/mw`、`gosdk/router`         | `github.com/bizshuk/proxy` 獨立 repo；不在本 workspace                                                                                 |
| CLI/config                | `spf13/cobra v1.10.2`、`spf13/viper v1.20.1`        | auth/proxy module CLI、samples、root `provider` / `wizard` 子指令                                                                      |
| Config 序列化             | stdlib `encoding/json`、`gopkg.in/yaml.v3 v3.0.1`   | `agent/spec` 只用 JSON；YAML 在 `agent` 走 JSON tag 轉譯，不另立 tag                                                                   |
| Markdown frontmatter      | `github.com/adrg/frontmatter v0.2.0`                | `utils/frontmatter` wrapper：自動偵測 YAML/TOML/JSON delimiter,解碼後攤平為 `map[string]string`,序列值以 `,` join 保留舊 `List()` 行為 |
| State/schema              | `testify v1.11.1`、`invopop/jsonschema v0.14.0`     | table-driven tests、`core.Tool` RawMessage Call、反射式 JSON Schema                                                                    |
| IDs/telemetry             | `google/uuid v1.6.0`、OpenTelemetry `v1.44.0`       | request ID、transform warning/loss metrics                                                                                             |
| Anthropic adapter         | `anthropics/anthropic-sdk-go v1.50.2`               | 只由 `provider/anthropic` package 引入                                                                                                 |
| Google adapter            | Go stdlib `net/http`                                | `provider/google` 使用 shared OpenAI-compatible codecs                                                                                 |
| Shared protocol codecs    | `net/http` + JSON + SSE                             | `provider/protocol/{sse,openaichat,openaiimage}`，不承接 vendor terminal semantics                                                     |
| Terminal UI               | Go stdlib only                                      | `sample/code-agent/tui`（zero-dep）；differential rendering、CSI 2026、不用 alternate screen；不屬 SDK 表面                            |

`core/` 不 import `gosdk`、Gin、任何 provider SDK；外部 auth 只由
`provider/credential` import，proxy 完全位於外部 repo。Root module 仍非 stdlib-only，
因為 composition、CLI、provider adapters 與 observability 都在此。

## 核心架構決策 (Core Decisions)

- `core` 是純狀態與 transition contract。`core.Decide(state, event)` 不做 I/O，只回傳下一個 `State` 與 `[]Instruction`；runtime 才執行 model、tool、approval、notify，並經 StateStore/WAL 自動持久化。
- `core` 不拆成 domain 子套件：`State`、`Event`、`Instruction`、model/tool contracts 會彼此交叉引用，硬拆只會製造 import cycle 或 root alias facade。domain 邊界改由一檔一職責表達；檔名直接對應公開詞彙，不再用 `input`、`effect`、`step`、`thinking`、`port` 等需先讀內容才能理解的名稱。
- `State` 的對話模型是 `Message{Role, Parts}`；`Part` 支援 text、reasoning、audio、image、tool use、tool result。reasoning 文字沿用 `Part.Text`，opaque continuation metadata 放 `ReasoningState`，不與 agent strategy 的 `ReasoningStyle` 混用。JSON tags `scratch` 與 `thinking_kind` 保留以相容舊 state。
- `Instruction` 是 tagged union，只保留有 production producer/consumer 的 `call_model`、`call_tool`、`request_approval`、`notify`、`done`；持久化由 runtime 的 StateStore/WAL lifecycle 負責，presentation 由 `core.EventSink` / `Engine.Emitter` 負責，不另立 `checkpoint` / `emit` command。
- 宣告式 `agent/spec` 只暴露 composition root 真正消費的設定；未接線的 `limits.max_wall_time`、`memory.compaction` 與只讓 JSON 生效的 `output.format` 已移除。Process deadline 以 `agent.WithTimeout` 注入，`memory/compaction` mechanism 由需要它的 caller 明確組裝，presentation 則由 frontend 以 `agent.WithSink` 或 `Engine.Sink` 接管。
- `runtime.Engine` 是 shell，維護 `core.Provider`、`ToolRegistry`、`StateStore`、`WriteAheadLog`、`Notifier` 與 middleware。`Middleware == nil` 代表 no-op；`preset.Default()` 與 `preset.Secure()` 由 composition root 明確選用。
- `preset.Default()` 順序為 `retry → timeout → budget → loopguard`；
  `preset.Secure()` 再加入 `sandbox → approval → spotlight → sanitizer`。工具輸出會被
  spotlight 標為 untrusted，prompt injection 命中會被 sanitizer 改寫。
- WAL recovery 先載入 snapshot，再依 `LastInputSeq` replay Event；replay 不重新呼叫模型，避免 crash recovery 產生重複副作用。`memory/filestore` 使用 atomic state write + JSONL append。
- `reasoning/` 擁有 `DecisionRule`、built-in factory `NewRule` 與 registry-backed `NewDecide`，因為 rule construction 與 dispatcher 都是 strategy 詞彙的 consumer；`DecisionRule.Kind()` 與 registry key 直接用 `string`。六個 strategy（ThinkThenAct、PlanThenRun、RunThenReview、OneShotReasoning、LearnFromFailure、ChooseAgent）都是 phase FSM。`core` 只保留 `Decide` function type，所以 `runtime` 不必反向依賴具體 reasoning package；working memory 是 rule 與 runtime/middleware 的通訊介面。
- `core.Tool` 是唯一可執行工具契約（`Name`/`Spec`/`Call(json.RawMessage)`），`tool.Tool` 只是 alias，不另立平行介面。`ToolCall`/`ToolResult` 同時是 model chunk、transcript part、instruction/event 的 canonical payload，不再另立串流或訊息專用衍生 struct。`tool.CallWithRawMessage` 在每個 concrete tool 的 `Call` 內把 raw JSON 轉成 typed args、驗證 required fields、執行 typed business function，再把輸出轉回 `ToolResult`；`RegisterFunc` 沿用同一 converter。`builtin.Register` 接受 allowlist（空 = 全部）並以 all-or-nothing 語意註冊 Read/Write/Edit/Bash/Glob/Grep；`RegisterDefaults` 是全選 convenience wrapper。
- `core.ModelRequest` 同時是 provider request 與 `Instruction.CallModel` 的 canonical payload；`RequestID` 供 middleware/tracing 辨識，runtime 只在 request 未帶 tools 時補 registry list，其餘 `MaxTokens`/`StopReasons`/`Auth` 原樣轉送。`agent/spec.Subagents.MaxDepth` 直接注入 `skill.Spawner.MaxDepth`；未接 composition 的 telemetry config block 已移除，OTel 仍是可組合的 `middleware/observability`。
- `core.ModelResult.Parts` 是 provider 回應與 transcript 之間的 canonical ordered content；`NormalizeContent` 讓舊 provider 的 `Text` / `ToolCalls` 可合成 Parts，也讓新 provider 的 Parts 回填相容投影。reasoning 不併進 `Text`，避免 frontend 無意顯示 chain-of-thought；runtime 仍保留它供下一輪 provider continuation 使用。
- Provider capability boundary：`core.Provider` 是 runtime 消費端定義的最小 port，只含
  blocking `Generate`；stream、live catalog、image generation 分別是 optional
  `core.StreamProvider`、`core.ModelLister`、`provider.ImageGenerator`。
  `provider.Entry` 單獨擁有 discovery metadata 與 factories。
- Provider config pipeline：`provider.Options` 是 unresolved live input，只在
  `Resolve(Entry.Metadata)` 查 env 並投影成 `ResolvedConfig{Model, BaseURL, Auth}`。
  Endpoint 不進 `core.Auth`；credential 優先序固定為
  `單次 request Auth → 明示 Options.APIKey → Decorator → env`。
- Provider wire sharing：只在 observable contract 相同時共用 codec。
  `provider/protocol/sse` 只處理 framing；各 adapter 保留 payload DTO 與 terminal
  semantics，並由跨 adapter contract tests 鎖定共用範圍。
- Harness ports：`core.Hooks` 與 `core.EventSink` 是純資料 ports；
  `runtime.Engine` 的 nil port 即 no-op。`PreToolUse` block 轉為失敗 `ToolResult`，
  `PostToolUse.SystemNote` 追加為 system message。
- `permission.Engine` 是兩軸決策（codex 式）：`Mode`（`default`/`acceptEdits`/`plan`/`bypassPermissions`）× rule specifier（`Bash(git:*)`、`Edit(src/**)`；自寫 `**` matcher，因 `filepath.Match` 的 `*` 不跨 `/`），優先序 `deny > ask > allow`；無 rule 命中時可注入 `Fallback`（如 `permission.DefaultApprovalPolicy` 的 autonomy grid）。
- Agent config：`spec.Config` 是宣告式資料，`agent` 是組裝層。Feature block 的 `nil`
  表示關閉、`{}` 表示啟用預設；block 內具名字串選 variant。`Reasoning.Enable` 決定
  registry，`Reasoning.Style` 決定 seed，未註冊 style 由 `spec.Validate` 提前拒絕。
- Tier 階梯（`oneshot`/`basic`/`standard`/`full`）是 block 集合的單調展開簡寫，展開後顯式 block 覆蓋（explicit wins）。`tier` 與 `reasoning` 正交，組合`不`視為衝突：`T0` 無工具 → provider 收不到 tool spec → `ModelResult.ToolCalls` 恆空 → `runtime/loop.go` 的 short-circuit 在 `Decide` 前收成 `COMPLETED`，任何 strategy 到 `T0` 都退化成一次 model call。`T0` 預設關持久化，否則 `agent/cli.OpenForCLI` 的空 appName 檢查會讓 `Name` 從「`T1+` 必填」變成「永遠必填」。
- `agent.Once` 不繞過 Engine：它用 `reasoning.OneShotReasoning` + 全 nil port 走同一條路。不得改寫成「每次回 `[CALL_MODEL, DONE]`」的 no-op `Decide`——那會破壞 `core.Decide` 的純函式不變式，retry/WAL replay 會重發 model call（理由見 `reasoning/one_shot.go` 型別註解）。
- Scheduled listener sample：`sample/log-agent-v2` 持續運作，但每個非空 batch 都建立新的
  `TIER_ONESHOT` agent，讓 transcript、budget 與 `RunID` 不跨 batch 累積。只有
  `agent.Run` 與輸出都成功才 atomic commit cursor。
- 注入用 `agent.Option`（`func(*builder) error`），不用 `Deps` struct：沿 repo 既有慣例（`app` + 7 個 provider adapter 共 `8` 處 `type Option func(*x)`）。`Option` 是 closure、不可列舉、只活在本 process；`spec.Choice` 是資料、可列舉、要跨序列化邊界寫進設定檔——兩者不可合併，只在 wizard 的 `--print-go` 輸出端相遇。
- `prompt` 擁有「這一輪送什麼進 context window」，`memory` 擁有「放不下時砍什麼」：前者是 policy、後者是 mechanism，合併會讓注入與裁切互相遞迴。`SLOT_SYSTEM` 內順序由不變到易變（persona `10` → context files `20` → skill 索引 `30` → env `40`），因為 prompt caching 要求前綴穩定；預算超限時從尾端整段丟，不切半。`skill` 不知道 `prompt` 存在，adapter 住在 `prompt/source`（sub-package，透過 `SkillProvider` interface 而非型別耦合）；context-file 載入已併入 `prompt.LoadContextFiles`（固定行為、無客製化縫）。
- Steering / follow-up queue（pi 式）：`Engine.Steer` 在下一次 Decide 前插入 user message；`Engine.FollowUp` 把「本應完成」轉為續跑（每次一則）。follow-up 續跑會清掉 `think_then_act.phase` 讓 FSM 回到 reason（與 loop 既有 pending_call seeding 同一 seam）；其他五個 rule 的通用 reset 慣例待補。
- Tool-call batch settlement：同一 assistant message 的 `N` 個 `tool_use` 必須在下一次
  `CALL_MODEL` 前對應 `N` 個 `tool_result`。Runtime 一次播種並 dispatch 整批 calls；
  pause、hook block、budget skip 由 `settleSkipped` / `settleUnrun` 補齊結果。
- Round budget：`round` 是一次 `CALL_MODEL` 及其 tool calls；`turn` 是 `Decide` 次數。
  `MaxToolCalls` 超限時整批 skip + settle 並進入 continue-gate，approve 後重讀規劃，
  reject 後完成。
- `agent.Interactive.NextRound` 是唯一互動縫；approval/continue-gate 與 round-end
  follow-up 共用。Advance 只走 `SubmitHumanDecision`，避免 WAL replay double-drive。
- Agent lifecycle：`Bootstrap` 是 engine 與 opening state 的唯一組裝 owner；
  `agent.Run` 回傳 `error`，process exit code 只由 `agent/cli` 擁有。
- Agent public surface：`Parts` 只回傳 `Engine`、`Sessions`、`Skills`、`Host`、`Cwd`；
  `agent/wire` 是 optional `core.EventSink` adapter，不是序列化設定的一部分。
- `session.Manager` 只加 lineage（meta sidecar：`ID`/`Parent`/`Title`/`Cwd`）與 fork（複製 state+WAL、改 RunID）；transcript 真相仍是 WAL JSONL。
- `wire` 的 caller path 是
  `runtime → core.EventSink → agent/wire.Sink → JSONL`；Envelope 是對外 API，必須維持
  JSON round-trip 穩定。
- `agent/permission`、`agent/session`、`agent/wire` 各自擁有完整 domain vocabulary，
  不攤平進 composition root，且 production dependencies 只允許 `core`。
- `sample/code-agent/tui` 是 zero-dependency application renderer，不 import agentsdk；
  SDK 與 `agent` composition 都不擁有 terminal presentation。
- `provider.Entry` 是 discovery/config 的唯一 owner；每個 `register.go` 在 Entry literal
  宣告 `Name`、`Metadata`、`Catalog`、`New` 與 optional `NewImage`。Static fallback
  讀 `Entry.Catalog`，live path 才使用 `core.ModelLister`。
- `provider.Decorator` 定義在 provider layer 並於每個 request 解析 `core.Auth`，
  讓 OAuth refresh 可涵蓋 retry/SSE reconnect 而不重建 adapter。只有
  `provider/credential` 可 import `github.com/bizshuk/auth`；其 Source/Login wiring
  尚未接入 production composition。
- `provider/credential` 擁有 `(provider name, credential kind) → auth route id`
  對照；auth 的扁平 route ID 不進 `spec.Model.Provider`。
- Process host 由 `agent.Host` / `agent/cli.OpenForCLI` 擁有，middleware presets
  由 `middleware/preset` 擁有；root 不設通用 `config/` package。
- Default provider 名稱由 `provider.DEFAULT_NAME` 擁有；credential kind constants
  留在 `core`，作為 `agent/spec` 與 provider resolution 共用的 core-only vocabulary。
- `prompt/source` 擁有內建 Sources，透過 `SkillProvider` interface 接 skill registry；
  discovery roots 由 `agent` 組裝，`prompt/source` 不 import `skill` 或 `agent`。
- 頂層 `sample/demo-*` 是單一 SDK 元件展示，`sample/*-agent` 是完整 agent；
  `provider/sample` 是 package-local provider API example。
- `provider.Metadata` 分別宣告 `OAuthEnv` / `APIKeyEnv`；
  `Options.CredentialKind` 的 `auto` / `api_key` / `oauth` 使用
  `core.CREDENTIAL_KIND_*`，`Resolve` 產生 canonical `ResolvedConfig`。

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
- Proxy 的 `upstream.CredentialResolver` 是 `auth.Resolver` 的 thin adapter，只把
  `auth.UnavailableError` 映射為 `credential_unavailable` ProxyError。Agentsdk 的
  request-time seam 是 `provider.Decorator`；`credential.Source.Decorator()` 已提供
  resolver implementation，但 production composition caller 尚未接線。
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

JSONL 對外 envelope 由 `agent/wire` 擁有，經
`runtime → core.EventSink → agent/wire.Sink` 接到真實 caller。不得把內部 `State`
欄位直接當成穩定外部 API；Envelope 變更需維持 JSON round-trip。

## 模組對應 (Module Mapping)

| 領域                      | 套件 / 進入點                                                                                                                                                                                                                                                                                                                                                                                          |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 宣告式設定                | `agentsdk/agent/spec`：`Config`、`Choice`、`Expand`（tier 展開）、`Validate`、`Prepare`（只 import `core` + 純 stdlib）                                                                                                                                                                                                                                                                        |
| agent 設定檔 I/O          | `agentsdk/utils/agentconfig`：`Decode`/`DecodeBytes`/`Encode`/`EncodeBytes`、`LoadFile`/`SaveFile`/`Marshal`/`FormatOf`、`Format`/`FORMAT_YAML`/`FORMAT_JSON`（re-export 自 `utils/configfile`）                                                                                                                                       |
| agent 組裝                | `agentsdk/agent`：`New`/`MustNew`/`Bootstrap`（實作 `agent.Runner`）、`Once`/`OnceStream`、`Option` 全部 `With*`、`BuildSources`、`Run`、`Interactive`                                                                                                                                                                                                                                                 |
| prompt 內容管理           | `agentsdk/prompt`：`Slot`（system/user/reminder）、`Section`、`Source`、`Builder.Seed`/`Turn`、`Static`、`PersonaSource`/`ContextFileSource`/`EnvSource`/`ReminderSource`                                                                                                                                                                                                                              |
| 狀態與 ports              | `agentsdk/core`：`State`/`Budget`、`Event`/`Observation`、`Instruction`/`Decide`、最小 `Provider`、optional `StreamProvider`/`ModelLister`、`Tool` 與 persistence/presentation ports；檔案依 domain 分組，package/API 不拆                                                                                                                                                                             |
| 推理策略                  | `agentsdk/reasoning`：`DecisionRule`、`NewRule` built-in factory、`NewDecide` dispatcher + 6 個 `New*` rule constructor                                                                                                                                                                                                                                                                                |
| runtime                   | `agentsdk/runtime`：`NewEngine`、`Run`、`RunWithEvent`、`Resume`、`SubmitHumanDecision`                                                                                                                                                                                                                                                                                                                |
| tools/safety              | `agentsdk/tool`、`agentsdk/tool/builtin`：`Tool`、`CallWithRawMessage`、`NewRegistry`、`RegisterFunc`、allowlist-aware `Register`、`RegisterDefaults`                                                                                                                                                                                                                                                  |
| memory                    | `agentsdk/memory`、`memory/checkpoint`、`memory/filestore`                                                                                                                                                                                                                                                                                                                                             |
| lifecycle hooks           | `agentsdk/middleware/hook`：`NewRunner`、`Rule`、`Func`、`Command`（實作 `core.Hooks`；lifecycle event 為 fan-out + decision merge，handler 串以 middleware-style 連續執行，但 signature 仍獨立）                                                                                                                                                                                                      |
| permission                | `agentsdk/agent/permission`：`Engine`、`Rule`、`MatchSpec`（實作 `core.ApprovalPolicy`；只 import `core`）                                                                                                                                                                                                                                                                                             |
| session 管理              | `agentsdk/agent/session`：`NewManager`、`Begin`、`List`、`Latest`、`Fork`、`Tree`（只 import `core`）                                                                                                                                                                                                                                                                                                  |
| context files             | `agentsdk/prompt`：`LoadContextFiles(cwd, userDir)`（AGENTS.md/CLAUDE.md 階層 + `@import`；無 Loader 結構、無 config knob）                                                                                                                                                                                                                                                                      |
| skills/commands/subagents | `agentsdk/skill`：`NewRegistry` 統一索引三類定義；`DiscoverSkills`／`DiscoverCommands`／`DiscoverSubagents` 採相同的 later-wins 覆寫規則，並以 `Skills`／`Commands`／`Subagents` 回傳排序結果；另提供 `SubAgent`、`Body`、`ExpandCommand`、`RenderTemplate`、`ParseDef`、`NewSpawner`、`Depth`／`WithDepth`。源碼分為 `skill.go`／`command.go`／`registry.go`／`subagent.go` 四檔                      |
| headless wire             | `agentsdk/agent/wire`：`Envelope`、`NewEncoder`/`NewDecoder`、`NewSink`、`ReadRequest`/`WriteResponse`、`FormatStream`                                                                                                                                                                                                                                                                                 |
| terminal UI               | `agentsdk/sample/code-agent/tui`（zero-dep，非 SDK 表面）：`Renderer`、`Component`、`Terminal`、`VisibleWidth`/`WrapText`                                                                                                                                                                                                                                                                              |
| dependency graph          | external CLI `github.com/bizshuk/go-dependency-analysis`：Go tooling facts + JSON policy heuristics；不加入本 workspace、不被本 repo import                                                                                                                                                                                                                                                            |
| middleware                | `agentsdk/middleware`、`harness`、`loopguard`、`security`、`observability`                                                                                                                                                                                                                                                                                                                             |
| agent lifecycle           | `agentsdk/agent`：`Run`、`Host`、`Interactive`、`Pause`/`Resume`、`WithRoundTimeout`；`agentsdk/agent/cli`：`Main`/`Run`、`OpenForCLI`/`MustOpenForCLI`                                                                                                                                                                                                                                                |
| middleware preset         | `agentsdk/middleware/preset`：`Default()`（retry→timeout→budget→loopguard）、`Secure(sandbox, approval)`（再加 sandbox→approval→spotlight→sanitizer）                                                                                                                                                                                                                                                  |
| credential                | `agentsdk/provider/credential`：`RouteID`/`Kinds`/`Names`、`NewSource`/`NewAutoSource`/`Source.Decorator()`、`Login`；唯一 import `bizshuk/auth` 之處                                                                                                                                                                                                                                                  |
| provider registry         | `agentsdk/provider`（package `provider`，非 `registry`）：`Entry` 單獨擁有 name / metadata / static catalog / model+image factories；`Names`/`Entries`/`Lookup`/`Catalog`/`Capabilities`/`New`/`NewImage`/`Options.Resolve`/`ResolvedConfig`/`DEFAULT_NAME`；`env` 查詢以 `LookupEnv` 注入                                                                                                             |
| root CLI subcommands      | `agentsdk/cmd`：`NewWizardCommand`（`wizard`/`w` 設定產生器）、`NewProviderCommand`（root cobra `provider` smoke-test CLI；打 `core.Provider.Generate` / `core.StreamProvider.Stream` 不走 Engine；`--list-models` 優先打 live `core.ModelLister`,失敗 fallback `Entry.Catalog`）                                                                                                                      |
| authentication            | 外部 module `github.com/bizshuk/auth/{model,svc,utils,provider}`：`Login`、`For`、`FileStore`、`NewResolver`；root 以 `go.mod` require                                                                                                                                                                                                                                                            |
| proxy                     | 外部 repo `github.com/bizshuk/proxy/{handlers,config,model,svc}`：`handlers.New`、`config.LoadConfig`、`model.Format`、`svc/route.Router`                                                                                                                                                                                                                                                        |
| provider adapters         | `agentsdk/provider/{anthropic,google,minimax,grok,ollama,codex,antigravity}`：各實作 `provider.Adapter`（`core.Provider` + `core.StreamProvider`）；Google/Grok 另實作 `provider.ImageGenerator`（`POST /images/generations`）；除 codex/antigravity 外皆另實作 optional `core.ModelLister`。identity / credential metadata / factories / static catalog 只存在於各自 `register.go` 的 `Entry` literal |

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

# auth 只准被 provider/credential 看見
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

## 專案追蹤文件 (Project Tracking)

- 當前技術結構、ownership 與不變式：本檔。
- 歷史變更與已完成里程碑：[`docs/CHANGELOG.md`](docs/CHANGELOG.md)。
- 尚未完成與刻意保留的工作：[`README.todo`](README.todo)。
- 仍在執行的落地計畫：`plans/`。
- 已實作規格的濃縮索引：[`docs/specs/2026-07-29-Summary.md`](docs/specs/2026-07-29-Summary.md)。

## 慣例與注意事項 (Conventions and Caveats)

- 常數使用 `SCREAMING_SNAKE_CASE`，變數、函式、型別使用 Go `MixedCaps`；package 名稱使用單字。
- 錯誤以 `fmt.Errorf("...: %w", err)` wrap；公開 error 不帶 credential、authorization、prompt 或未清理 upstream body。
- 測試採 table-driven + `t.Run`，`testify/assert` 與 `testify/require` 並用；`utils/testutil` 只可被測試使用。
- `core.Decide`、reasoning rules、transform pair 不得直接做 I/O、讀 credential 或建立 HTTP request；這些責任分別屬於 runtime、upstream 與 auth。
- `sample/logdoctor-agent/core` 與 `agentsdk/core` 是不同 module path；import 時使用 `domain` / `sdkcore` alias。
- `proxy/docs/specs/2026-07-16-client-llm-adaptor.md` 是 legacy historical design；修改 proxy 時以 pairwise spec、現行 `proxy/` code 與測試為準。
- 修改 package tree、module、路由或 protocol contract 後，必須同步本檔；業務範疇變更才同步 `README.md`。格式 catalog 的 entity/來源異動則同步 `proxy/docs/specs/format/README.md`（外部 repo）。
