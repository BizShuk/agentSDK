# 變更紀錄 (Changelog)

本檔承接原先散落於 `CLAUDE.md` 的演進紀錄，以及 `README.todo` 的已完成與
`Archive` 項目。現行技術契約以 [`CLAUDE.md`](../CLAUDE.md) 為準；尚未完成的工作以
[`README.todo`](../README.todo) 為準。

日期優先沿用來源文字；來源未標日期時，採該項目首次出現於 Git history 的日期。
本檔記錄歷史事實，識別符可能已在後續重構中改名或移除。

## 2026-08-06

### 正典文件範疇清理 (Canonical docs scope cleanup)

- provider adapter 的 wire 細節（antigravity Cloud Code envelope、MiniMax 五個
  surface、ElevenLabs audio-only、Google Live / Codex Realtime handshake、vision
  編碼形狀）由 `CLAUDE.md` 移入 `docs/providers.md`；`CLAUDE.md` 只留 capability
  boundary 規則。
- `provider` / `wizard` / sample / benchmark 的指令目錄由 `CLAUDE.md` 與
  `README.md` 移入 `docs/cli.md`，正典文件只留 quick start 與指標。
- `README.md` 的靜態 provider capability 矩陣刪除，改指向
  `go run . provider --list`：該表已與實際不符（antigravity image 支援未反映，
  live / translate 兩欄從未存在）。
- `技術棧` 表的版本 pin 移除（真相是 `go.mod`）；外部 `bizshuk/proxy` 的
  dependency 列移除。
- 修正陳述：root binary 掛的是 `cmd.ProviderCmd` / `wizard.WizardCmd`
  package-level var，不是 `NewProviderCommand()` / `NewWizardCommand()`；
  `tmp/` 下不存在 `auth2api` / `cliproxyapi`。

### 歷史事實（自 `CLAUDE.md` 搬入）

- MiniMax 曾宣告的 `MINIMAX_OAUTH_TOKEN` 因無對應 OAuth flow、也無 auth route
  而移除；credential kind 是 entry 內的軸，不另立 provider entry。
- `core/` 的檔名曾使用 `input`、`effect`、`step`、`thinking`、`port` 等需先讀
  內容才能理解的名稱，後改為直接對應公開詞彙。
- `provider/sample` 已整併進 `cmd/provider/`。

## 2026-08-03

### Audio capability（STT/TTS）+ ElevenLabs / MiniMax speech adapters

- `provider` 新增 `Transcriber`（STT）與 `SpeechGenerator`（TTS；optional
  `SpeechStreamer` 回 `io.ReadCloser`）contract、`CAPABILITY_AUDIO_TRANSCRIBE` /
  `CAPABILITY_AUDIO_SPEECH`、`Entry.NewTranscriber` / `Entry.NewSpeech` 與
  `provider.NewTranscriber` / `provider.NewSpeech` 建構路徑；`Register` 的
  「至少一 factory」檢查放寬納入 audio factories。
- 新 adapter `provider/elevenlabs`（首個 audio-only、`New == nil` 的 provider）：
  `xi-api-key` header、`ELEVENLABS_API_KEY` / `ELEVENLABS_BASE_URL`、multipart
  STT（`scribe_v1`）、TTS + `/stream`（`eleven_flash_v2_5`）。
- `provider/minimax` 增 `NewSpeech`：`t2a_v2`、hex 解碼為 canonical bytes、
  `extra_info` → `SpeechInfo`、預設 `speech-02-hd`；speech models 入 catalog；
  `Metadata.SpeechBaseURLEnv` + `MINIMAX_SPEECH_BASE_URL`（比照 music/video，
  speech 解析不再消費 `MINIMAX_BASE_URL`），resolved base 尾段 `/anthropic`
  一律 trim（不分來源，文件化於 `speechBaseURL`）。
- `provider/sample`：audio 欄由 generic unsupported stub 改為真實
  `NewSpeech` / `NewTranscriber` dispatch。
