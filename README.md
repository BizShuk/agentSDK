# agentsdk

Go Agentic Loop SDK：以`宣告式設定`組裝目標導向控制迴圈 (Goal-directed Control Loop)。應用層宣告`要開哪些能力`，SDK 負責`怎麼接`。

## 範疇 (Scope)

五大支柱對應到頂層 package,架構即文件:

| 支柱        | 套件                       | 角色                                                                                         |
| ----------- | -------------------------- | -------------------------------------------------------------------------------------------- |
| 1. 認知架構 | `core` (ObservationSource) | 觀察來源 port (Observations channel)                                                          |
| 2. 系統韌性 | `memory/`                  | Window / Compactor / Checkpoint                                                               |
| 3. 工具生態 | `tool/`                    | `core.Tool` / RawMessage converter / Registry / allowlist-aware 內建工具 factory / Sandbox   |
| 4. 推理     | `reasoning/`               | `NewRule` + 6 種 DecisionRule (ReAct / Planner-Executor / Executor-Critic / CoT / Reflexion / Router) |
| 5. 組裝     | `agent/` + `agent/spec/`   | 宣告式 `Config` → 7 stage pipeline → `*agent.Engine`；`prompt/` 管進 context window 的內容   |

`core/` 是純狀態機 (state + event + instruction + reasoning),只依賴 stdlib,連 gosdk 都不 import。root module 的 `runtime/loop.go` 是 shell,負責 dispatch instructions 到綁定的 port (model / tools / store / notifier)。

圖片生成是 `provider.ImageGenerator` optional capability，不進 agent runtime 的
`core.Provider`。caller 必須明確走 `NewImage`；不支援的 adapter 回傳可用
`errors.Is(err, provider.ErrUnsupportedCapability)` 判斷的錯誤：

```go
generator, err := provider.NewImage("grok", provider.Options{})
if err != nil {
	return err
}
result, err := generator.GenerateImage(ctx, provider.ImageRequest{
	Prompt:         "新加坡雨夜的電影感街景",
	ResponseFormat: "b64_json",
})
```

binary 仍需 blank-import 目標 adapter（或 `provider/all`）讓它註冊。URL result 可能是
短效連結；要持久化時由 caller 複製資產。

可執行的 [`provider/sample`](provider/sample/README.md) 直接展示 provider、auth mode 與
`chat / image / audio` API type matrix。`audio` 目前刻意回 typed unsupported：audio
尚未決定是 speech synthesis、transcription 或 audio-chat，也沒有 adapter wire consumer。

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

五大支柱對應到頂層 package（見上表）。完整目錄樹、每個 package 的 ownership 與
架構不變式由 [`CLAUDE.md`](CLAUDE.md) 擁有，不在此重複——重複的兩份樹已經開始分岔。


## 執行範例

`sample/code-agent` — 全能力組合，以宣告 `agent.Config` 取代手工接線：

```bash
cd sample/code-agent
go run . --fake -p "看看這個專案"   # print 模式
go run . --fake --json -p "test"   # stream-json envelope
go run . --fake --sessions         # session 列表
go run . --fake                     # 互動 TUI
```

`sample/log-agent-v2` — scheduler 先建立 batch，再透過 `agent.WithListener`
進入完整 agent lifecycle：

```bash
export MINIMAX_API_KEY=...
go run ./sample/log-agent-v2 --interval 1m
```

第一次掃描會先等待一分鐘；每個非空 batch 都使用新的 `agent.Run`，成功後才
提交 cursor。完整行為見
[`sample/log-agent-v2/README.md`](sample/log-agent-v2/README.md)。

`sample/logdoctor-agent` — 比較用的精簡 `agent.OnceStream` 路徑：

```bash
cd sample/logdoctor-agent
export MINIMAX_API_KEY=...
go run . watch
```

診斷 Markdown 寫入 stdout；canonical `core.StreamEvent` JSONL 寫入 stderr：

```json
{"kind":"run_start","run_id":"once-..."}
{"kind":"message","run_id":"once-...","turn":1,"text":"# Diagnosis\n..."}
{"kind":"run_end","run_id":"once-...","status":"completed"}
```

## 規格與歷史

- 現行技術契約：[`CLAUDE.md`](CLAUDE.md)
- 歷史變更與已完成里程碑：[`docs/CHANGELOG.md`](docs/CHANGELOG.md)
- 尚未完成的工作：[`README.todo`](README.todo)
- 已實作規格：
  - [`2026-07-29-Summary.md`](docs/specs/2026-07-29-Summary.md)（M1–M5 歷史摘要）
  - [`2026-07-18-continuous-logdoctor-minimax.md`](docs/specs/2026-07-18-continuous-logdoctor-minimax.md)
  - [`2026-07-27-agent-sdk-contract-alignment.md`](docs/specs/2026-07-27-agent-sdk-contract-alignment.md)
  - [`2026-07-27-provider-auth-image-capabilities.md`](docs/specs/2026-07-27-provider-auth-image-capabilities.md)
