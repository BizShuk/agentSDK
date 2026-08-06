# CLAUDE.md — agentsdk 技術脈絡 (Technical Context)

`agentsdk` 是 Go Agentic Loop SDK、provider adapter、認證整合與範例程式的 workspace。
本文只記錄目前程式碼採用的邊界與決策；歷史變更見
[`docs/CHANGELOG.md`](docs/CHANGELOG.md)，未完成工作見 [`README.todo`](README.todo)。

## 技術基準 (Current Baseline)

- 語言與 workspace：Go `1.26.0`、`go.work` 納入 root 與 `sample/` 下各 module。
  `auth` 與 `proxy` 是外部 repo，不列入本 workspace；dependency analyzer 亦然。
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
- Provider capability surface：media（`ImageGenerator`／`VideoGenerator`／
  `MusicGenerator`／`Transcriber`／`SpeechGenerator`）與 realtime（`LiveConnector`／
  `Translator`）都留在 provider layer，不進 `core.Provider`。`Entry.NewX` /
  `provider.NewX` 是一致建構路徑，unsupported adapter 回 typed
  `ErrUnsupportedCapability`；`SpeechStreamer`／`VoiceLister`／`TranslateStreamer`
  這類同一 client 的附加能力以 type assertion 發現，decorator 包裝後仍保留。
  `Register` 的「至少一 factory」檢查涵蓋 audio factories（audio-only entry 的
  `New` 為 nil）。每個 adapter 的 endpoint、預設 model、base override 與 wire 例外
  處置由 [`docs/providers.md`](docs/providers.md) 擁有——upstream 改版屬高頻異動,
  不放進本檔的 boundary 規則。
- Reasoning content boundary：`core.Part` 以 `PART_KIND_REASONING` 表示可攜 reasoning
  文字，`ReasoningState` 保存 opaque continuation metadata。`ModelResult.Parts` 是有序
  canonical assistant content；無法表示 metadata 的 wire path 必須明確報錯。
- `cmd/provider/` 是 `provider` 子指令的 per-type handler package：一檔一 type
  （chat/image/music/speech/transcribe）+ matrix.go + catalog.go；cobra wiring、
  flags 與 dispatch 留在 `cmd/provider.go`（集合式 cmd package）。handler 各自建
  client（`provider.New` / `NewImage` / `NewMusic` / `NewSpeech` /
  `NewTranscriber`），不支援的 provider 回 typed `ErrUnsupportedCapability`。
  package 名為 `provider`，import 端以 `providercli` alias 與 SDK `provider` 區分。
  指令與 flag 用法見 [`docs/cli.md`](docs/cli.md)。
- 外部依賴：`auth` 是 go.mod require 的外部 module，只被 `provider/credential`
  使用；`proxy` 完全在外部 repo，本 repo 無目錄、無 require、無 import。

## 專案結構 (Project Structure)