- 發起方：`customer_service` 語音客服（STT → agent → TTS 級聯）；
  live smoke（打真 API）尚未執行。

## 2026-07-31

### MiniMax music optional capability

- `provider` 新增 non-streaming `MusicGenerator`、`MusicRequest`、`MusicResult`、
  `Entry.NewMusic`、`provider.NewMusic` 與 `CAPABILITY_MUSIC_GENERATE`，不擴肥 agent
  runtime 的 `core.Provider`，也不把 music 假裝成 generic audio。
- `provider/minimax` 接上 `POST /v1/music_generation`，支援 text-to-music 與 one-step /
  feature-ID cover inputs、request-time Bearer auth、`music-3.0` / `music-cover` defaults、
  bounded response/error、typed `APIError` 與獨立 `MINIMAX_MUSIC_BASE_URL`。
- `provider/sample --type music` 成為真實 consumer，提供 cover URL、lyrics、output/audio
  settings 與 safe terminal output；`--type audio` 繼續回 typed unsupported。
- deterministic acceptance 使用 `httptest`；尚未以真實 `MINIMAX_API_KEY` 與
  operator-owned source audio 執行 paid live smoke。

### MiniMax video optional capability

- `provider` 新增 `VideoGenerator`、`VideoRequest`、`VideoResult`、`Entry.NewVideo`、
  `provider.NewVideo` 與 `CAPABILITY_VIDEO_GENERATE`，不擴肥 agent runtime 的
  `core.Provider`。
- `provider/minimax` 接手 text / image / startend / subject 四種 mode，以及
  asynchronous polling、authenticated download、MP4 `ftyp` verification 與獨立的
  `MINIMAX_VIDEO_BASE_URL` endpoint override。
- `ip-incubation` 改為 AgentSDK consumer，移除原本重複的 `svc/video` ownership；
  deterministic acceptance 使用 `httptest`，尚未執行 live MiniMax smoke test。

## 2026-07-29

### Provider 串流協定

- Anthropic、Antigravity、Codex、Grok、MiniMax 的 `stream.go` 全數由逐行
  `bufio.Scanner` 遷移到 `provider/protocol/sse.Decoder`。
- 各 adapter 保留自己的 payload 與 terminal semantics；共用 contract tests 覆蓋
  terminal、transport error、cancellation、multiline data、partial frame、size limit
  與 Grok UTF-8 BOM。

### 排程式 log agent

- `sample/log-agent-v2` 落地為持續執行、逐 batch 建立 `TIER_ONESHOT` agent 的 listener。
- 每批 transcript、budget 與 `RunID` 隔離；只有 model run 與輸出都成功才 atomic commit
  cursor，失敗批次於下一個 interval 重試。

### 遷移時里程碑快照

- `M1` 核心範式與 sample、`M2` state/WAL/checkpoint/retry/timeout/loopguard、
  `M3` tool schema/security/tracing、`M4` HITL/provider、`M5` built-in tools/sample
  wiring、`M6` auth mechanism/CLI 均已完成。
- Proxy `3×3` pairwise cutover、安全 hardening 與四來源 `37` entity wire-format catalog
  已完成。
- `auth`、`proxy` 已成為外部 module，`config/` 解體，`perception/` 移除。
- Agent skeleton `M1`–`M7` 與 Harness/UX skeleton 已落地；細節缺口留在
  `README.todo`。

## 2026-07-28

### Lossless reasoning content

- `core.Part` 新增 `PART_KIND_REASONING`；可攜 reasoning 文字留在 `Part.Text`，
  provider continuation metadata 留在 `ReasoningState`。
- `ModelResult.Parts` 成為有序 canonical assistant content，`Text` / `ToolCalls` 保留為
  compatibility projections；runtime 在寫入 transcript 前先執行 `NormalizeContent`。
- Anthropic、Antigravity、MiniMax、Codex 與 Grok 的 reasoning non-stream／stream path
  已接線；無法表示 continuation metadata 的 wire path 明確報錯，不靜默丟棄。

