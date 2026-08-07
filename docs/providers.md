# Provider Adapters — capability surface 與 wire 細節

本檔擁有`每個 adapter 怎麼接 upstream`的細節：endpoint、payload 形狀、預設 model、
env override 與各家 wire 的例外處置。`哪一層擁有什麼`（capability boundary、
config pipeline、codec 共用原則）由 [`CLAUDE.md`](../CLAUDE.md) 擁有，不在此重複。

adapter 的 upstream 會改版，本檔因此是`高頻異動`區；CLAUDE.md 的 boundary 規則不會
因為某家 endpoint 改名而失效，兩者刻意分開。

## Capability 矩陣 (Capability Matrix)

矩陣由 registry 產生，`不`在文件裡維護靜態副本 —— 靜態表格會在新增 adapter 時靜默過期：

```bash
go run . provider --list      # provider × chat/catalog/image/video/music/transcribe/speech/live/translate × auth env
```

`Capability` 表示 operation；model 是否能讀 image/audio 或產生 text/image/audio/video，
另由 catalog 的 directional modalities 表示，不另塞進 provider-level matrix。

## Capability Interfaces

每個 capability 一個小 interface；optional capability 不併進 `core.Provider`，
不支援的 provider 在 factory 回 typed `provider.ErrUnsupportedCapability`
（以 `errors.Is` 判斷）：

| Capability       | Interface                    | Method                                                                           | 取得方式                         |
| ---------------- | ---------------------------- | -------------------------------------------------------------------------------- | -------------------------------- |
| Chat (blocking)  | `core.Provider`              | `Generate(ctx, ModelRequest) (ModelResult, error)`                               | `provider.New`                   |
| Chat (streaming) | `core.StreamProvider`        | `Stream(ctx, ModelRequest) (<-chan ModelChunk, error)`                           | 同上（`Adapter` = 兩者合體）     |
| Live catalog     | `provider.ModelLister`       | `ListModels(ctx) ([]ModelSpec, error)`                                           | type assertion                   |
| Image (t2i/i2i)  | `provider.ImageGenerator`    | `GenerateImage(ctx, ImageRequest) (ImageResult, error)`                          | `provider.NewImage`              |
| Video            | `provider.VideoGenerator`    | `MaxPromptLength() int`；`GenerateVideo(ctx, VideoRequest) (VideoResult, error)` | `provider.NewVideo`              |
| Music            | `provider.MusicGenerator`    | `GenerateMusic(ctx, MusicRequest) (MusicResult, error)`                          | `provider.NewMusic`              |
| Speech (TTS)     | `provider.SpeechGenerator`   | `GenerateSpeech(ctx, SpeechRequest) (SpeechResult, error)`                       | `provider.NewSpeech`             |
| Speech streaming | `provider.SpeechStreamer`    | `StreamSpeech(ctx, SpeechRequest) (io.ReadCloser, error)`                        | `NewSpeech` 值 type assertion    |
| Voice list       | `provider.VoiceLister`       | `ListVoices(ctx, VoiceListRequest) (VoiceListResult, error)`                     | `NewSpeech` 值 type assertion    |
| Transcribe (STT) | `provider.Transcriber`       | `Transcribe(ctx, TranscribeRequest) (TranscribeResult, error)`                   | `provider.NewTranscriber`        |
| Live session     | `provider.LiveConnector`     | `ConnectLive(ctx, LiveRequest) (LiveSession, error)`                             | `provider.NewLive`               |
| Translate        | `provider.Translator`        | `Translate(ctx, TranslateRequest) (TranslateResult, error)`                      | `provider.NewTranslate`          |
| Translate stream | `provider.TranslateStreamer` | `StreamTranslation(ctx, TranslateRequest) (<-chan TranslateChunk, error)`        | `NewTranslate` 值 type assertion |

streaming、voice list、translate stream 這類「同一 client 的附加能力」以 type
assertion 發現，credential decorator 包裝後仍保留。

binary 需 blank-import 目標 adapter（或 `provider/all`）讓它註冊。URL result 可能是
短效連結；要持久化時由 caller 複製資產。

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