```text
agentsdk/
├── README.md                         # 業務範疇與使用者導覽
├── CLAUDE.md                         # 技術脈絡與架構決策（本檔）
├── README.todo                       # 尚未完成的工作
├── go.work                           # root + sample/ 下各 module
├── go.mod                            # github.com/bizshuk/agentsdk
├── main.go                           # cobra root binary；掛載 `provider` 與 `wizard` 兩個子指令
├── cmd/                              # root cobra subcommands
│   ├── agent/                        # agent 相關子指令
│   │   └── wizard/                   # `wizard`/`w` 設定產生器（逐階段 wizard prompt，產出 agent.Config）
│   ├── provider.go                   # `provider` 手動測試指令：flags + dispatch（不走 Agent/Engine/harness）
│   └── provider/                     # per-type handlers：chat.go / image.go / music.go / speech.go / transcribe.go + matrix.go / catalog.go
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
├── utils/                            # 根層共用 utilities umbrella
│   ├── agentconfig/                  # agent 設定檔 I/O：Decode/Encode/LoadFile/SaveFile（re-export utils/configfile）
│   ├── configfile/                   # 副檔名決定編碼、一律以 JSON 呈現給 caller,故 `json` tag 是唯一真相
│   ├── frontmatter/                  # adrg/frontmatter YAML/TOML/JSON wrapper,key:value 攤平為 string map
│   └── testutil/                     # in-process fake provider / state store / notifier（僅測試可用）
├── middleware/                       # chain、retry/timeout/budget/loopguard、安全與 OTel tracing；preset/ 與 hook/ 子包
├── memory/                           # context window、compactor、checkpoint、JSON state/WAL
├── runtime/                          # Engine：dispatch Instruction、fold Event、Run/Resume/HITL
├── provider/                         # registry、credential bridge、protocol codecs 與七個 adapters
│   ├── capability.go                 # model/image/video/music/audio capability discovery + typed unsupported error
│   ├── image.go / video.go / music.go / speech.go / transcribe.go / error.go # media contracts、request/result、auth wrappers、structured API error
│   ├── live.go / translate.go        # realtime session（LiveConnector/LiveSession/LiveEvent）與 translation（Translator + optional TranslateStreamer）contracts
│   ├── registry.go                   # Entry 是 name / metadata / static catalog / factories 的唯一真相 + DEFAULT_NAME
│   ├── registry_options.go           # Options（unresolved）→ ResolvedConfig（construction input）；集中 env / credential class resolution
│   ├── adapter.go                    # Adapter = core.Provider + core.StreamProvider；discovery data 不進 runtime client
│   ├── decorator.go                  # Decorator = func(ctx) (core.Auth, error)：model/image/video/music 每次 request 共用解析規則
│   ├── credential/                   # 全 repo 唯一 import bizshuk/auth 之處：(name, kind) → auth route id 對照、Decorator 實作、Login 委派
│   ├── all/                          # meta-package：blank-import 全部 adapter 的便利入口
│   ├── protocol/
│   │   ├── sse/                      # stdlib-only 完整 SSE frame decoder / writer；不含 provider terminal semantics
│   │   ├── openaichat/               # Google/Ollama 共用 request/response codec + Frame → ModelChunk
│   │   └── openaiimage/              # Google/Grok 共用 /images/generations JSON codec + bounded response/error
│   ├── utils/                        # provider 共用 utilities：live model catalog helper（Fetch/DecodeIDList/Merge）
│   ├── anthropic/                    # anthropic-sdk-go adapter
│   ├── antigravity/                  # adapter：Google Cloud Code v1internal（Gemini + Claude），OAuth-only
│   ├── codex/                        # adapter：OpenAI Codex OAuth
│   ├── elevenlabs/                   # adapter：ElevenLabs STT/TTS（audio-only，New 為 nil、無 chat surface）
│   ├── google/                       # stdlib HTTP adapter + Gemini Live API websocket（live/translate）
│   ├── grok/                         # adapter：xAI Grok
│   ├── minimax/                      # adapter：MiniMax model HTTP/SSE + video/music/speech generation transport
│   └── ollama/                       # adapter：本地 Ollama endpoint
├── benchmark/                        # provider-model capability benchmark；root package 擁有 run→iterate→store flow
│   ├── testdata/                     # 共用輸入資產：shape.png（vision）、tone.wav（transcribe）
│   ├── cmd/                          # flag 驅動 runner：-provider/-model（all = sweep）/-kinds/-list，結果寫進同一 pkg/<pair-slug>
│   ├── gen/                          # 產生器：registry × DefaultCatalog × KindsOf → pkg/ 全部套件（DO NOT EDIT）
│   └── pkg/<provider-model>/         # 每個 runnable catalog model 一個 gen 產生的可執行套件（dir 名 = PairSlug）；結果存自身 tmp/<session-id>/case-NN-<name>/
├── sample/                           # demo-* 是單一元件展示，*-agent 是完整 agent
│   ├── code-agent/                   # 全 harness 組合 CLI：tui 互動 / -p print / --json（wire）+ session flags；composition 位於 cmd/
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
│   ├── providers.md                  # 各 adapter 的 capability、endpoint 與 wire 細節（高頻異動）
│   ├── cli.md                        # provider / wizard / sample / benchmark 指令參考
│   ├── terminology.md                # 領域術語單一定義來源
│   ├── specs/                        # root SDK architecture/spec history
│   ├── memory/                       # retrospective 與來源研究
│   └── tutorials/                    # 教學文件
├── scripts/                          # verify-workspace.sh 等專案腳本
├── plans/                            # 進行中的落地計畫
└── tmp/                              # runtime symlink，不放 production logic
```

## 技術棧 (Tech Stack)