## 2026-07-27

### Provider contract 與 config pipeline

參考：[`docs/specs/2026-07-27-provider-auth-image-capabilities.md`](specs/2026-07-27-provider-auth-image-capabilities.md)、
[`docs/specs/2026-07-27-agent-sdk-contract-alignment.md`](specs/2026-07-27-agent-sdk-contract-alignment.md)。

- Credential kind 只決定 request auth，不決定 endpoint 或 body；endpoint 統一由
  `ResolvedConfig.BaseURL` 管理，`core.Auth` 只保留 `APIKey`、`Bearer`、`Headers`。
- `Phase C2` 將 decorator 接到外部 `auth`，移除零 caller 的 OAuth token DTO、
  `NewWithOAuth` 與 rebuild-provider wrapper。
- `Phase 5` 收斂成單一 `Options.Resolve` → `ResolvedConfig` → `Entry.New` pipeline，
  移除七份 private config、functional options、env resolver 與 register converter。
- `Phase 6` 以跨 adapter golden/httptest 證明 Google／Ollama wire contract 相容後，
  共用 `provider/protocol/openaichat`；generic SSE framing 下沉至
  `provider/protocol/sse`。
- `Phase 7` 接上 model/image 共用 auth policy 與 optional image capability：
  `Entry.NewImage`、`provider.NewImage`、`Entry.Supports`，以及 Google／Grok 共用的
  `provider/protocol/openaiimage`。
- `provider.ImageGenerator`、`ImageRequest`、`ImageResult` 留在 provider layer；
  unsupported adapter 回傳 typed `ErrUnsupportedCapability`，不擴肥 `core.Provider`。
- `provider.Entry` 成為 provider name、metadata、static catalog、model/image factory 的
  唯一 owner；constructed adapter 不再複製 discovery data。
- `provider.Metadata` 將 OAuth 與 API-key env 分開；`Options.CredentialKind` 支援
  `auto`、`api_key`、`oauth`，`Resolve` 回傳 canonical `ResolvedConfig`。
- `tool/builtin.Register`、`reasoning.NewRule`、`core.ParseAutonomyLevel` 分別接管
  tool、reasoning style、autonomy 的 name-to-implementation mapping。
- `core.Provider` 收窄為 blocking `Generate`；stream、live catalog、image generation
  分別使用 optional capability。
- `provider/sample` 落地為 root module 內的 package-local executable，支援
  provider × chat/image/audio × auth-env matrix；audio 在尚無 production contract 時回
  typed unsupported error。

### Agent lifecycle 與 public surface

- `agent/build.go` 收斂為七階段 composition；`Preflight` 併回單次 `Bootstrap`。
- `Bootstrap` 成為 engine 與 opening state 的唯一組裝 owner；`agent.Run` 回傳 `error`，
  process exit code 只由 `agent/cli` 轉譯。
- `Parts` 只保留 driver 真正消費的 `Engine`、`Sessions`、`Skills`、`Host`、`Cwd`。
- 移除無 production consumer 的 `spec.Output`、tier output default、wizard output stage
  與 `buildSink`；presentation 由 `WithSink` 或 `Engine.Sink` 接管。
- `provider/credential.Source.Decorator()` 以 `auth.Resolver` 在每次 request
  解析／換發 credential，不重建 inner provider。

## 2026-07-26

### Package 與 ownership 收斂

- `core/` 依 run state、transition、model boundary、tool/HITL、runtime ports 重分檔，
  仍維持單一 package；`DecisionRule` / `NewDecide` 移交 `reasoning/`。
- `agent/` 收斂為七個 root production files；`agent.go` 集中公開契約，
  `options.go` 集中注入，engine/runtime seams 歸入 `build.go` 與 `host.go`。
- `agent/spec` built-in vocabulary 改為純宣告值並以 drift test 對齊 owner API，
  恢復只依賴 `core` 的邊界。
