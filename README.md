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

圖片、影片、音樂、語音 (TTS) 與轉錄 (STT) 分別是 `provider.ImageGenerator`、
`provider.VideoGenerator`、`provider.MusicGenerator`、`provider.SpeechGenerator`、
`provider.Transcriber` optional capability，不進 agent runtime 的 `core.Provider`。
caller 必須明確走 `NewImage`、`NewVideo`、`NewMusic`、`NewSpeech` 或
`NewTranscriber`；不支援的 adapter 回傳可用
`errors.Is(err, provider.ErrUnsupportedCapability)` 判斷的錯誤：

### Provider Capabilities

| Provider     | Chat | Vision read | Image | Video | Music | Speech (TTS) | STT | Voice list | Live catalog |
| ------------ | ---- | ----------- | ----- | ----- | ----- | ------------ | --- | ---------- | ------------ |
| `anthropic`  | V    | V           | -     | -     | -     | -            | -   | -          | V            |
| `antigravity`| V    | V           | -     | -     | -     | -            | -   | -          | -            |
| `codex`      | V    | V           | -     | -     | -     | -            | -   | -          | -            |
| `elevenlabs` | -    | -           | -     | -     | -     | V (+stream)  | V   | V          | V            |
| `google`     | V    | V           | V     | -     | -     | -            | -   | -          | V            |
| `grok`       | V    | V           | V     | -     | -     | -            | -   | -          | V            |
| `minimax`    | V    | V           | V     | V     | V     | V            | -   | V          | V            |
| `ollama`     | V    | V           | -     | -     | -     | -            | -   | -          | V            |

- `Vision read`：chat request 可攜帶 image part（各 adapter 編成自家 wire 形狀）
- `Image`：MiniMax 同時支援 t2i 與 i2i（`ImageRequest.SubjectReferences`）
- `Voice list`：`provider.VoiceLister`，從 `NewSpeech` 回傳值以 type assertion 取得
- `Live catalog`：`core.ModelLister`；沒有的 provider 讀 static `Entry.Catalog`

### Capability Interfaces

每個 capability 一個小 interface；optional capability 不併進 `core.Provider`，
不支援的 provider 在 factory 回 typed `ErrUnsupportedCapability`：

| Capability       | Interface                  | Method                                                      | 取得方式                        |
| ---------------- | -------------------------- | ----------------------------------------------------------- | ------------------------------- |
| Chat (blocking)  | `core.Provider`            | `Generate(ctx, ModelRequest) (ModelResult, error)`          | `provider.New`                  |
| Chat (streaming) | `core.StreamProvider`      | `Stream(ctx, ModelRequest) (<-chan ModelChunk, error)`      | 同上（`Adapter` = 兩者合體）    |
| Live catalog     | `core.ModelLister`         | `ListModels(ctx) ([]ModelSpec, error)`                      | type assertion                  |
| Image (t2i/i2i)  | `provider.ImageGenerator`  | `GenerateImage(ctx, ImageRequest) (ImageResult, error)`     | `provider.NewImage`             |
| Video            | `provider.VideoGenerator`  | `MaxPromptLength() int`；`GenerateVideo(ctx, VideoRequest) (VideoResult, error)` | `provider.NewVideo` |
| Music            | `provider.MusicGenerator`  | `GenerateMusic(ctx, MusicRequest) (MusicResult, error)`     | `provider.NewMusic`             |
| Speech (TTS)     | `provider.SpeechGenerator` | `GenerateSpeech(ctx, SpeechRequest) (SpeechResult, error)`  | `provider.NewSpeech`            |
| Speech streaming | `provider.SpeechStreamer`  | `StreamSpeech(ctx, SpeechRequest) (io.ReadCloser, error)`   | `NewSpeech` 值 type assertion   |
| Voice list       | `provider.VoiceLister`     | `ListVoices(ctx, VoiceListRequest) (VoiceListResult, error)` | `NewSpeech` 值 type assertion  |
| Transcribe (STT) | `provider.Transcriber`     | `Transcribe(ctx, TranscribeRequest) (TranscribeResult, error)` | `provider.NewTranscriber`    |

streaming 與 voice list 這類「同一 client 的附加能力」以 type assertion 發現，
credential decorator 包裝後仍保留。

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

MiniMax 的 video adapter 支援 text / image / startend / subject 四種 mode，負責
asynchronous polling、authenticated download 與 MP4 verification；caller 提供
`VideoRequest.OutputPath`，完成後取得 `VideoResult.Path`。

MiniMax 的 music adapter 提供 non-streaming text-to-music 與 cover generation。
以下是 user-supplied Python request 的 Go 等價寫法：

```go
generator, err := provider.NewMusic("minimax", provider.Options{})
if err != nil {
	return err
}
result, err := generator.GenerateMusic(ctx, provider.MusicRequest{
	Model:        "music-cover",
	AudioURL:     "https://example.com/original-song.mp3",
	Prompt:       "Jazz, smooth, late night lounge, saxophone",
	OutputFormat: "url",
	AudioSetting: provider.MusicAudioSetting{
		SampleRate: 44100,
		Bitrate:    256000,
		Format:     "mp3",
	},
})
```

`result.Audio.URL` 是短效連結；需要 durable asset 時由 caller 及時下載保存。

可執行的 [`provider/sample`](provider/sample/README.md) 直接展示 provider、auth mode 與
`chat / image / music / speech / transcribe` API type matrix。speech synthesis、
transcription 與 audio-chat 是三個不同 contract，分別走 `SpeechGenerator`、
`Transcriber` 與（尚未有 adapter 支援的）audio-chat。

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
  - [`2026-08-04-Summary.md`](docs/specs/2026-08-04-Summary.md)（2026-07-21 之前的歷史摘要）
  - [`2026-07-27-agent-sdk-contract-alignment.md`](docs/specs/2026-07-27-agent-sdk-contract-alignment.md)
  - [`2026-07-27-provider-auth-image-capabilities.md`](docs/specs/2026-07-27-provider-auth-image-capabilities.md)

## 已淘汰功能 (Deprecated Features)

| 淘汰日期 | 功能 | 原始文件 | 說明 |
| -------- | ---- | -------- | ---- |
| 2026-08-04 | Logdoctor proposal / approval lifecycle | `2026-07-18-continuous-logdoctor-minimax.md` | immutable proposal + digest 與 `list` / `show` / `approve` / `reject` 子指令已於 `879e246` 隨 log-agent-v2 重構移除；`sample/logdoctor-agent` 現只保留 `analyze` 與 `watch` |
| 2026-08-04 | `provider/openaicompat` | `2026-07-18-continuous-logdoctor-minimax.md` | 已於 `551410d` 移除；OpenAI-compatible wire 改由 `provider/protocol/openaichat` 共用 codec 承接 |
