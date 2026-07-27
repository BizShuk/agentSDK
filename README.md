# agentsdk

Go Agentic Loop SDK：以`宣告式設定`組裝目標導向控制迴圈 (Goal-directed Control Loop)。應用層宣告`要開哪些能力`，SDK 負責`怎麼接`。

## 範疇 (Scope)

五大支柱對應到頂層 package,架構即文件:

| 支柱        | 套件                       | 角色                                                                                         |
| ----------- | -------------------------- | -------------------------------------------------------------------------------------------- |
| 1. 認知架構 | `core` (ObservationSource) | 觀察來源 port (Observations channel);原 `perception/` 套件無 consumer 已移除                 |
| 2. 系統韌性 | `memory/`                  | Window / Compactor / Checkpoint (M2)                                                         |
| 3. 工具生態 | `tool/`                    | `core.Tool` / RawMessage converter / Registry / allowlist-aware 內建工具 factory / Sandbox   |
| 4. 推理     | `reasoning/`               | `NewRule` + 6 種 DecisionRule (ReAct / Planner-Executor / Executor-Critic / CoT / Reflexion / Router) |
| 5. 組裝     | `agent/` + `agent/spec/`   | 宣告式 `Config` → 7 stage pipeline → `*agent.Engine`；`prompt/` 管進 context window 的內容   |

`core/` 是純狀態機 (state + event + instruction + reasoning),只依賴 stdlib,連 gosdk 都不 import。root module 的 `runtime/loop.go` 是 shell,負責 dispatch instructions 到綁定的 port (model / tools / store / notifier)。

模型執行介面刻意保持最小：`core.Provider` 只要求 blocking `Generate`；串流是 optional
`core.StreamProvider`，live model catalog 是 optional `core.ModelLister`。provider 名稱、認證 metadata
與 static catalog 由 `provider.Entry` 統一持有，不塞回 runtime client。

provider 建構只有一條 pipeline：`provider.Options`（尚未解析的 live input）經
`Options.Resolve(Entry.Metadata)` 變成 `provider.ResolvedConfig{Model, BaseURL, Auth}`，
再交給 adapter `New`。env / viper lookup 不進 adapter；`core.Auth` 只承載 credential 與
provider-specific headers，endpoint 固定是 construction config。

Google 與 Ollama 經跨 adapter golden tests 證明使用相同的 OpenAI Chat Completions
request、response 與 SSE wire contract，因此共用 internal codec
`provider/internal/openaichat`。其他 provider 保留各自 DTO，不以欄位相似作為合併依據。

## 怎麼用 (Getting Started)

engagement 是四階`階梯`，不是一堆獨立開關。每一階都是下一階的子集，往上爬只改設定不改 API。

| tier | 內容 | 典型場景 |
| --- | --- | --- |
| `oneshot` | 只有 provider，一次 model call | 嵌在服務裡跑一次分類 / 摘要 |
| `basic` | `+` 推理迴圈、middleware、state/WAL | 有記憶的對話 agent |
| `standard` | `+` 內建工具、permission、session、context files | 會動檔案的工作 agent |
| `full` | `+` skills、subagents、env/reminder prompt | 完整 coding agent |

最小接觸面——一行，沒有 Engine 概念、不需要應用名稱：

```go
out, err := agent.Once(ctx, agent.Config{Model: agent.Model{Provider: "minimax"}}, "ping")
```

完整應用——六行，`*agent.Agent` 實作 `agent.Runner`，直接插進既有 lifecycle：

```go
func main() {
    cli.Main(agent.MustNew(agent.Config{
        Name:  "my-agent",
        Tier:  "standard",
        Model: agent.Model{Provider: "minimax"},
    }))
}
```

嵌入其他 process 時自行建立 `agent.Host`，並直接處理 `agent.Run(...)` 回傳的
`error`；只有 `agent/cli` 將錯誤轉成 process exit code。

設定檔驅動——應用層完全不出現任何 harness package：

