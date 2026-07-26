# CLAUDE.md — agentsdk 技術脈絡 (Technical Context)

`agentsdk` 是 Go Agentic Loop SDK、LLM protocol proxy、provider adapter、認證 CLI 與範例程式的 workspace。本文記錄目前程式碼真正採用的邊界與決策；舊的 `proxy/adaptor` one-to-one 設計已不再是 production path。

## 技術基準 (Current Baseline)

- 語言與 workspace：Go `1.26.0`、`go.work`，共 `9` 個 module entries（root + `8` 個 sample module）。`proxy` 與 `llm_provider` 均為外部 module dependency，不再列入本 workspace。Standalone dependency analyzer repo 仍位於 `~/projects/go-dependency-analysis`；in-tree prototype 已於 `19f0d41` 移除。`provider/*` 已於 `551410d` 併回 root module；`tui/` 於 2026-07-21 併回，不再各自帶 `go.mod`。2026-07-19 移除原 `cli/` + `mcp/` 兩個未對接的套件,並移除外部 `video-utils` 依賴 (`go.mod` 與 `cmd/` wiring 同步清掉)。同日落地 harness/UX skeleton：`hook`、`permission`、`session`、`contextfile`、`skill`、`subagent`、`wire` 七個 core-only package + `tui` sub-package（zero-dep） + runtime steering/follow-up queue；`contextfile` 於 2026-07-24 併入 `prompt`（固定行為、無客製化縫），計畫見 [`plans/2026-07-19-harness-ux-modularization.md`](plans/2026-07-19-harness-ux-modularization.md)、來源調查見 [`docs/memory/2026-07-19-agent-client-feature-catalog.md`](docs/memory/2026-07-19-agent-client-feature-catalog.md)。2026-07-22 落地 agent skeleton：`agent/`（含 `agent/spec` 宣告層）、`prompt/`、`provider/registry.go` 三個新 package + root `wizard` 子指令，計畫見 [`plans/2026-07-22-agent-skeleton-config-opt-in.md`](plans/2026-07-22-agent-skeleton-config-opt-in.md)。
- root module：`github.com/bizshuk/agentsdk`，內容為 SDK 核心群（core/planning/action/tool/memory/middleware/runtime）、harness 群（middleware/hook、permission、session、skill、wire、prompt，全部只依賴 core；2026-07-24 contextfile 併入 prompt 後已從獨立 package 移除）、組合層（agent/config）+ root CLI 子指令（`cmd/provider.go` 的 `provider` smoke-test 直接呼叫 `core.Provider.Generate/Stream` 不走 Engine；`cmd/agent/wizard.go` 的 `wizard`/`w` 產生 `agent.Config` 設定檔）。2026-07-24 `app/` 併入 `agent/`（lifecycle + Interactive seam 收在 `agent` 內），介面 `app.Agent` 改名 `agent.Runner`。`agent/spec` 是宣告層，只 import `core`；`agent` 才是知道 planning/provider/harness 全部存在的組裝層。`core/` 保持標準函式庫 only；`auth` 與 `proxy` 都已脫離本 repo，是外部獨立 repo（本 repo 無 `auth/`、`proxy/` 目錄，也無 `.gitmodules`）。root `main.go` 掛載 `cmd.NewProviderCommand()` 與 `cmd.NewWizardCommand()`，不掛載 auth/proxy 指令集；root module 仍透過 `config/` 直接 import `auth/model`、`auth/svc`、`auth/utils`，因此 `auth` 仍是 root 的 direct require，`proxy` 已從 root `go.mod` 移除。SDK 核心群不依賴兩者。
- 目前 proxy 架構：`protocol → route → transform → upstream`，三種 client wire format 的 `3×3` directed pair 已接上 handler。
- 來源與規格：現行 pairwise 決策見 `proxy/docs/specs/2026-07-16-pairwise-agent-provider-transform.md`（外部 repo）；四個來源的 wire-format 盤點見 `proxy/docs/specs/format/README.md`（外部 repo）。
- 外部依賴：`auth` 是外部 module（`github.com/bizshuk/auth`，go.mod require，非 submodule），只被 `config/provider.go`（`model`/`svc`）與 `config/app.go`（`utils.FileStore`）使用；`proxy` 已無任何殘留（無目錄、無 require、無 import）。`tmp/auth2api` 與 `tmp/cliproxyapi` 僅供格式研究，不是 runtime dependency。

## 專案結構 (Project Structure)