| 類別 | 技術 | 為什麼是它 |
| ---------------------- | ----------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Language | Go `1.26+` | `go.work` 納入 root 與 `sample/` 下各 module |
| Root runtime | Go stdlib、`bizshuk/gosdk` | config/log/notify 等組合點在 root 或 sample |
| External auth module | `bizshuk/auth` | 只由 `provider/credential` import |
| CLI/config | `spf13/cobra`、`spf13/viper` | samples 與 root `provider` / `wizard` 子指令 |
| Config 序列化 | stdlib `encoding/json`、`gopkg.in/yaml.v3` | `agent/spec` 只用 JSON；YAML 在 `agent` 走 JSON tag 轉譯，不另立 tag |
| Markdown frontmatter | `adrg/frontmatter` | `utils/frontmatter` wrapper：自動偵測 YAML/TOML/JSON delimiter，攤平為 `map[string]string` |
| State/schema | `testify`、`invopop/jsonschema` | table-driven tests、`core.Tool` RawMessage Call、反射式 JSON Schema |
| IDs/telemetry | `google/uuid`、OpenTelemetry | request ID、transform warning/loss metrics |
| Anthropic adapter | `anthropics/anthropic-sdk-go` | 只由 `provider/anthropic` 引入 |
| Websocket | `coder/websocket` | Gemini Live 與 OpenAI Realtime 的 bidirectional session |
| Shared protocol codecs | `net/http` + JSON + SSE | `provider/protocol/{sse,openaichat,openaiimage}`，不承接 vendor terminal semantics |
| Terminal UI | Go stdlib only | `sample/code-agent/tui`（zero-dep）；differential rendering、CSI 2026、不用 alternate screen |

版本 pin 的真相是 `go.mod`，不在此重複。

`core/` 不 import `gosdk` 或任何 provider SDK；外部 auth 只由 `provider/credential`
import。Root module 仍非 stdlib-only，因為 composition、CLI、provider adapters 與
observability 都在此。

## 核心架構決策 (Core Decisions)

- `core` 是純狀態與 transition contract。`core.Decide(state, event)` 不做 I/O，只回傳下一個 `State` 與 `[]Instruction`；runtime 才執行 model、tool、approval、notify，並經 StateStore/WAL 自動持久化。
- `core` 不拆成 domain 子套件：`State`、`Event`、`Instruction`、model/tool contracts 會彼此交叉引用，硬拆只會製造 import cycle 或 root alias facade。domain 邊界改由一檔一職責表達，檔名直接對應公開詞彙。
- `State` 的對話模型是 `Message{Role, Parts}`；`Part` 支援 text、reasoning、audio、image、tool use、tool result。reasoning 文字沿用 `Part.Text`，opaque continuation metadata 放 `ReasoningState`，不與 agent strategy 的 `ReasoningStyle` 混用。JSON tags `scratch` 與 `thinking_kind` 保留以相容舊 state。
- `Instruction` 是 tagged union，只保留有 production producer/consumer 的 `call_model`、`call_tool`、`request_approval`、`notify`、`done`；持久化由 runtime 的 StateStore/WAL lifecycle 負責，presentation 由 `core.EventSink` / `Engine.Emitter` 負責。
- 宣告式 `agent/spec` 只暴露 composition root 真正消費的設定。Process deadline 以 `agent.WithTimeout` 注入，`memory/compaction` mechanism 由需要它的 caller 明確組裝，presentation 則由 frontend 以 `agent.WithSink` 或 `Engine.Sink` 接管。
- `runtime.Engine` 是 shell，維護 `core.Provider`、`ToolRegistry`、`StateStore`、`WriteAheadLog`、`Notifier` 與 middleware。`Middleware == nil` 代表 no-op；`preset.Default()` 與 `preset.Secure()` 由 composition root 明確選用。
- `preset.Default()` 順序為 `retry → timeout → budget → loopguard`；
  `preset.Secure()` 再加入 `sandbox → approval → spotlight → sanitizer`。工具輸出會被
  spotlight 標為 untrusted，prompt injection 命中會被 sanitizer 改寫。