```go
cfg, err := agentconfig.LoadFile("agent.yaml")
cli.Main(agent.MustNew(cfg))
```

```yaml
name: code-agent
tier: full
model: {provider: anthropic, name: claude-sonnet-5}
reasoning: {style: plan_then_run}
safety:
  mode: acceptEdits
  deny: ["bash(sudo:*)"]
```

JSON、text 與 TUI 是 frontend 選擇，不進 `agent.Config`；composition root 以
`agent.WithSink(...)` 注入，或直接使用 `agent/wire`。

設定檔用 `wizard` 產生（逐階段挑選，Enter 收預設）：

```bash
go run . w                       # 互動，寫 ./agent.yaml
go run . w -y --tier full -o -   # 非互動，輸出 stdout
go run . w --list model.provider # 列出單一欄位的選項
```

### 兩層 opt-in

設定只有一條規則要記：

```text
層 1 feature：block 是 pointer —— 缺 key = 關；空物件 {} = 開且用預設
層 2 variant：block 內的具名欄位 —— 空字串 = 該 feature 的預設實作
```

`reasoning` 再多一層正交軸：`reasoning.enable` 決定`註冊哪些`策略，`reasoning.style` 決定`這次跑哪個`——需要跑到一半換策略（`choose_agent` 當 router）時才註冊多個。

### 什麼要用注入 (Option)

寫不進設定檔的東西一律走 `agent.Option`——這份清單就是應用層需要知道的全部接觸面：

| Option | 為什麼不能寫進設定 |
| --- | --- |
| `WithProvider` | 活物件（測試 fake、已建好的 client）；API key 是密鑰，不該進設定檔 |
| `WithToolRegistrar` / `WithToolFunc` | 應用自有工具的實作 |
| `WithHooks` | closure 安全閘：要看實際參數內容，任何 specifier pattern 都做不到 |
| `WithSources` | 自訂 prompt 內容來源 |
| `WithRules` | 超出內建六個的推理策略 |
| `WithSink` / `WithNotifier` | 呈現與通知的實作 |
| `WithCustomize` | 最終逃生艙：拿到組好的 `*agent.Engine` 再改 |

## 模組結構

```tree
agentsdk/
├── go.work                    # 9 modules：root + 8 samples（provider 已併回 root，tui 已下沉 code-agent）
├── go.mod                     # module github.com/bizshuk/agentsdk
├── main.go                    # cobra root binary;掛載 `provider` 與 `wizard` 兩個子指令
├── cmd/                       # root subcommands (provider: smoke-test CLI；wizard/w: 設定產生器)
├── agent/                     # 組裝層：Config → 7 stage pipeline → Engine；公開契約集中於 agent.go
│   ├── cli/                   # process host：signal、slog、os.Exit
│   ├── spec/                  # 宣告層：Config / Choice / tier 展開 / 驗證（只 import core）
│   ├── permission/            # permission rules × mode (allow/ask/deny specifier, deny > ask > allow)
│   ├── session/               # session 管理層 (list / resume / fork / tree; WAL JSONL 為 transcript 真相)
│   └── wire/                  # headless 表面: stream-json envelope / RPC framing / print formatter
├── prompt/                    # content management：Slot(system/user/reminder)、Source、Builder、LoadContextFiles（AGENTS.md/CLAUDE.md 階層載入）
├── core/                      # 純狀態機 (stdlib only, 含 ObservationSource port)
├── memory/                    # 支柱 2 (M2)
├── reasoning/                 # 6 thinking patterns
├── tool/                      # core.Tool bridge、RegisterFunc、Registry、Sandbox
│   └── builtin/              # Read/Write/Edit/Bash/Glob/Grep
├── middleware/                # (M2 鏈)
│   └── hook/                  # lifecycle hooks (PreToolUse/PostToolUse/...; command hook exit 2 = block)
├── runtime/                   # Loop: dispatch + checkpoint + WAL
├── skill/                     # SKILL.md skills + slash commands + prompt templates (progressive disclosure) + SubAgent/Spawner ("task" tool)
├── provider/                  # 7 個 adapter（Generate + Stream capability；已併回 root module）
│   ├── registry.go            # Entry 是 name / metadata / static catalog / factory 的唯一真相
│   ├── registry_options.go    # Options → ResolvedConfig 的唯一 env / credential resolution pipeline
│   └── internal/openaichat/   # Google/Ollama 共用且不暴露為 public API 的 wire codec
├── auth / proxy               # 外部獨立 repo，本 repo 無此目錄（auth 為 go.mod require，proxy 已完全脫離）
├── utils/                     # 根層共用 utilities umbrella：utils/frontmatter/ + utils/testutil/（FakeProvider / MemStore / CapturingNotifier）
├── sample/code-agent/         # 全 harness 組合 CLI（tui 互動 / -p / --json、session flags、.agentsdk 探索）
│   └── tui/                   # zero-dep differential-rendering terminal UI，不 import agentsdk（只有 agent 實作需要）
├── sample/logdoctor-agent/          # 驗證 sample (cobra CLI + 兩個 tool)
```