## Usage and Cost Accounting

Chat、image、video、music、speech、transcribe、live 與 translate result 會攜帶
canonical `Usage` / `Cost` metadata。Chat token usage 分成 input、output、cache read 與
web search；其他 capability 使用各自可觀察的計費維度。Streaming 只在 terminal chunk
回報一次 usage/cost，避免 consumer 重複加總。

`Cost.Status` 明確區分：

- `exact`：provider 回傳 authoritative billed cost。
- `estimated`：以 checked-in pricing snapshot 和實際 usage 計算。
- `free`：不計費；本地 Ollama 一律使用此狀態。
- `unpriced`：缺少 model identity、公開價格或必要計費維度，不能可靠換算。不得把它
  當成免費。

估價 snapshot 取自 OpenRouter `/api/v1/models` manifest；`prompt` 對應 input token、
`completion` 對應 output token、`input_cache_read` 對應 cache-read token、`web_search`
對應 search count，價格字串的單位皆是 USD per unit。計算使用 exact decimal arithmetic；
application 若要保存 `cost_cents`，只在自身 persistence boundary round 一次。SDK 不寫
`~/.config/agentsdk/data/cost` 或其他全域帳本。

```bash
go run . provider pricing refresh          # preview manifest diff
go run . provider pricing refresh --write  # update checked-in snapshot
```

## Antigravity — Google Cloud Code `v1internal`

走 `:generateContent`／`:streamGenerateContent?alt=sse`／`:fetchAvailableModels`／
`:loadCodeAssist`，body 是 Cloud Code envelope 包 Gemini `GenerateContent`；Gemini 與
Claude 共用同一個 envelope，family 由 model id 偵測（thinking config 大小寫、
tool-call signature、output-token ceiling 三者依 family 分歧）。

- credential 只有 OAuth Bearer（`Metadata.APIKeyEnv` 為空，api_key kind 在 resolve
  階段就被拒）。
- host chain 預設 daily → prod，`ANTIGRAVITY_BASE_URL` 一旦指定即取代整條 chain；
  只有 403/404/5xx 與 transport error 會換 host。
- project id 依序取 `WithProjectID` → `ANTIGRAVITY_PROJECT_ID` → `loadCodeAssist`
  （每個 Provider 最多一次）→ sentinel。
- thinking model 的 `Generate` 走 SSE 再 fold，因為 blocking endpoint 不回 thought
  part。`ModelSpec.Reasoning` 由 `isThinkingModel(id)` 推導而非手寫，與 SSE／blocking
  路由共用同一判斷，不會互相漂移。
- tool schema 由 `schema.go` 轉成 Google dialect（type 大寫、剔除 protobuf 沒有的
  keyword），否則整個 request 會被拒。
- unsigned reasoning part 不回送（gateway 驗簽），Gemini tool call 缺簽名時補
  `skip_thought_signature_validator`。
- `ListModels` 丟棄 `ContextWindow`／`MaxTokens` 為 0 的項目，濾掉 gateway 的 IDE
  內部 route（`chat_*`／`tab_*`）——代價是新 model 需先進 `CATALOG` 才會出現在清單。

image generation（`gemini-*-flash-image`）整張圖以單一 base64 `inlineData` part 放在
`一個` SSE frame，實測達 MB 級，故 antigravity 用
`sse.NewBoundedDecoder(r, MAX_STREAM_FRAME_BYTES)` 取代預設上限；`core.ModelChunk`
也因此帶 `Image`／`ImageMIME`——原本 `Kind` 可宣告 `PART_KIND_IMAGE` 卻無欄位承載，
image 會被靜默丟棄。`provider.ImageGenerator` 由 `antigravity.ImageProvider` 以
`同一個 chat surface` 實作（gateway 沒有影像端點，與 google/grok 的 `openaiimage`、
MiniMax 的 `/v1/image_generation` 機制不同），預設 model `gemini-3.1-flash-image`，
無 `ImageBaseURLEnv`；`Provider.generateWith` 是共用 transport 的 per-request model
縫，讓 image 與 chat 共用 session／project cache。`Size`/`Quality`/`Background` 等
chat surface 無法表達的欄位一律`明確拒收`不靜默忽略，`SubjectReferences` 則原生支援
（i2i 就是同一則訊息多一個 `inlineData` part，但只收 inline bytes、拒收 URL），
`Count` 以重複請求滿足並以 `MAX_IMAGE_COUNT` 設限。