- WAL recovery 先載入 snapshot，再依 `LastInputSeq` replay Event；replay 不重新呼叫模型，避免 crash recovery 產生重複副作用。`memory/filestore` 使用 atomic state write + JSONL append。
- `reasoning/` 擁有 `DecisionRule`、built-in factory `NewRule` 與 registry-backed `NewDecide`，因為 rule construction 與 dispatcher 都是 strategy 詞彙的 consumer；`DecisionRule.Kind()` 與 registry key 直接用 `string`。六個 strategy（ThinkThenAct、PlanThenRun、RunThenReview、OneShotReasoning、LearnFromFailure、ChooseAgent）都是 phase FSM。`core` 只保留 `Decide` function type，所以 `runtime` 不必反向依賴具體 reasoning package；working memory 是 rule 與 runtime/middleware 的通訊介面。
- `core.Tool` 是唯一可執行工具契約（`Name`/`Spec`/`Call(json.RawMessage)`），`tool.Tool` 只是 alias，不另立平行介面。`ToolCall`/`ToolResult` 同時是 model chunk、transcript part、instruction/event 的 canonical payload，不另立串流或訊息專用衍生 struct。`tool.CallWithRawMessage` 在每個 concrete tool 的 `Call` 內把 raw JSON 轉成 typed args、驗證 required fields、執行 typed business function，再把輸出轉回 `ToolResult`；`RegisterFunc` 沿用同一 converter。`builtin.Register` 接受 allowlist（空 = 全部）並以 all-or-nothing 語意註冊 Read/Write/Edit/Bash/Glob/Grep；`RegisterDefaults` 是全選 convenience wrapper。
- `core.ModelRequest` 同時是 provider request 與 `Instruction.CallModel` 的 canonical payload；`RequestID` 供 middleware/tracing 辨識，runtime 只在 request 未帶 tools 時補 registry list，其餘 `MaxTokens`/`StopReasons`/`Auth` 原樣轉送。`agent/spec.Subagents.MaxDepth` 直接注入 `skill.Spawner.MaxDepth`；OTel 是可組合的 `middleware/observability`，不走 config block。
- `core.ModelResult.Parts` 是 provider 回應與 transcript 之間的 canonical ordered content；`NormalizeContent` 讓舊 provider 的 `Text` / `ToolCalls` 可合成 Parts，也讓新 provider 的 Parts 回填相容投影。reasoning 不併進 `Text`，避免 frontend 無意顯示 chain-of-thought；runtime 仍保留它供下一輪 provider continuation 使用。
- Provider capability boundary：`core.Provider` 是 runtime 消費端定義的最小 port，只含
  blocking `Generate`；stream、live catalog、image generation、video generation、
  music generation
  分別是 optional `core.StreamProvider`、`core.ModelLister`、
  `provider.ImageGenerator`、`provider.VideoGenerator`、`provider.MusicGenerator`。
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
  宣告 `Name`、`Metadata`、`Catalog`、`New` 與 optional `NewImage` / `NewVideo` /
  `NewMusic`。Static fallback
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
  provider API 的手動測試一律走 `agentsdk provider` 子指令（`cmd/` 是手動測試
  的集合式 package，不另立平行 sample CLI）。
- `benchmark/` 是 root module 內的 provider-model capability benchmark：root
  package 擁有 `run → iterate cases → query → store` flow 與預定義 case sets
  （chat/image/speech/transcribe/video/music），每個 `benchmark/pkg/<provider-model>`
  子套件是單一 pair 的 runnable main，結果以 session id 落在該套件自身 `tmp/`
  （gitignored）。case 失敗只報告並跳過，不中斷 session。model 是每個 kind 共有的
  軸：chat model 由 `Target.Model` 釘住，media model 由 `WithModel` 釘在
  `Case.Model`，兩者皆取自該 provider 的 `DefaultCatalog`；`offCatalog` 只印
  warning 不擋——live catalog、本地 Ollama 與 catalog 未收錄的 media model 可合法
  超出 snapshot。`pkg/` 全部由 `benchmark/gen` 產生（registry × `DefaultCatalog` ×
  `KindsOf`），`KindsOf(provider, spec)` 擁有 model→kind 對照——`ModelSpec` 沒有
  output modality，命名知識集中於此，回空 = 不產生套件。重新產生只覆寫 main.go
  不動 `tmp/`；離開 catalog 的 model 其帶 marker 的產生檔會被移除。testdata anchor
  由 `benchmark.Root()`（`runtime.Caller` 定位本套件源碼）提供，與巢狀深度無關；
  credential 只走 `os.Getenv`，不掛 gosdk/viper。runner 指令見
  [`docs/cli.md`](docs/cli.md)。