`auth` 與 `proxy` 都已脫離本 repo，是外部獨立 repo（無 `auth/`、`proxy/` 目錄，也無 `.gitmodules`）。`auth` 只由 `provider/credential` 使用；`proxy` 已無任何殘留（無目錄、無 require、無 import）。Root `main.go` 只掛載 root module 自己的兩個子指令：`provider`（wire-format smoke test，直接打 `core.Provider.Generate` / `core.StreamProvider.Stream`）與 `wizard`（設定產生器）。

## Proxy protocol bridge

> 範圍：`proxy` 已是外部獨立 repo，本節保留為協定橋接的設計說明，不對應本 repo 的任何目錄。

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
- 四個參考來源的 `37` 個 directed wire-format entity 與雙向 payload 範例見外部 repo `bizshuk/proxy` 的 `docs/specs/format/README.md`。

## 設計原則

- **核心純粹**:`core/` 零 vendor 依賴,可獨立發佈;所有 I/O 都在 `runtime/` 與 ports adapter
- **六種 ThinkingPattern**:`reasoning.NewRule` 擁有 style → implementation，`reasoning.NewDecide` 以純函式 DecisionRule dispatch；working memory 作為 pattern 與 runtime 間的通訊介面
- **Tagged union Instruction**:5 種 live instruction kind 透過 Kind discriminator + optional pointer 欄位表達,JSON round-trip 透過 `omitempty` 精簡
- **Notifier 結構性相容**: `core.Notifier` 介面方法集與 `gosdk/notify.Notifier` 完全相同,gosdk 的 Multi / Stdout / Slack 直接傳入,無需 adapter
- **Harness 能力可插拔**: hooks / permission / session / skill（內含 `SubAgent`/`Spawner`）/ wire / prompt 各自為只依賴 `core` 的 package,`runtime.Engine` 持有 nil 即 no-op 的 port,全部由 `agent` (composition root) 注入 — 借鏡 pi 的單向依賴與 claude-code 的 harness 事件面
- **宣告與組裝分離**: `agent/spec` 是純資料 (只 import `core`),任何只想`讀`或`產生`設定的工具 (wizard / schema generator / web 表單) 不必背上 provider SDK 與 harness 的重量;`agent` 才是知道那些實作存在的組裝層
- `wire codec` 只在 golden bytes、folded result 與 failure semantics 都相容時局部共用；vendor-specific protocol 保留自己的 DTO
- **presets, not walls**: 設定挑 preset 而非組合細節 (middleware 鏈的順序是正確性,不是偏好);`WithCustomize` 在全部 stage 之後拿到組好的 `*runtime.Engine`,任何設定詞彙沒覆蓋的都還做得到

## 執行範例