- `permission`、`session`、`wire` 收入 `agent/` 子套件，仍各自只 import `core`。
- `sample/code-agent/tui` 成為 zero-dependency application renderer；SDK surface 不再暗示
  terminal UI 是必要依賴。
- `provider` 目錄的 package 名由 `registry` 收斂為 `provider`；
  default provider ownership 移到 `provider.DEFAULT_NAME`。
- `PersonaSource`、`ContextFileSource`、`EnvSource`、`ReminderSource` 與
  `SkillSource` 收入 `prompt/source`，透過 `SkillProvider` 避免反向依賴 `skill` 或
  `agent`。
- `sample/` 以 `demo-*` 表示單一元件展示，以 `*-agent` 表示完整 agent；
  `provider/sample` 維持 package-local example。

### Provider 認證解耦

計畫：[`plans/2026-07-26-auth-under-provider-and-config-dissolution.md`](../plans/2026-07-26-auth-under-provider-and-config-dissolution.md)。

- `Phase A` 移除 agent 對 auth 的死連線：`RefreshingProvider` 歸入
  `provider/credential`，`Host` 移除零 caller 的 `AuthStore` / `AuthDir`。
- `Phase B` 解體 `config/`：process host 歸 `agent/cli`，middleware presets 歸
  `middleware/preset`。
- `Phase C1` 將七個 adapter 統一到 `core.Auth`，接上 per-request
  `provider.Decorator`；adapter 本身不 import `auth`。
- `core.Auth` 的 canonical union 與七個 adapter 的消費路徑完成驗證。

### Samples 與互動修正

- `sample/logdoctor-agent` 接入 `tool.RegisterDefaults`，共八個工具；secure middleware
  policy 由 nil 改為明確 policy。
- `reasoning.decodeCalls` 修正 JSON round-trip 後 pending calls 消失的問題；
  batch settlement 會補齊 skipped/unrun tool results。
- `agent.Interactive` 的 advance path 只走 `SubmitHumanDecision`，避免 double-drive。
- `sample/skeleton-agent` 以單一背景 stdin reader 實作 `Interactive` REPL。

## 2026-07-24

### Round、batch 與 interactive seam

計畫：[`plans/2026-07-24-round-batch-and-interactive-seam.md`](../plans/2026-07-24-round-batch-and-interactive-seam.md)。

- `Task 0`：`core.Budget` 新增 `MaxRounds`、`UsedRounds`、`MaxToolCalls`，
  `Exceeded()` 新增 `round_budget`；同步 `spec.Limits`、tier defaults、validation 與
  `build.go` mapping。
- `Task 1`：`ThinkThenAct` 一次 dispatch 同一 assistant message 的整批 `CALL_TOOL`；
  runtime 以 `settleSkipped` / `settleUnrun` 保證每個 `tool_use` 都有 `tool_result`。
- `Task 2`：`MaxToolCalls` 超限時整批 skip + settle，暫停於 continue-gate；
  approve 後重讀重新規劃，reject 後完成。
- `Task 3`：三個互動草案收斂為 `agent.Interactive.NextRound` 與
  `WithRoundTimeout`。
- `Task 4`：`sample/skeleton-agent` 的 stdin REPL 通過 `rounds:2` live 驗證。
- `Task 5`：同步 `docs/terminology.md`、`CLAUDE.md` 與 `README.todo`。
- `contextfile` 併入 `prompt.LoadContextFiles`，保留固定階層搜尋與 `@import` 行為。
- Codex 與 Antigravity 進入 provider registry。
- in-tree dependency analyzer prototype `19f0d41` 移除；工具維持為外部
  `github.com/bizshuk/go-dependency-analysis`。

## 2026-07-22

### Agent skeleton `M1`–`M7`

計畫：[`plans/2026-08-06-Refresh.md`](../plans/2026-08-06-Refresh.md)（原
`2026-07-22-agent-skeleton-config-opt-in.md`，已整併）。

- `M1`：`agent/spec` 提供 `Config`、八個 feature blocks、`Choice` metadata、
  tier expansion、validation 與 JSON encoding。