- `provider.Metadata` 分別宣告 `OAuthEnv` / `APIKeyEnv`，image / video / music /
  speech / live endpoint 可另以 `ImageBaseURLEnv` / `VideoBaseURLEnv` /
  `MusicBaseURLEnv` / `SpeechBaseURLEnv` / `LiveBaseURLEnv` 宣告 override；
  `Options.CredentialKind` 的 `auto` / `api_key` / `oauth` 使用
  `core.CREDENTIAL_KIND_*`，`Resolve` 產生 canonical `ResolvedConfig`。
  Credential entry 規則：每個 provider entry `必須`宣告 `APIKeyEnv`；`OAuthEnv`
  只給真的有 OAuth flow 的特定 provider（anthropic／codex／grok／antigravity）。
  不為單一 credential kind 另立新 entry——kind 是 entry 內的軸
  （`spec.Model.CredentialKind`），不是 provider 身分。唯一例外是 antigravity：
  Cloud Code gateway 在 wire 層只收 OAuth Bearer，無 `APIKeyEnv`。
- 外部 module 的消費邊界：`bizshuk/auth` 只能被 `provider/credential` import
  （由 `TestAuthImportedOnlyByProviderCredential` 把關），其 credential storage、
  OAuth / device-code flow 與 CLI 屬 [`bizshuk/auth`](https://github.com/BizShuk/auth)
  自身的契約；LLM protocol proxy 是外部 repo
  [`bizshuk/proxy`](https://github.com/BizShuk/proxy)，本 repo 無目錄、無 require、
  無 import，其架構由該 repo 自行記錄。
- `core.Notifier` 的介面方法集與 `gosdk/notify.Notifier` 完全相同（結構性相容），
  gosdk 的 Multi / Stdout / Slack 可直接傳入，不需 adapter。
- Presets, not walls：設定挑 preset 而非組合細節（middleware 鏈的順序是正確性，
  不是偏好）；`WithCustomize` 在全部 stage 之後拿到組好的 `*runtime.Engine`，
  設定詞彙沒覆蓋的都還做得到。

## CLI、設定與持久化 (CLI, Config, Persistence)

`main.go` 直接建立 Cobra root（binary 名稱 `agentsdk`），掛載 `cmd.ProviderCmd` 的
`provider`（直接呼叫 `core.Provider.Generate` / `core.StreamProvider.Stream`，不走
Agent/Engine/harness）與 `wizard.WizardCmd` 的 `wizard`（alias `w`）。命令一律
package-level exported var + `init()` 綁 flag，不用 `NewXxxCmd()` constructor。
Wizard 的設定詞彙來自 `spec`，provider 資料直接來自 `provider.Entries`/`Catalog`；
指令用法見 [`docs/cli.md`](docs/cli.md)。`agent/cli.OpenForCLI(appName, level)`
為 sample 建立：

```text
~/.config/<appName>/
├── data/
│   ├── states/<runID>.json
│   ├── wal/<runID>.jsonl
│   └── auth/*.json
└── logs/<runID>.log
```

`agent.Run` 是可嵌入 lifecycle：input validation → wall-clock deadline → 單次 Bootstrap → panic-safe Engine.Run → optional OnComplete，失敗直接回傳 `error`。`agent/cli.Main` 負責 signal binding，`agent/cli.Run` 才將錯誤轉成 exit code。

JSONL 對外 envelope 由 `agent/wire` 擁有，經
`runtime → core.EventSink → agent/wire.Sink` 接到真實 caller。不得把內部 `State`
欄位直接當成穩定外部 API；Envelope 變更需維持 JSON round-trip。

## 模組對應 (Module Mapping)

| 領域 | 套件 / 進入點 |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 宣告式設定 | `agentsdk/agent/spec`：`Config`、`Choice`、`Expand`（tier 展開）、`Validate`、`Prepare`（只 import `core` + 純 stdlib） |
| agent 設定檔 I/O | `agentsdk/utils/agentconfig`：`Decode`/`DecodeBytes`/`Encode`/`EncodeBytes`、`LoadFile`/`SaveFile`/`Marshal`/`FormatOf`、`Format`/`FORMAT_YAML`/`FORMAT_JSON`（re-export 自 `utils/configfile`） |
| agent 組裝 | `agentsdk/agent`：`New`/`MustNew`/`Bootstrap`（實作 `agent.Runner`）、`Once`/`OnceStream`、`Option` 全部 `With*`、`BuildSources`、`Run`、`Interactive` |
| prompt 內容管理 | `agentsdk/prompt`：`Slot`（system/user/reminder）、`Section`、`Source`、`Builder.Seed`/`Turn`、`Static`、`PersonaSource`/`ContextFileSource`/`EnvSource`/`ReminderSource` |
| 狀態與 ports | `agentsdk/core`：`State`/`Budget`、`Event`/`Observation`、`Instruction`/`Decide`、最小 `Provider`、optional `StreamProvider`/`ModelLister`、`Tool` 與 persistence/presentation ports；檔案依 domain 分組，package/API 不拆 |
| 推理策略 | `agentsdk/reasoning`：`DecisionRule`、`NewRule` built-in factory、`NewDecide` dispatcher + 6 個 `New*` rule constructor |
| runtime | `agentsdk/runtime`：`NewEngine`、`Run`、`RunWithEvent`、`Resume`、`SubmitHumanDecision` |
| tools/safety | `agentsdk/tool`、`agentsdk/tool/builtin`：`Tool`、`CallWithRawMessage`、`NewRegistry`、`RegisterFunc`、allowlist-aware `Register`、`RegisterDefaults` |
| memory | `agentsdk/memory`、`memory/checkpoint`、`memory/filestore` |
| lifecycle hooks | `agentsdk/middleware/hook`：`NewRunner`、`Rule`、`Func`、`Command`（實作 `core.Hooks`；lifecycle event 為 fan-out + decision merge，handler 串以 middleware-style 連續執行，但 signature 仍獨立） |
| permission | `agentsdk/agent/permission`：`Engine`、`Rule`、`MatchSpec`（實作 `core.ApprovalPolicy`；只 import `core`） |
| session 管理 | `agentsdk/agent/session`：`NewManager`、`Begin`、`List`、`Latest`、`Fork`、`Tree`（只 import `core`） |
| context files | `agentsdk/prompt`：`LoadContextFiles(cwd, userDir)`（AGENTS.md/CLAUDE.md 階層 + `@import`；無 Loader 結構、無 config knob） |
| skills/commands/subagents | `agentsdk/skill`：`NewRegistry` 統一索引三類定義；`DiscoverSkills`／`DiscoverCommands`／`DiscoverSubagents` 採相同的 later-wins 覆寫規則，並以 `Skills`／`Commands`／`Subagents` 回傳排序結果；另提供 `SubAgent`、`Body`、`ExpandCommand`、`RenderTemplate`、`ParseDef`、`NewSpawner`、`Depth`／`WithDepth`。源碼分為 `skill.go`／`command.go`／`registry.go`／`subagent.go` 四檔 |
| headless wire | `agentsdk/agent/wire`：`Envelope`、`NewEncoder`/`NewDecoder`、`NewSink`、`ReadRequest`/`WriteResponse`、`FormatStream` |
| terminal UI | `agentsdk/sample/code-agent/tui`（zero-dep，非 SDK 表面）：`Renderer`、`Component`、`Terminal`、`VisibleWidth`/`WrapText` |
| middleware | `agentsdk/middleware`、`harness`、`loopguard`、`security`、`observability` |
| agent lifecycle | `agentsdk/agent`：`Run`、`Host`、`Interactive`、`Pause`/`Resume`、`WithRoundTimeout`；`agentsdk/agent/cli`：`Main`/`Run`、`OpenForCLI`/`MustOpenForCLI` |
| middleware preset | `agentsdk/middleware/preset`：`Default()`（retry→timeout→budget→loopguard）、`Secure(sandbox, approval)`（再加 sandbox→approval→spotlight→sanitizer） |
| credential | `agentsdk/provider/credential`：`RouteID`/`Kinds`/`Names`、`NewSource`/`NewAutoSource`/`Source.Decorator()`、`Login`；唯一 import `bizshuk/auth` 之處 |
| provider registry | `agentsdk/provider`（package `provider`，非 `registry`）：`Entry` 單獨擁有 name / metadata / static catalog / model+image+video+music+audio factories；`Names`/`Entries`/`Lookup`/`Catalog`/`Capabilities`/`New`/`NewImage`/`NewVideo`/`NewMusic`/`NewTranscriber`/`NewSpeech`/`NewLive`/`NewTranslate`/`Options.Resolve`/`ResolvedConfig`/`DEFAULT_NAME`；`env` 查詢以 `LookupEnv` 注入 |
| capability benchmark | `agentsdk/benchmark`：`Target`、`Case`/`Kind`、六組 case sets（`ChatCases`/`ImageCases`/`SpeechCases`/`TranscribeCases`/`VideoCases`/`MusicCases`）與 `WithModel`、`Main`/`Run`/`RunPair`/`Root`/`PairSlug`、`CatalogSpecs`/`KindsOf`、`Record`；`benchmark/gen` 產生 `benchmark/pkg/<provider-model>` 全部子套件，`benchmark/cmd` 是 flag runner，結果為 `pkg/<pair-slug>/tmp/<session-id>/case-NN-<name>/`（meta.json + outputs）+ session `summary.json` |
| root CLI subcommands | `agentsdk/cmd`：`cmd.ProviderCmd`（`provider` 手動測試 CLI，不走 Engine；per-type handler 在 `cmd/provider` package：`Chat`/`Image`/`Music`/`Speech`/`Transcribe`/`WriteMatrix`/`Catalog`/`JoinCapabilities` + `Request`）、`cmd/agent/wizard.WizardCmd`（`wizard`/`w` 設定產生器）；flag 用法見 [`docs/cli.md`](docs/cli.md) |
| authentication | 外部 module `github.com/bizshuk/auth`：只由 `provider/credential` 消費；API 契約見該 repo |
| provider adapters | `agentsdk/provider/{anthropic,google,minimax,grok,ollama,codex,antigravity,elevenlabs}`：前七者實作 `provider.Adapter`（`core.Provider` + `core.StreamProvider`），elevenlabs 是 audio-only（`New` 為 nil）。identity / credential metadata / factories / static catalog 只存在於各自 `register.go` 的 `Entry` literal；每家實作哪些 optional capability、endpoint 與 wire 形狀見 [`docs/providers.md`](docs/providers.md) |

## 開發與驗證 (Development and Verification)

前置需求：Go `1.26+`；使用 provider adapter 時依該 module 的 API key/environment。

```bash
go work sync
go mod download
go test ./...                      # root module，含依賴紀律測試
bash scripts/verify-workspace.sh   # go.work 全部 module 的 build + test
```

依賴紀律不是文件裡的手動指令，是 `layering_test.go` 的三個測試：

| 不變式 | 測試 |
| ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| `agent/spec`、`prompt`、`prompt/source`、`agent/{permission,session,wire}` 的 agentsdk 依賴閉包只含 `core` 與自身 | `TestDeclarativeLayersOnlySeeCore` |
| `core` 只依賴 stdlib | `TestCoreImportsStdlibOnly` |
| 只有 `provider/credential` 可 import `github.com/bizshuk/auth` | `TestAuthImportedOnlyByProviderCredential` |

常用指令（provider 手動測試、wizard、sample、benchmark、依賴圖分析）集中於
[`docs/cli.md`](docs/cli.md)。

## 專案追蹤文件 (Project Tracking)

- 當前技術結構、ownership 與不變式：本檔。
- Provider adapter wire 細節：[`docs/providers.md`](docs/providers.md)。
- 指令與 flag 參考：[`docs/cli.md`](docs/cli.md)。
- 領域術語單一定義：[`docs/terminology.md`](docs/terminology.md)。
- 歷史變更與已完成里程碑：[`docs/CHANGELOG.md`](docs/CHANGELOG.md)。
- 尚未完成與刻意保留的工作：[`README.todo`](README.todo)。
- 仍在執行的落地計畫：`plans/`。
- 已實作規格的濃縮索引：[`docs/specs/2026-08-04-Summary.md`](docs/specs/2026-08-04-Summary.md)。

## 慣例與注意事項 (Conventions and Caveats)

- 常數使用 `SCREAMING_SNAKE_CASE`，變數、函式、型別使用 Go `MixedCaps`；package 名稱使用單字。
- 錯誤以 `fmt.Errorf("...: %w", err)` wrap；公開 error 不帶 credential、authorization、prompt 或未清理 upstream body。
- 測試採 table-driven + `t.Run`，`testify/assert` 與 `testify/require` 並用；`utils/testutil` 只可被測試使用。
- `core.Decide` 與 reasoning rules 不得直接做 I/O、讀 credential 或建立 HTTP request；這些責任屬於 runtime 與 provider adapter。
- `sample/logdoctor-agent/core` 與 `agentsdk/core` 是不同 module path；import 時使用 `domain` / `sdkcore` alias。
- 修改 package tree、module 或 protocol contract 後必須同步本檔；業務範疇變更才同步 `README.md`；provider adapter 的 wire 異動同步 `docs/providers.md`，指令異動同步 `docs/cli.md`。