`sample/code-agent` — 全能力組合，composition 只有 `101` 行（宣告 `agent.Config` 而非手工接線）：

```bash
cd sample/code-agent
go run . --fake -p "看看這個專案"   # print 模式
go run . --fake --json -p "test"   # stream-json envelope
go run . --fake --sessions         # session 列表
go run . --fake                     # 互動 TUI
```

`sample/logdoctor-agent` — M1 e2e：

```bash
cd sample/logdoctor-agent
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

| Milestone | 範疇                                                                                                 | 狀態        |
| --------- | ---------------------------------------------------------------------------------------------------- | ----------- |
| M1        | 核心範式 + sample 骨架 (無 provider / 無 middleware / 無 dedupe)                                     | ✅ 完成     |
| M2        | 系統韌性 + 循環防禦 (memory / checkpoint / WAL / loopguard / retry)                                  | ✅ 完成     |
| M3        | 工具生態 + 執行期安全 (schema / sandbox / spotlight / sanitizer / tracing)                          | ✅ 完成     |
| M4        | 架構解耦 + HITL 完整 + 三個 LLM provider (anthropic / openaicompat / google)                         | ✅ 完成     |
| M5        | built-in tools、sample wiring、`agent` lifecycle                                                     | ✅ 完成     |
| M6        | auth mechanism、9 provider ids、auth CLI                                                             | ✅ 完成     |
| Proxy     | 3×3 pairwise protocol transform、provider profile routing、SSE hardening                             | ✅ 完成     |
| Format    | 四來源 `37` 個 client/provider wire-format entity catalog                                            | ✅ 完成     |
| Harness   | hooks / agent/permission / agent/session / skill（內含 subagent）/ agent/wire + steering queue（`contextfile` 已併入 `prompt.LoadContextFiles`；tui 已下沉 code-agent） | 🚧 skeleton |
| Agent     | 宣告式組裝：`agent/spec` + `agent` 7 stage pipeline + `prompt` + `provider`（registry）+ `wizard` 子指令 | ✅ 完成     |

詳細規格見 `docs/specs/` 與 `plans/`（proxy 規格已隨 repo 移出）,root milestone 實作完成後會轉為 `docs/specs/YYYY-MM-DD-<feature>.md`:

- `2026-07-04-core-paradigm-and-sample-skeleton.md` (M1)
- `2026-07-04-system-resilience-and-loop-defense.md` (M2)
- `2026-07-04-tool-ecosystem-and-runtime-security.md` (M3)
- `2026-07-04-architecture-decoupling-hitl-and-providers.md` (M4)

## 慣例衝突 (Naming Collision)

`agentsdk/core` (純狀態機) 與 `sample/logdoctor-agent/core` (gosdk noun 層領域邏輯) 撞名,屬不同 module path (`github.com/bizshuk/agentsdk/core` vs `github.com/bizshuk/agentsdk/sample/logdoctor-agent/core`),編譯安全。Sample 端 import 時以 `sdkcore` / `domain` 別名區分。

## 慣例

- 常數一律 `SCREAMING_SNAKE_CASE` (含 unexported、block-scoped),與 gosdk 一致
- `go.work` 多模組：目前 9 個 `use` entries（root + 8 samples；`provider/*` 已併回 root，`tui` 已下沉 `sample/code-agent/tui`）；`core/` 維持 stdlib-only，root config + agent 可使用應用層依賴
- 宣告層依賴紀律（CI 可驗）：`go list -deps ./agent/spec | grep agentsdk` 與 `go list -deps ./prompt | grep agentsdk` 都只該出現 `core` 與自己
- 依賴分析工具已移至獨立 repo `~/projects/go-dependency-analysis`：`go-dependency-analysis --workspace /Users/shuk/projects/ai/agentSDK/go.work --format text`
- 測試:table-driven + `t.Run` + `testify`
- 中文註解 + 英文關鍵字,遵循 `playground/CLAUDE.md` 慣例
