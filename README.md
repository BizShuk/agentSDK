# agentsdk

Go Agentic Loop SDK：以`宣告式設定`組裝目標導向控制迴圈 (Goal-directed Control Loop)。應用層宣告`要開哪些能力`，SDK 負責`怎麼接`。

## 範疇 (Scope)

五大支柱對應到頂層 package,架構即文件:

| 支柱 | 套件 | 角色 |
| ----------- | -------------------------- | -------------------------------------------------------------------------------------------- |
| 1. 認知架構 | `core` (ObservationSource) | 觀察來源 port (Observations channel) |
| 2. 系統韌性 | `memory/` | Window / Compactor / Checkpoint |
| 3. 工具生態 | `tool/` | `core.Tool` / RawMessage converter / Registry / allowlist-aware 內建工具 factory / Sandbox |
| 4. 推理 | `reasoning/` | `NewRule` + 6 種 DecisionRule (ReAct / Planner-Executor / Executor-Critic / CoT / Reflexion / Router) |
| 5. 組裝 | `agent/` + `agent/spec/` | 宣告式 `Config` → 7 stage pipeline → `*agent.Engine`；`prompt/` 管進 context window 的內容 |

`core/` 是純狀態機 (state + event + instruction + reasoning),只依賴 stdlib,連 gosdk 都不 import。root module 的 `runtime/loop.go` 是 shell,負責 dispatch instructions 到綁定的 port (model / tools / store / notifier)。

完整目錄樹、每個 package 的 ownership 與架構不變式由 [`CLAUDE.md`](CLAUDE.md)
單一擁有，本檔不維護第二份。

### 多模態能力 (Multimodal Capabilities)

圖片、影片、音樂、語音 (TTS)、轉錄 (STT)、realtime live session 與翻譯都是
`provider` layer 的 optional capability，`不`進 agent runtime 的 `core.Provider`——
runtime 只需要 blocking `Generate`。caller 明確走 `provider.NewImage` / `NewVideo` /
`NewMusic` / `NewSpeech` / `NewTranscriber` / `NewLive` / `NewTranslate`；不支援的
adapter 回傳可用 `errors.Is(err, provider.ErrUnsupportedCapability)` 判斷的錯誤。

哪個 provider 支援哪些 surface 由 registry 產生，不在文件裡維護靜態副本：

```bash
go run . provider --list      # provider × capability × auth env
```

每個 capability 的 interface 與方法簽名、各 adapter 的 endpoint 與 wire 細節見
[`docs/providers.md`](docs/providers.md)；指令用法見 [`docs/cli.md`](docs/cli.md)。

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

## 執行範例

| Sample | 展示什麼 |
| ------ | -------- |
| `sample/code-agent` | 全能力組合：TUI / print / stream-json / session 續跑，以宣告 `agent.Config` 取代手工接線 |
| `sample/log-agent-v2` | scheduler 建立 batch 後透過 `agent.WithListener` 進入完整 agent lifecycle |
| `sample/logdoctor-agent` | 比較用的精簡 `agent.OnceStream` 路徑 |
| `sample/skeleton-agent` | `wizard --print-go` 輸出的單檔對應範本 |
| `sample/demo-*` | memory / middleware / reasoning strategy 的單一元件展示 |

```bash
cd sample/code-agent
go run . --fake -p "看看這個專案"   # 不打 provider 的 print 模式
```

各 sample 的完整 flag 與環境變數見 [`docs/cli.md`](docs/cli.md)。

## 規格與歷史

- 現行技術契約：[`CLAUDE.md`](CLAUDE.md)
- Provider adapter 能力與 wire 細節：[`docs/providers.md`](docs/providers.md)
- 指令參考：[`docs/cli.md`](docs/cli.md)
- 領域術語：[`docs/terminology.md`](docs/terminology.md)
- 歷史變更與已完成里程碑：[`docs/CHANGELOG.md`](docs/CHANGELOG.md)
- 尚未完成的工作：[`README.todo`](README.todo)
- 已實作規格：
  - [`2026-08-04-Summary.md`](docs/specs/2026-08-04-Summary.md)（2026-07-21 之前的歷史摘要）
  - [`2026-07-27-agent-sdk-contract-alignment.md`](docs/specs/2026-07-27-agent-sdk-contract-alignment.md)
  - [`2026-07-27-provider-auth-image-capabilities.md`](docs/specs/2026-07-27-provider-auth-image-capabilities.md)

## 已淘汰功能 (Deprecated Features)

| 淘汰日期 | 功能 | 原始文件 | 說明 |
| -------- | ---- | -------- | ---- |
| 2026-08-04 | Logdoctor proposal / approval lifecycle | `2026-07-18-continuous-logdoctor-minimax.md` | immutable proposal + digest 與 `list` / `show` / `approve` / `reject` 子指令已於 `879e246` 隨 log-agent-v2 重構移除；`sample/logdoctor-agent` 現只保留單一 `watch` 子指令 |
| 2026-08-04 | `provider/openaicompat` | `2026-07-18-continuous-logdoctor-minimax.md` | 已於 `551410d` 移除；OpenAI-compatible wire 改由 `provider/protocol/openaichat` 共用 codec 承接 |