- `M2`：`agent.Once` / `OnceStream` 以 one-shot reasoning 走同一條 Engine path。
- `M3`：`prompt` 提供 `Slot`、`Section`、`Source`、`Builder` 與五類 prompt source。
- `M4`：`agent/build.go` / `agent/options.go` 建立七階段 pipeline、`New`、`MustNew`、
  `Bootstrap` 與注入 options。
- `M5`：provider registry 成為 name → adapter 的共用真相，env lookup 可注入。
- `M6`：`sample/code-agent/cmd/compose.go` 由 `333` 行降至 `101` 行；policy 與 session
  selection 分離。
- `M7`：`wizard` / `w` 逐階段產生設定，choices 來自 `spec` 與 provider catalog。
- `agent.subagentRunner` 接管 scoped ephemeral engine factory，套用
  `SubAgent.Tools` allowlist。
- `CLAUDE.md` / `README.md` 同步移除幽靈 package tree 與失效驗證路徑。

## 2026-07-21

- `tui/` 曾併回 root module，之後依唯一 caller ownership 於 2026-07-26 下沉至
  `sample/code-agent/tui`；全程未建立獨立 `go.mod`。

## 2026-07-20

### Root provider CLI

- `cmd/provider.go` 新增 `NewProviderCommand()`，支援 provider/model/auth/base URL、
  system/max tokens、stream/JSON 與 provider/model listing flags。
- CLI 直接呼叫 `core.Provider.Generate` / `core.StreamProvider.Stream`，不經
  Engine 或 harness。
- `main.go` 掛載 root `provider` subcommand。
- project tree、module mapping 與 provider smoke-test 文件同步更新。
- 共用 registry 連接 MiniMax 與 Anthropic，後續擴充 Google、Grok、Ollama。
- `cmd/provider.go` 與 agent composition 共用 registry，避免 provider choices 漂移。

## 2026-07-19

### Harness/UX skeleton

計畫歷史：[`plans/2026-08-06-Refresh.md`](../plans/2026-08-06-Refresh.md)（原
`2026-08-03-refresh.md`，已整併）。
來源調查：[`docs/memory/2026-07-19-agent-client-feature-catalog.md`](memory/2026-07-19-agent-client-feature-catalog.md)。

- `sample/code-agent` 接上 TUI、wire、hooks、permission、session、skills、commands、
  subagents、steering/follow-up queue；支援互動、`-p`、`--json`、session flags 與
  `--fake` scripted provider。
- `provider/*` module 併回 root module（`551410d`）。
- 移除無 production caller 的 root `cli/` JSONL codec 與 `mcp/` client；若重新引入，
  必須以真實 consumer 與 wire contract 為前提。
- 移除外部 `video-utils` dependency、root command wiring、相關測試與
  ffmpeg/ffprobe 前置需求。

## 2026-07-18

- `auth`、`proxy` 拆為外部 module；root module 不再包含其 runtime tree。
- 刪除無 production consumer 的 `perception/`，保留 `core.ObservationSource` port。
- `DefaultEnvironmentNames` 新增 Ollama 與 LLMBOX environment names。

## 2026-07-17

- Proxy 三種 client format 全數切換為 directed pair transforms，完成 `3×3` handler
  wiring 與安全 hardening。
- 完成四來源、`37` entities 的 wire-format catalog。

## 2026-07-15

### Provider 認證 `M6`

- 完成 stdlib-only auth mechanism：`Credential`、`Authenticator`、`VerifyResult`、
  `Options`。
- 完成 OAuth PKCE/client-secret、state CSRF、single-flight refresh 與
  `429 Retry-After` backoff。