## MiniMax

| Surface | Endpoint / 行為                                                                                                    | Base override             |
| ------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------- |
| Chat    | HTTP + SSE；vision 以 content blocks 編碼                                                                          | `MINIMAX_BASE_URL`        |
| Image   | `/v1/image_generation`（t2i + i2i 同 endpoint，預設 `image-01`；`Size` 收 `W:H` aspect ratio 或 `WxH` dimensions） | `MINIMAX_IMAGE_BASE_URL`  |
| Video   | 四種 mode（text / image / startend / subject）、asynchronous poll、authenticated download、MP4 verification        | `MINIMAX_VIDEO_BASE_URL`  |
| Music   | non-streaming text-to-music / cover；request validation、bounded response/error、typed API error                   | `MINIMAX_MUSIC_BASE_URL`  |
| Speech  | `t2a_v2`（Bearer、hex 解碼、`extra_info` → `SpeechInfo`，預設 `speech-02-hd`）                                     | `MINIMAX_SPEECH_BASE_URL` |

- `ImageRequest.SubjectReferences` 是 provider-neutral 的 i2i 輸入：MiniMax 編成
  `subject_reference`（type `character`，URL 或 data URI）；`openaiimage` codec
  （Google/Grok）明確拒收。
- speech 的 resolved base 尾段 `/anthropic` 一律 trim——`ResolvedConfig` 不帶來源，
  trim 不分 env 出處。
- voice list 走 `POST /v1/get_voice`（僅 `voice_type` 一參數），search 與 page-size
  由 adapter local 補齊、pagination token 直接拒收。
- caller 提供 `VideoRequest.OutputPath`，完成後取得 `VideoResult.Path`。

## ElevenLabs（audio-only，`Entry.New` 為 nil）

- `xi-api-key` header；multipart STT（`scribe_v2`）；TTS + `/stream`
  （`eleven_flash_v2_5`、預設 voice Rachel）。
- `GET /v1/models` live catalog 掛在 `SpeechProvider.ListModels`，非 chat client。
  該 endpoint 只列 synthesis models（bare JSON array，非 `{"data":[...]}`），
  scribe 不在其中，因此 STT static entry 於 `utils.Merge` 後補回；
  `scribe_v2_realtime` 是 websocket-only，batch STT route 會拒收，不進 catalog。
- `provider.VoiceLister` 的 request 詞彙以 ElevenLabs `GET /v2/voices`
  （search/category/pagination 皆 server-side）為標準，MiniMax 依此對齊。

`Register` 的「至少一 factory」檢查涵蓋 audio factories——ElevenLabs 是首個
`New == nil` 的 audio-only provider。`SpeechAsset.Bytes` 是 canonical decoded
bytes：hex 是 MiniMax wire 細節，於 adapter 內解碼。

## FunASR（local ASR，transcribe-only，`Entry.New` 為 nil）

自架 OpenAI-compatible HTTP server（`POST /v1/audio/transcriptions`，multipart）。

- 預設 `http://localhost:8000`（`FUNASR_BASE_URL` 覆寫）；keyless by default，
  `FUNASR_API_KEY` 只給 gateway-fronted 部署（`Authorization: Bearer`）。
- 一律送 `response_format=verbose_json`：`segments`（`句級` timing + nullable
  speaker）摺入 `Words`，`language` 的 `auto` placeholder 摺疊為空。
- response `duration` 是 server 處理耗時、非音訊長度，刻意不進 `Usage`；
  audio duration 由 request 端經 accounting fallback 補齊。
- `Audio.URL` 與 `Diarize` 無 wire 對應，`明確報錯`（diarization 由部署掛
  spk model 決定；有掛時 speaker 仍流入結果）。