```text
agentsdk/
├── README.md                         # 業務範疇與使用者導覽
├── CLAUDE.md                         # 技術脈絡與架構決策（本檔）
├── go.work                           # root + analyzer + 7 sample modules (tui 已併回 root)
├── go.mod                            # github.com/bizshuk/agentsdk
├── main.go                           # cobra root binary；掛載 `provider` 與 `wizard` 兩個子指令
├── cmd/                              # root cobra subcommands
│   ├── agent/                        # agent 相關子指令
│   │   └── wizard/                   # `wizard`/`w` 設定產生器（逐階段 wizard prompt，產出 agent.Config）
│   └── provider.go                   # `provider` smoke-test（直接打 core.Provider.Generate/Stream）
├── agent/                            # 組裝層：spec.Config → 8 stage pipeline → *runtime.Engine；實作 agent.Runner；CLI lifecycle + Interactive 收在此處（2026-07-24 自 app/ 併入）
│   └── spec/                         # 宣告層：Config/Choice/tier 展開/驗證（只 import core，可獨立被讀取）
├── prompt/                           # content management：Slot(system/user/reminder)、Source、Builder、LoadContextFiles（AGENTS.md/CLAUDE.md 階層 + @import；2026-07-24 自 contextfile 併入）
├── prompt/source/                    # 內建 Source 實作：PersonaSource/ContextFileSource/EnvSource/ReminderSource + SkillSource（透過 SkillProvider interface 收 *skill.Registry，prompt/source 仍只 import prompt + stdlib）
├── config/                           # AppConfig、middleware presets、RefreshingProvider（runtime wiring，非宣告層）
├── core/                             # 純狀態機、Message/Part、Event、Instruction、ports（含 ObservationSource）
├── planning/                         # 6 個純函式 DecisionRule FSM
├── action/                           # tool registry、TypedTool/schema、sandbox、approval policy
├── tool/                             # Read/Write/Edit/Bash/Glob/Grep 內建工具
├── permission/                       # core.ApprovalPolicy 實作：Mode × allow/ask/deny specifier rules
├── session/                          # StateStore/WAL 之上的 session 管理：list/resume/fork/tree + meta sidecar
├── skill/                            # SKILL.md/commands/subagents registry（progressive disclosure + Def/Spawner "task" tool）
├── wire/                             # headless envelope：stream-json/RPC/print（core.EventSink adapter）
├── utils/                            # 根層共用 utilities umbrella：utils/frontmatter/（adrg/frontmatter YAML/TOML/JSON wrapper,key:value 攤平為 string map）+ utils/configfile/（副檔名決定編碼、一律以 JSON 呈現給 caller,故 `json` tag 是唯一真相）+ utils/testutil/（in-process fake provider/state store/notifier）
├── tui/                              # sub-package（zero-dep）：differential renderer、ANSI 工具、Component/Terminal 抽象
├── middleware/                       # chain、retry/timeout/budget/loopguard、安全與 OTel tracing；hook/ 子包（core.Hooks 實作：Rule matcher + Func/Command handler，exit 2 = block；以 middleware-style handler 連續執行 + HookDecision 合併，signature 仍獨立）
├── memory/                           # context window、compactor、checkpoint、JSON state/WAL
├── runtime/                          # Engine：dispatch Instruction、fold Event、Run/Resume/HITL
├── cmd/                              # root CLI 子指令；目前只有 `provider` smoke-test（Cobra 註冊進 main.go）
├── cli/                              # (已刪除) 9 種 JSONL Envelope 與 codec —— 無 production caller
├── auth 外部依賴                     # `github.com/bizshuk/auth`：獨立 repo + go.mod require（非 in-tree 目錄）
│                                     # config/provider.go 用 model/svc（Resolver），config/app.go 用 utils.FileStore
├── proxy 外部依賴                    # 已完全脫離本 repo：無目錄、無 go.mod require、無任何 import
├── mcp/                              # (已刪除) 獨立 module：MCP Client → action.ToolSource —— 無 production caller
├── provider/                         # package provider（2026-07-26 自 package registry 改名，目錄與 package 名終於一致，移除全部 import alias）
│   ├── registry.go                   # name → adapter 的唯一真相 + DEFAULT_NAME；env 查詢用注入（不綁 viper），CLI 與 agent 共用；adapter 以 init() 自我註冊
│   ├── adapter.go                    # Adapter = core.Provider + Name() + Metadata()；Metadata 的 OAuthEnv / APIKeyEnv 分離
│   ├── all/                          # meta-package：blank-import 全部 adapter 的便利入口
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
│   ├── file-agent/                   # 6 內建工具的檔案操作 agent
│   ├── greet-agent/                  # 內建工具 + greet tool
│   ├── logdoctor-agent/              # log listener、todo tools、watch/resume/approve CLI（唯一繞過 agent/ 的完整 agent，理由見 bd83a07）
│   ├── skeleton-agent/               # wizard --print-go 樣板:agent.Main(agent.MustNew(cfg)) + 12 行 stdinAgent 包裝
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
| State/schema              | `testify v1.11.1`、`invopop/jsonschema v0.14.0`     | table-driven tests、TypedTool JSON Schema                                                                                              |
| IDs/telemetry             | `google/uuid v1.6.0`、OpenTelemetry `v1.44.0`       | request ID、transform warning/loss metrics                                                                                             |
| Anthropic adapter         | `anthropics/anthropic-sdk-go v1.50.2`               | 只在 `provider/anthropic` module 引入                                                                                                  |
| Google adapter            | `google.golang.org/genai v1.62.0`                   | 只在 `provider/google` module 引入                                                                                                     |
| OpenAI-compatible adapter | `net/http` + JSON + SSE                             | `provider/openaicompat` 不依賴 vendor SDK                                                                                              |
| MCP                       | `modelcontextprotocol/go-sdk v1.6.1`                | `mcp` 獨立 module，轉成 `action.ToolSource`                                                                                            |
| Terminal UI               | Go stdlib only                                      | `tui` sub-package（zero-dep）；differential rendering、CSI 2026、不用 alternate screen                                                 |

`core/` 不 import `gosdk`、Gin、任何 provider SDK；auth、proxy、provider 與 MCP 的重依賴藉由獨立 `go.mod` 隔離。root module 的直接外部依賴縮減為 `gosdk`、OTel(trace)、`cobra`/`viper`、`jsonschema`、`uuid` 已隨 proxy 遷出；root 仍非 stdlib-only（組合層在此）。

## 核心架構決策 (Core Decisions)

- `core` 是純狀態與 transition contract。`core.Decide(state, event)` 不做 I/O，只回傳下一個 `State` 與 `[]Instruction`；runtime 才執行 model、tool、approval、notify、checkpoint。
- `State` 的對話模型是 `Message{Role, Parts}`；`Part` 支援 text、audio、image、tool use、tool result。JSON tags `scratch` 與 `thinking_kind` 保留以相容舊 state。
- `Instruction` 是 tagged union，透過 `Kind` + optional payload 表示 `call_model`、`call_tool`、`request_approval`、`notify`、`checkpoint`、`emit`、`done`；不建立 vendor-specific effect type。
- `runtime.Engine` 是 shell，維護 `core.Provider`、`ToolRegistry`、`StateStore`、`WriteAheadLog`、`Notifier` 與 middleware。`Middleware == nil` 代表 no-op；`config.DefaultMiddleware()` 與 `config.SecureMiddleware()` 由 composition root 明確選用。
- `config.DefaultMiddleware()` 順序為 `retry → timeout → budget → loopguard`；安全版本再加入 `sandbox → approval → spotlight → sanitizer`。工具輸出會被 spotlight 標為 untrusted，prompt injection 命中會被 sanitizer 改寫。
- WAL recovery 先載入 snapshot，再依 `LastInputSeq` replay Event；replay 不重新呼叫模型，避免 crash recovery 產生重複副作用。`memory/filestore` 使用 atomic state write + JSONL append。
- `planning/` 的六個 strategy（ThinkThenAct、PlanThenRun、RunThenReview、OneShotReasoning、LearnFromFailure、ChooseAgent）都是 phase FSM；working memory 是 rule 與 runtime/middleware 的通訊介面。
- `action.TypedTool` 用 `invopop/jsonschema` 反射 args schema，呼叫前做 required-field validation；`tool.RegisterDefaults` 集中註冊 Read/Write/Edit/Bash/Glob/Grep。高風險工具由 `DefaultApprovalPolicy` 依 L0–L4 gate。
- `mcp.Client` 只做 MCP `ListTools`/`CallTool` 與 `core.ToolSpec`/`ToolResult` 轉換，不把 MCP transport 混入 core 或 runtime。
- Harness ports（2026-07-19，借鏡 claude-code harness + pi 模組化）：`core` 新增 `Hooks`（`HookEvent`/`HookDecision`）與 `EventSink`（`StreamEvent`）兩個純資料 port；`runtime.Engine` 持有 nil 即 no-op 的 `Hooks`/`Sink` 欄位，`PreToolUse` block 會折成失敗 `ToolResult` 讓 model 看到拒絕、`PostToolUse` 的 `SystemNote` 追加為 system message。所有 harness package 只依賴 `core`，由 composition root 注入。
- `permission.Engine` 是兩軸決策（codex 式）：`Mode`（`default`/`acceptEdits`/`plan`/`bypassPermissions`）× rule specifier（`Bash(git:*)`、`Edit(src/**)`；自寫 `**` matcher，因 `filepath.Match` 的 `*` 不跨 `/`），優先序 `deny > ask > allow`；無 rule 命中時可注入 `Fallback`（如 `action.DefaultApprovalPolicy` 的 autonomy grid）。
- Agent skeleton（2026-07-22）：`spec.Config` 是宣告式資料，`agent` 是組裝層。opt-in 有兩層——層 `1` feature 開關（block 是 pointer，`nil` = 關、`{}` = 開且用預設，剛好對應 JSON 的「缺 key / 空物件」），層 `2` variant 選擇（block 內具名字串）。`planning` 再多一層正交軸：`Reasoning.Enable` 決定註冊哪些 rule 進 `NewDecide` 的 map，`Reasoning.Style` 決定這次 seed 哪個——未註冊的 style 會讓 `NewDecide` 回 `NOTIFY error`，故 `spec.Validate` 早報。
- Tier 階梯（`oneshot`/`basic`/`standard`/`full`）是 block 集合的單調展開簡寫，展開後顯式 block 覆蓋（explicit wins）。`tier` 與 `reasoning` 正交，組合`不`視為衝突：`T0` 無工具 → provider 收不到 tool spec → `ModelResult.ToolCalls` 恆空 → `runtime/loop.go` 的 short-circuit 在 `Decide` 前收成 `COMPLETED`，任何 strategy 到 `T0` 都退化成一次 model call。`T0` 預設關持久化，否則 `config.OpenForCLI` 的空 appName 檢查會讓 `Name` 從「`T1+` 必填」變成「永遠必填」。
- `agent.Once` 不繞過 Engine：它用 `planning.OneShotReasoning` + 全 nil port 走同一條路。不得改寫成「每次回 `[CALL_MODEL, DONE]`」的 no-op `Decide`——那會破壞 `core.Decide` 的純函式不變式，retry/WAL replay 會重發 model call（理由見 `planning/one_shot.go` 型別註解）。
- 注入用 `agent.Option`（`func(*builder) error`），不用 `Deps` struct：沿 repo 既有慣例（`app` + 7 個 provider adapter 共 `8` 處 `type Option func(*x)`）。`Option` 是 closure、不可列舉、只活在本 process；`spec.Choice` 是資料、可列舉、要跨序列化邊界寫進設定檔——兩者不可合併，只在 wizard 的 `--print-go` 輸出端相遇。
- `prompt` 擁有「這一輪送什麼進 context window」，`memory` 擁有「放不下時砍什麼」：前者是 policy、後者是 mechanism，合併會讓注入與裁切互相遞迴。`SLOT_SYSTEM` 內順序由不變到易變（persona `10` → context files `20` → skill 索引 `30` → env `40`），因為 prompt caching 要求前綴穩定；預算超限時從尾端整段丟，不切半。`skill` 不知道 `prompt` 存在，adapter 住在 `prompt/source`（sub-package，透過 `SkillProvider` interface 而非型別耦合）；context-file 載入已併入 `prompt.LoadContextFiles`（固定行為、無客製化縫）。
- Steering / follow-up queue（pi 式）：`Engine.Steer` 在下一次 Decide 前插入 user message；`Engine.FollowUp` 把「本應完成」轉為續跑（每次一則）。follow-up 續跑會清掉 `think_then_act.phase` 讓 FSM 回到 reason（與 loop 既有 pending_call seeding 同一 seam）；其他五個 rule 的通用 reset 慣例待補。
- Tool-call batch settlement（2026-07-24）：一個 assistant message 的 `N` 個 `tool_use` part，必須在下一次 `CALL_MODEL` 前對應到 `N` 個 `tool_result`，否則 Anthropic-format provider 回 `400` 且 model 把缺結果讀成「還在跑」。`runtime` 播種整批 `ToolCalls` 進 `think_then_act.pending_calls`（`ThinkThenAct` 的 dispatch phase 一次發 `N` 個 `CALL_TOOL`），未執行者（pause / hook block / budget skip）由 `settleSkipped`/`settleUnrun` 補上失敗 `tool_result`。`planning.decodeCalls` 依形狀解碼 working memory，避免 JSON round-trip 後 pending call 消失（crash 後 Resume 靜默完成的 bug）。
- Round / MaxToolCalls budget（2026-07-24）：`round` = 一次 `CALL_MODEL` 及其 tool call（使用者面），`Budget.MaxRounds` 上限、`UsedRounds` 在 `CALL_MODEL` 遞增；`turn` = `Decide` 次數（loopguard 用），兩者不混。`MaxToolCalls` 限單 round 批量：超限`整批 skip + settle` 並暫停於 `continue-gate`（`ToolCall == nil` 的 `PendingApproval`），approve → resume 讓 model 重讀重新規劃（不重發原批次），reject → `COMPLETED`。
- `agent.Interactive`（2026-07-24）：`NextRound(ctx, Pause) (Resume, error)` 是唯一互動縫，`PAUSE_APPROVAL`（含 continue-gate）與 `PAUSE_ROUND_END`（follow-up）共用。`agent.Run` 在 `safeRun` 後 loop 呼叫，`advance` 只走 `SubmitHumanDecision`（不再 `Resume`，避免 WAL replay 重複執行）、follow-up 走 `Steer`+重開。不實作 `Interactive` = 維持外部 verb 語意。收斂自三介面草案（`PauseHandler`/`ApprovalResolver`/`RejectionHandler`），計畫見 [`plans/2026-07-24-round-batch-and-interactive-seam.md`](plans/2026-07-24-round-batch-and-interactive-seam.md)。
- `session.Manager` 只加 lineage（meta sidecar：`ID`/`Parent`/`Title`/`Cwd`）與 fork（複製 state+WAL、改 RunID）；transcript 真相仍是 WAL JSONL。
- `wire` 是 `cli/` 的復活但有真 caller path：`runtime → core.EventSink → wire.Sink → JSONL`；Envelope 欄位是對外 API，需維持 JSON round-trip 穩定。
- `tui` sub-package（zero-dep）且不 import agentsdk：caller 把 `core.StreamEvent` 轉成 component 更新；不用 alternate screen、CSI 2026 synchronized output、frame 未變零輸出。
- `provider.Adapter` (2026-07-24)：`provider` 內定義的 adapter 完整契約 = `core.Provider` + `Name()` + `Metadata()`。`Metadata` struct 承接原本 `Entry` 上的 `Label/Note/APIKeyEnv/BaseURLEnv`，credential 解決 `Options.Resolve(m Metadata)` 只讀 metadata 不碰整個 `Entry`。每個 `provider/<name>/` 以 package-level `adapterMetadata()` 函式作為 single source of truth（每次回傳新 slice,避免外部突改 `APIKeyEnv`），同時被 `Entry.Metadata` 與 `Provider.Metadata()` 引用，使兩處無法 drift。`var _ registry.Adapter = (*Provider)(nil)` 移到 `register.go` 內掛保證,既涵蓋 `core.Provider` 也涵蓋 `Name`+`Metadata`；OAuth 走 `NewWithOAuth` 的 constructor 不走 registry,仍能從 `adapterMetadata()` 拿到一致描述。設計見 [`plans/modular-frolicking-key.md`](plans/modular-frolicking-key.md)。
- 分層修正三則 (2026-07-26)：`provider` 目錄下的 package 由 `registry` 改名為 `provider`,目錄與 package 名一致,移除全部 `registry "…/provider"` import alias。`core.DefaultProvider`（vendor 名 `"minimax"`）移出 `core` 成為 `provider.DEFAULT_NAME` —— 純狀態機不該因為換預設廠商而改動,而宣告層 `agent/spec` 本來就看不到哪些 adapter 被 link 進來,所以 `spec.Expand` 不再填 `Model.Provider`,空字串一路留到 `provider.Lookup` 才解析。`core.CredentialKind*` 保留在 `core` 但改為 `CREDENTIAL_KIND_*` 以符合全 repo 常數慣例（它們是 `core.Provider.AuthSchemes` 的回傳詞彙,不是 vendor 知識）。
- prompt 內建 Source 歸位 + SkillSource 下降 (2026-07-26)：`PersonaSource`/`ContextFileSource`/`EnvSource`/`ReminderSource`（含 `gitBranch` shell-out）自 `agent/sources.go` 移入 `prompt/sources.go`。判準是「寫這個東西需不需要同時知道兩個 package 存在」——只需要 `prompt` + stdlib 的是 content 職責,屬於 `prompt`;`agent/sources.go` 因此只剩 `SkillSource`（唯一真正的跨套件配接,因為 `skill` 與 `prompt` 互不相見）與 `BuildSources` 的 name→Source dispatch,214 → 128 行。`prompt` 仍只 import `core`。同時把 skills / commands / agents 三處各自複製的「userDir 優先、projectDir 覆蓋」目錄走訪收斂成 `discoveryRoots(cfg, userDir, cwd, kind)`。接著把 `prompt/sources.go` 五個 Source（含 `agent.SkillSource` 與新增的 `SkillProvider` interface）一同移入 `prompt/source/` sub-package，每個 Source 一檔；`prompt/source` 不 import `skill` 也不 import `agent`，只透過 `SkillProvider` interface 收 `*skill.Registry`。`agent/sources.go` 因此只剩 `BuildSources` 的 name→Source dispatch（含 typed-nil `*skill.Registry` → true-nil interface 的轉換）、`promptUserDir`、`discoveryRoots`、`discoverSkills`。`prompt/source` 仍只 import `prompt` 與 stdlib。
- sample 兩類命名 (2026-07-26)：目錄前綴即分類——`demo-*` 是手接單一 SDK 元件的展示（`demo-memory`/`demo-middleware`/`demo-strategy`,不經 `agent/`）,`*-agent` 是完整 agent。分類看「是什麼」不看「怎麼建的」,所以繞過 `agent/` 的 `logdoctor-agent` 仍歸完整 agent 一類。

- `provider.Metadata` 拆 OAuthEnv / APIKeyEnv (2026-07-26)：原本以 `APIKeyEnv []string` 隱含 OAuth-first precedence,現拆為兩個明確欄位讓 `Options.Resolve` 區分 strict 模式。新增 `Options.CredentialKind` (`""` / `"auto"` / `"api_key"` / `"oauth"`)：空字串保留舊 precedence,strict 模式只查對應 env,缺失 → `provider.Options.Resolve` 回 error 並由 `provider.New` 包進既有錯誤格式。三層 (`provider cmd --credential-kind` flag / `agent/spec.Model.CredentialKind` YAML 欄位 / `provider.Options.CredentialKind` 內部選項) 共用同一組常數 `core.CREDENTIAL_KIND_AUTO/_APIKEY/_OAUTH`,確保 doc、schema、cmd、registry 不會各自漂移。常數留在 `core` 是因為它們正是 `core.Provider.AuthSchemes` 回報的詞彙——port 定義在哪,它說的話就定義在哪;這也讓 `agent/spec` 維持只 import `core`。7 個 adapter 的 `adapterMetadata()` 同步更新:anthropic 同時擁有 OAuthEnv + APIKeyEnv,其他僅 API key path;codex OAuth 走 `NewWithOAuth` constructor 不走 provider registry env,strict oauth 會回 "not OAuth-capable"。設計見 [`plans/validated-meandering-rabbit.md`](plans/validated-meandering-rabbit.md)。

## 認證與 provider 決策 (Auth and Providers)

> 範圍：本節描述`外部 repo` [`github.com/bizshuk/auth`](https://github.com/BizShuk/auth) 的設計，不是本 repo 的目錄。本 repo 只透過 `config/` 消費它的 `model`/`svc`/`utils`。

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
- `Credential.BaseURL` 隨 API key 保存，gateway/proxy 憑證 verify 與後續 request 必須回到同一端點。
- `auth.Resolver` 是共用的 credential 解析機制：active.json 選取 → 同 provider 字母序 fallback → 環境變數 fallback（`DefaultEnvironmentNames`，含 ollama/llmbox）→ 過期自動 refresh 並持久化。active.json 讀寫慣例由 `auth.LoadActiveNames`/`SaveActiveName` 統一。
- proxy 的 `upstream.CredentialResolver` 是 `auth.Resolver` 的 thin adapter，只把 `auth.UnavailableError` 映射為 `credential_unavailable` ProxyError；runtime 路徑由 `config.NewRefreshingProvider` 包裝任意 `core.Provider`，每次呼叫前 resolve/refresh，token rotation 時重建 inner provider。
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

`main.go` 直接建立 Cobra root（binary 名稱 `agentsdk`，版本 `0.1.0`），掛載兩個子指令：`cmd.NewProviderCommand()` 的 `provider`（直接呼叫 `core.Provider.Generate`/`Stream`，不走 Agent/Engine/harness，用於 provider adapter 的 wire-format smoke test），與 `cmd.NewWizardCommand()` 的 `wizard`（alias `w`）——逐階段產生 `agent.Config` 設定檔，階段序列刻意對齊 `agent` 的 8 stage 組裝順序，選項全部來自 `spec` 與 `agent.ProviderChoices()`，wizard 本身不持有任何詞彙。Root 不再掛載 `auth/cmd.Install` 或 `proxycmd.NewCommand()`——auth CLI 與 proxy CLI 都從各自 module root 各自 `go build .` 建出獨立 binary，與 root binary 共用相同設定與憑證目錄。auth 函式庫在 `auth/model`、`auth/svc`、`auth/utils`、`auth/provider`，proxy 函式庫在 `proxy/handlers`、`proxy/config`、`proxy/model`、`proxy/svc`。`config.OpenForCLI(appName, level)` 為 sample 建立：

```text
~/.config/<appName>/
├── data/
│   ├── states/<runID>.json
│   ├── wal/<runID>.jsonl
│   └── auth/*.json
└── logs/<runID>.log
```

`agent.Run` 是 CLI agent 的共同 lifecycle：config → optional preflight → wall-clock deadline → Bootstrap → panic-safe Engine.Run → optional OnComplete。`agent.Main` 只負責 signal binding 與 exit code。

Proxy defaults（`proxy/config.go`）：port `8317`、body limit `200 MB`、model timeout `120s`、stream timeout `600s`、count-tokens timeout `30s`、stats enabled、debug off；未設定 API key 時在記憶體產生 `sk-...`，設定持久化由上層 command 負責。

JSONL 對外 envelope (`cli/`) 於 2026-07-19 移除：原 9 種 type (`observation`、`assistant`、`tool_call`、`tool_result`、`approval_request`、`human_decision`、`checkpoint`、`result`、`error`) 無 production caller；外部 wire-format 需求改由 sample 自訂 codec 承接，或待新需求再決定是否以獨立 module 重啟。不得把內部 `State` 欄位直接當成穩定外部 API，新增欄位需維持 JSON round-trip。

## 模組對應 (Module Mapping)

| 領域                      | 套件 / 進入點                                                                                                                                                                                                                                                                                       |
| ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 宣告式設定                | `agentsdk/agent/spec`：`Config`、`Choice`、`Expand`（tier 展開）、`Validate`、`Prepare`、`Decode`/`Encode`（只 import `core`）                                                                                                                                                                      |
| agent 設定檔 I/O          | `agentsdk/utils/agentconfig`：`LoadFile`/`SaveFile`/`Marshal`/`FormatOf`、`Format`/`FORMAT_YAML`/`FORMAT_JSON`（re-export 自 `utils/configfile`）；`LoadFile` = `configfile.ReadJSON` → `spec.DecodeBytes`、`SaveFile` = `spec.EncodeBytes` → `configfile.Write`                                                              |
| agent 組裝                | `agentsdk/agent`：`New`/`MustNew`/`Bootstrap`/`Preflight`（實作 `agent.Runner`）、`Once`/`OnceStream`、`Option` 全部 `With*`、`BuildSources`、`ProviderChoices`、`Main`/`Run`（CLI lifecycle、`Interactive` 互動縫）              |
| prompt 內容管理           | `agentsdk/prompt`：`Slot`（system/user/reminder）、`Section`、`Source`、`Builder.Seed`/`Turn`、`Static`、`PersonaSource`/`ContextFileSource`/`EnvSource`/`ReminderSource`                                                                                                                                                                                             |
| 狀態與 ports              | `agentsdk/core`：`NewDecide`、`State`、`Instruction`、`Provider`、`CREDENTIAL_KIND_*`                                                                                                                                                                                                                               |
| 推理策略                  | `agentsdk/planning`：6 個 `New*` DecisionRule constructor                                                                                                                                                                                                                                           |
| runtime                   | `agentsdk/runtime`：`NewEngine`、`Run`、`RunWithEvent`、`Resume`、`SubmitHumanDecision`                                                                                                                                                                                                             |
| tools/safety              | `agentsdk/action`、`agentsdk/tool`：`NewRegistry`、`NewTypedTool`、`RegisterDefaults`                                                                                                                                                                                                               |
| memory                    | `agentsdk/memory`、`memory/checkpoint`、`memory/filestore`                                                                                                                                                                                                                                          |
| lifecycle hooks           | `agentsdk/middleware/hook`：`NewRunner`、`Rule`、`Func`、`Command`（實作 `core.Hooks`；lifecycle event 為 fan-out + decision merge，handler 串以 middleware-style 連續執行，但 signature 仍獨立）                                                                                                                                                                       |
| permission                | `agentsdk/permission`：`Engine`、`Rule`、`MatchSpec`（實作 `core.ApprovalPolicy`）                                                                                                                                                                                                                  |
| session 管理              | `agentsdk/session`：`NewManager`、`Begin`、`List`、`Latest`、`Fork`、`Tree`                                                                                                                                                                                                                         |
| context files             | `agentsdk/prompt`：`LoadContextFiles(cwd, userDir)`（AGENTS.md/CLAUDE.md 階層 + `@import`；2026-07-24 自 contextfile 併入，無 Loader 結構、無 config knob）                                                                                                                                                       |
| skills/commands/subagents | `agentsdk/skill`：`NewRegistry`、`DiscoverSkills`、`Body`、`ExpandCommand`、`RenderTemplate` + subagent 入口（`ParseDef`、`DiscoverDefs`、`NewSpawner`、`Depth`/`WithDepth`）；源碼分為 `skill.go`/`command.go`/`registry.go`/`subagent.go` 四檔                                                    |
| headless wire             | `agentsdk/wire`：`Envelope`、`NewEncoder`/`NewDecoder`、`NewSink`、`ReadRequest`/`WriteResponse`、`FormatStream`                                                                                                                                                                                    |
| terminal UI               | `agentsdk/tui`（sub-package, zero-dep）：`Renderer`、`Component`、`Terminal`、`VisibleWidth`/`WrapText`                                                                                                                                                                                             |
| dependency graph          | external CLI `github.com/bizshuk/go-dependency-analysis`：Go tooling facts + JSON policy heuristics；不加入本 workspace、不被本 repo import                                                                                                                                                         |
| middleware                | `agentsdk/middleware`、`harness`、`loopguard`、`security`、`observability`                                                                                                                                                                                                                          |
| agent/config              | `agentsdk/agent`（CLI lifecycle 收在此處）+ `agentsdk/config`：`agent.Run`、`OpenForCLI`、`SecureMiddleware`、`NewRefreshingProvider`；`agent.Interactive`、`Pause`/`Resume`/`PauseReason`、`WithRoundTimeout`                                                              | <!-- markdownlint-disable-line MD060 -->
| provider registry         | `agentsdk/provider`（package `provider`，非 `registry`）：`Names`/`Entries`/`Lookup`/`New`/`Options.Resolve`/`DEFAULT_NAME`；`env` 查詢以 `LookupEnv` 注入，CLI 傳 viper-backed、library 用 `os.Getenv`                                                                                                                                |
| root CLI subcommands      | `agentsdk/cmd`：`NewWizardCommand`（`wizard`/`w` 設定產生器）、`NewProviderCommand`（root cobra `provider` smoke-test CLI；打 `core.Provider.Generate`/`Stream` 不走 Engine；registry 掛 minimax/anthropic/google/grok/ollama;`--list-models` 優先打 live `core.ModelLister`,失敗 fallback static） |
| authentication            | `github.com/bizshuk/auth/{model,svc,utils,provider}`（Git submodule + 獨立 module，root 有 `auth` binary main）：`Login`、`For`、`FileStore`、`NewResolver`                                                                                                                                         |
| proxy                     | `agentsdk/proxy/handlers`、`agentsdk/proxy/config`、`agentsdk/proxy/model`、`agentsdk/proxy/svc/{route,transform,upstream}`（獨立 module，root 有 `proxy` binary main）：`handlers.New`、`config.LoadConfig`、`model.Format`、`svc/route.Router`                                                    |
| JSONL                     | (已移除 2026-07-19) 原 `agentsdk/cli`：`Envelope`、`JSONLCodec`                                                                                                                                                                                                                                     |
| MCP                       | (已移除 2026-07-19) 原 `agentsdk/mcp` 獨立 module：`mcp.NewClient` 實作 `action.ToolSource`，目前無 caller；若日後需要 MCP，重新以獨立 module 落地並補上 wiring                                                                                                                                     |
| provider adapters         | `agentsdk/provider/{anthropic,google,minimax,grok,ollama,codex,antigravity}`：各實作 `registry.Adapter` (`core.Provider` + `Name()` + `Metadata()`)；除 codex/antigravity 外皆另實作 `core.ModelLister`（live `GET /models`），失敗時 caller fallback 回 `DefaultCatalog()` static list；每個 adapter 以 package-level `adapterMetadata()` 函式作為 `Entry.Metadata` 與 `Provider.Metadata()` 的 single source of truth                  |
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
for mod in . sample/code-agent sample/file-agent sample/greet-agent sample/logdoctor-agent \
  sample/demo-memory sample/demo-middleware sample/skeleton-agent sample/demo-strategy; do
  (cd "$mod" && go build ./... && go test ./... -count=1 -timeout=120s)
done

# 依賴紀律：兩個宣告層 package 只准看到 core
go list -deps ./agent/spec | grep agentsdk   # 只該有 core 與 agent/spec
go list -deps ./prompt     | grep agentsdk   # 只該有 core 與 prompt 與 prompt/source
go list -deps ./prompt/source | grep agentsdk # 只該有 prompt 與 prompt/source（不該出現 skill/agent/core 在 production code）

# core 不得知道任何 vendor 名稱（預設 provider 住在 provider.DEFAULT_NAME）
grep -rn 'minimax\|anthropic\|openai\|google\|ollama\|grok' core/*.go
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

cd sample/logdoctor-agent
go run . --fake --max-turns=10 run --once --fixture testdata/error.log

cd sample/code-agent
go run . --fake -p "看看這個專案"        # print 模式（進度走 stderr）
go run . --fake --json -p "test"        # stream-json envelope（wire）
go run . --fake                          # 互動 TUI（tui sub-package；執行中輸入 = Steer）
go run . --fake --sessions               # 列出本目錄 sessions；-c / -r / --fork 續跑

# 真實 provider：--provider 選 adapter，各自讀自己的 env（--fake 拿掉）
export MINIMAX_API_KEY=...                # minimax adapter 預設就讀這個
go run . -p "explore this repo"           # 預設 --provider minimax、model=MiniMax-M3
go run .                                   # 互動 TUI 打真實 model
go run . --provider anthropic -p "..."    # 改讀 ANTHROPIC_API_KEY（含 minimax gateway：--base-url）
```

`code-agent` 的 provider 選擇：`--provider minimax`（預設，讀 `MINIMAX_API_KEY`/`MINIMAX_BASE_URL`，`provider/minimax` stdlib adapter）或 `--provider anthropic`（讀 `ANTHROPIC_API_KEY`）；`--model` 留空用 adapter flagship 預設；`--api-key`/`--base-url` 為顯式覆寫。

`sample/logdoctor-agent` 的 real provider 旗標為 `anthropic`、`openaicompat`、`google`；`--fake` 與 `--provider` 互斥。`sample/file-agent` 與 `sample/greet-agent` 使用 Anthropic-compatible adapter 與 `SecureMiddleware`。

`sample/skeleton-agent` 是 `cmd/agent/wizard.go::goLiteral` 輸出範本逐字對應的單檔 sample:沒有 cobra、沒有四種 dispatch mode、不需要 `*Parts.Sessions`/`*Parts.Skills`。差別在 main 比 code-agent 少 `~260` 行,只為了展示 wizard `--print-go` 模板可直接落地;唯一的 12 行 `stdinAgent` 包裝是為了把 stdin 內容塞進 Bootstrap 回傳的 opening state。對比 shape 見 `sample/skeleton-agent/README.md`。

## 目前狀態與待辦 (Status and Open Items)

| 項目                                                                                                                           | 狀態                                                                                                                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| M1 核心範式與 sample                                                                                                           | 完成                                                                                                                                                                   |
| M2 state/WAL/checkpoint/retry/timeout/loopguard                                                                                | 完成                                                                                                                                                                   |
| M3 tool schema、sandbox、MCP、spotlight、sanitizer、tracing                                                                    | 完成                                                                                                                                                                   |
| M4 HITL、security wiring、三個 provider adapter                                                                                | 完成                                                                                                                                                                   |
| M5 built-in tools、sample wiring、`app` lifecycle                                                                              | 完成                                                                                                                                                                   |
| M6 auth mechanism、9 provider ids、auth CLI                                                                                    | 完成；`config.NewRefreshingProvider` 已補上呼叫前自動 refresh                                                                                                          |
| Proxy 3×3 pairwise cutover 與安全 hardening                                                                                    | 完成（現行 branch）                                                                                                                                                    |
| 四來源 37 entity wire-format catalog                                                                                           | 完成，見 `proxy/docs/specs/format/README.md`（外部 repo）                                                                                                              |
| module 拆分：`auth`、`proxy` 獨立 module；`config` 解體；`perception/` 刪除                                                    | 完成，見 [`plans/2026-07-18-architecture-module-split-roadmap.md`](plans/2026-07-18-architecture-module-split-roadmap.md)                                              |
| Agent skeleton：`agent/spec`（宣告）+ `agent`（組裝）+ `prompt`（content management）+ `provider`（registry）+ `wizard` 子指令 | 完成（`M1`–`M7` 全落地，`compose.go` `333` → `101` 行）；計畫見 [`plans/2026-07-22-agent-skeleton-config-opt-in.md`](plans/2026-07-22-agent-skeleton-config-opt-in.md) |
| Harness/UX skeleton：`hook`/`permission`/`session`/`skill`/`wire` + `tui` sub-package + steering queue（`contextfile` 於 2026-07-24 併入 `prompt`）           | skeleton 完成（全部含測試），細節項見 `README.todo`；計畫見 [`plans/2026-07-19-harness-ux-modularization.md`](plans/2026-07-19-harness-ux-modularization.md)           |

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
- `core.Decide`、planning rules、transform pair 不得直接做 I/O、讀 credential 或建立 HTTP request；這些責任分別屬於 runtime、upstream 與 auth。
- `sample/logdoctor-agent/core` 與 `agentsdk/core` 是不同 module path；import 時使用 `domain` / `sdkcore` alias。
- `proxy/docs/specs/2026-07-16-client-llm-adaptor.md` 是 legacy historical design；修改 proxy 時以 pairwise spec、現行 `proxy/` code 與測試為準。
- 修改 package tree、module、路由或 protocol contract 後，必須同步本檔；業務範疇變更才同步 `README.md`。格式 catalog 的 entity/來源異動則同步 `proxy/docs/specs/format/README.md`（外部 repo）。