- 完成 RFC 8628 device authorization 與 HTTPS-only OIDC discovery。
- 完成 `APIKeySpec` 與泛用 `APIKeyAuth` env/model verification。
- 完成權限固定為 `0700` / `0600` 且以 temp + rename 寫入的 `FileStore`。
- 完成 Anthropic、OpenAI、Google、xAI、Antigravity、Vertex 六個 provider packages。
- 完成 `ROUTES` registry 與 `New` / `Login` / `For` / `IDs`，涵蓋九個 provider IDs。
- 完成供 auth 與 provider packages 共用的 `authtest` fake provider。
- 完成 `login --provider`、list、verify、refresh、logout CLI。
- fake provider e2e 跑通 login → list → verify → refresh → logout。
- xAI device flow 對真實 `auth.x.ai` 跑通 discovery 與 device-code issuance。
- 到期自動 refresh 後續由 `provider/credential.Source.Decorator()` 接管。

## 2026-07-13

- `sample/logdoctor-agent` 接入六個 built-in tools、`read_log_tail`、`notify` 與
  secure middleware policy。
- 建立 `sample/file-agent`，以六個 built-in file tools 示範完整 agent。

## 2026-07-11

以下項目由 `README.todo` 的原 `Archive` 區遷入；保留原完成時間。

### `M5` built-in tools 與 sample wiring

- `17:34:51`：`sample/greet-agent` 接入六個 built-in tools 與 `SecureMiddleware`。
- `17:34:49`：完成 `config.MustOpenForCLI` / `OpenForCLI` / `Host` wiring helper。
- `17:34:46`：`action.Policy` 在 tool 內執行 sandbox defense-in-depth re-check。
- `17:34:44`：`tool/tool.go` 提供一次註冊的 `RegisterDefaults(reg, opts)`。
- `17:34:42`：完成 `tool/{read,write,edit,bash,glob,grep}.go` 六個 built-in tools。

### `M4` 架構解耦、HITL 與三 Provider

- `17:34:39`：sample 接通 `--provider=anthropic|openaicompat|google`。
- `17:34:37`：完成三 provider mock-driven tests。
- `17:34:34`：`runtime/m4_hitl_e2e_test.go` 覆蓋 approve 與 reject。
- `17:34:31`：`runtime.consumeApprovedPendingCall` 消費 out-of-band decision。
- `17:34:29`：`config.SecureMiddleware` 接上 sandbox、approval、spotlight、sanitizer。
- `17:34:27`：建立 `provider/{anthropic,openaicompat,google}` 三個獨立 modules。
- `17:34:25`：`cli/envelope.go` / `cli/codec.go` 定義九種 JSONL message types。
- `17:34:22`：`action/approval_policy.go` 完成 `DefaultApprovalPolicy` L0–L4 grid。

### `M3` 工具生態與 runtime security

- `17:34:19`：`runtime/m3_e2e_test.go` 驗證 sanitizer、spotlight 與 transcript。
- `17:34:17`：`sample/logdoctor-agent` 建立 todo tools、domain core 與 list command。
- `17:34:15`：`mcp/client.go` 以 `modelcontextprotocol/go-sdk` 實作
  `action.ToolSource`。
- `17:34:13`：`middleware/observability/tracing.go` 接入 OTel spans。
- `17:34:11`：`middleware/security/spotlight.go` 標記 untrusted tool output。
- `17:34:08`：`approval_gate.go` 以 L0–L4 × risk grid 將 `ASK` 改寫為
  `REQUEST_APPROVAL`。
- `17:34:06`：`sanitizer.go` 將 prompt injection 改寫為
  `[SANITIZED_BY_AGENTSDK]` banner。
- `17:34:04`：`action/sandbox.go` / `sandbox_mw.go` 實作 path/command allow-deny。
- `17:34:02`：`action/schema.go` 以 `invopop/jsonschema` 反射 schema 並驗證 required
  fields。
- `17:32:31`：建立 `sample/file-agent`。

## 來源 (Sources)

- `CLAUDE.md`：原技術基準、核心決策、module mapping 與完成狀態中的歷史段落。
- `README.todo`：原有 `75` 筆 completed items 與 `Archive` 項目。
- Git history：只用於補足來源未標日期的完成紀錄。