- catalog model id 是部署端 `models.json` 的 key（預設 `sensevoice`）；
  pricing 一律 `free`（local provider，與 ollama 同 policy）。
- `GET /v1/models` live catalog 掛在 `TranscribeProvider.ListModels`（標準
  OpenAI `{"data":[...]}` envelope，走 `utils.DecodeIDList`）：live list（=
  server 的 models.json registry）決定 membership，bundled catalog 補 metadata，
  未知 id 仍標為 transcribe-capable（server 只註冊 ASR model）。transcriber 的
  decorator 與 accounting wrapper 都保留 `ModelLister` type assertion。
- 完整優缺點與 WebSocket / sherpa-onnx 取捨見 [`docs/funASR.md`](funASR.md)；
  docker 部署在 `~/projects/platform/inf`（`funasr` service）。

## Google — Gemini Live API（live / translate）

`BidiGenerateContent` websocket（`coder/websocket`；`GOOGLE_LIVE_BASE_URL` 覆寫，
API key 走 `?key=` query、OAuth Bearer 走 `Authorization` header，read limit 提高以
承載 inline PCM）。

- dialogue 預設 model `gemini-3.1-flash-live-preview`（`thinkingLevel`、prebuilt
  voice、input/output transcription 皆映射進 setup）。
- translation 是同一條 socket 加 `generationConfig.translationConfig`，預設 model
  `gemini-3.5-live-translate-preview`。
- 兩個 live model 以 `live` / `translate` capability 進 catalog，不宣告 `chat`；
  benchmark 因不具對應 case set，不會為它們產生套件。

## Codex — OpenAI Realtime API（live）

`wss://api.openai.com/v1/realtime?model=…`（`CODEX_LIVE_BASE_URL` 覆寫，預設 model
`gpt-realtime`）。這是 codex entry 唯一真實使用 `OPENAI_API_KEY` 的 surface
（chat surface 的 chatgpt.com backend 只收 OAuth），因此 adapter 對 Bearer-only
credential `明確拒收`並要求 api_key；`ThinkingLevel`／`Translation` 因 gpt-realtime
無對應 knob 一律明確拒收。

- handshake 是 `session.update` → `session.updated`（GA session shape：
  `output_modalities`、`audio.input.transcription`、`audio.output.voice`）。
- `SendText` = `conversation.item.create` + `response.create`；
  `SendAudio` = `input_audio_buffer.append`（24kHz PCM）。
- input transcript 只映射 `…transcription.completed` 不映射 delta（同時映射會讓
  caller 每句收兩次）；`response.done` → `TurnComplete`，`speech_started` →
  `Interrupted`，server `error` event 直接令 `Receive` 失敗。

live credential 在 connect 時解析一次（websocket handshake 驗證後 session 終身有效），
不逐 message 解析。`Entry.NewLive` 與 `Entry.NewTranslate` 共用
`Metadata.LiveBaseURLEnv`——每個註冊的 translator 都騎在同一條 realtime socket 上。

## Vision 輸入編碼 (Vision Input)

chat adapter 可把 `PART_KIND_IMAGE` 編碼成各家 wire 形狀；個別 model 是否接受 image
以 `ModelSpec.InputModalities` 為準：

| Adapter                                     | 形狀                      |
| ------------------------------------------- | ------------------------- |
| `anthropic`                                 | `image` source block      |
| `antigravity`                               | Gemini `inlineData`       |
| `grok`、`openaichat` codec（google/ollama） | `image_url` content array |
| `codex`                                     | `input_image`             |
| `minimax`                                   | content blocks            |

## Model catalog

除 codex 外的 chat adapter 皆實作 optional `provider.ModelLister`（ElevenLabs 掛在
`*SpeechProvider` 上；antigravity 走 `/v1internal:fetchAvailableModels`）。沒有
live lister 的 provider 讀 static `Entry.Catalog`。已知 live ID 會保留 bundled
`ModelSpec` 的 capability 與 directional modalities；未知 ID 不以名稱猜測 metadata。
