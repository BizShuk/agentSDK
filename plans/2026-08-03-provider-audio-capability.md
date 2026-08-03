# Provider Audio Capability（STT/TTS）+ ElevenLabs / MiniMax Adapter — 擴充計畫

日期：2026-08-03
狀態：**已核准動工**（2026-08-03 user review）——修訂：STS 移出本期範圍、
加入 MiniMax speech 直接接入；打真 API 的 live smoke 仍需另行核准。
發起方：[customer_service/plans/2026-08-03-customer-service-voice-agent.md](../../customer_service/plans/2026-08-03-customer-service-voice-agent.md)

## 0. 設計原則（本次 review 定調）

agentSDK 是**通用層**：audio capability contract 為 provider-neutral，
消費端（如 customer_service）只依賴 `provider.Transcriber` /
`SpeechGenerator` 介面，不綁定任何供應商；adapter 一律 stdlib `net/http`
直接對接官方 wire，不引入 vendor SDK／第三方封裝層。兩個獨立 adapter
（ElevenLabs、MiniMax）同時實作同一 contract，用以證明介面確實通用。

## 1. 現況與缺口

`provider/` 已有 image / video / music 三個 media capability 擴充模式：
capability 常數 → 單一介面 contract 檔 → `Entry` factory → `provider.NewXxx`
→ per-request `Decorator` → typed `ErrUnsupportedCapability`。
`provider/sample` matrix 預留的 audio 欄目前回 typed unsupported——
本計畫補上 production contract 與兩個 adapter。

## 2. Capability 面設計

兩個獨立 capability（沿用「一介面一 capability」慣例，granular
unsupported error 才有意義——MiniMax 就只做 TTS）：

| Capability | 介面 | ElevenLabs | MiniMax |
| ---- | ---- | ---- | ---- |
| `audio_transcribe` | `Transcriber.Transcribe` | `POST /v1/speech-to-text`（scribe_v1） | —（unsupported） |
| `audio_speech` | `SpeechGenerator.GenerateSpeech` | `POST /v1/text-to-speech/{voice_id}` | `POST /v1/t2a_v2`（speech-02-hd 等） |
| （optional）streaming | `SpeechStreamer.StreamSpeech → io.ReadCloser` | `/stream` endpoint | 本期不做 |

Request/Result 形狀：`TranscribeRequest{Model, Audio AudioSource, Language,
Diarize, Auth}` → `TranscribeResult{Text, Language, Words[]}`；
`SpeechRequest{Model, Text, Voice, OutputFormat, VoiceSetting, Auth}` →
`SpeechResult{Audio SpeechAsset{Bytes, Format}, Info SpeechInfo}`。
`SpeechAsset.Bytes` 是 canonical decoded bytes——MiniMax 回傳的 hex 是
wire 細節，由 adapter 解碼，不外洩給 caller（對比 `MusicAsset.Hex` 的
歷史包袱，audio 面不重複）。

Registry 接線：`Entry` 加 `NewTranscriber` / `NewSpeech` factory、
`provider.NewTranscriber` / `NewSpeech` 建構路徑、兩個 decorator wrapper
（speech wrapper 需保留底層的 `SpeechStreamer` 能力）；`Register` 的
「至少一個 factory」panic 條件放寬納入 audio——ElevenLabs 是第一個
`New == nil`（無 chat model）的 provider。`Metadata` 加一欄
`SpeechBaseURLEnv`（比照 `MusicBaseURLEnv`，實作時定案）。

## 3. Adapters

- `provider/elevenlabs`（新 package，mirror minimax 佈局）：
  `xi-api-key` header（非 Bearer）、`ELEVENLABS_API_KEY` /
  `ELEVENLABS_BASE_URL` env、預設 base `https://api.elevenlabs.io`、
  預設 TTS `eleven_flash_v2_5`（低延遲）＋預設 voice Rachel、STT
  `scribe_v1`（multipart 上傳，words 時間戳 → ms）。static catalog，
  無 `core.ModelLister`（無 chat adapter 可掛）。
- `provider/minimax` 擴充：`NewSpeech` 走 `t2a_v2`（Bearer、既有
  music client 紀律：bounded read、`base_resp` typed error）、hex 解碼、
  `extra_info` → `SpeechInfo`；speech 模型入 DefaultCatalog。
  Base URL（實作時定案）：`MINIMAX_SPEECH_BASE_URL` 覆寫（比照 music/video，
  speech 解析不消費 `MINIMAX_BASE_URL`——chat 慣例指向 anthropic-compat
  surface，t2a_v2 在帳號根路徑）；resolved base 尾段 `/anthropic` 一律
  trim（`ResolvedConfig` 不帶來源，無法只對 fallback 生效，如實文件化）。

Env 備註：user 環境另有 `MINIMAX_API_BASE`——SDK canonical 維持
`MINIMAX_BASE_URL`，由消費端以 `Options.LookupEnv`（registry 為此而設的
注入縫）做 fallback 對照，不進 Metadata。

## 4. 驗證計畫

- 單元（免核准，全 local httptest/fake，絕不打真 API）：request Validate、
  registry unsupported / decorator precedence / Register 放寬、streaming
  能力保留、兩個 adapter 的 wire 測試（header/multipart/hex/typed error/
  bounded body）。`go build ./... && go vet ./... && go test ./...` 全綠。
- `provider/sample --list`：audio 欄轉為真實 capability 呈現。
- Live smoke（花錢，**需明確核准**）：TTS 一句 → STT 回讀 round-trip。
  核對表（httptest 已編碼的 wire 假設，跑前對照官方 docs / 回應）：
  ElevenLabs `POST /v1/text-to-speech/{voice}`(+`/stream`) + `output_format`
  query、`POST /v1/speech-to-text` multipart（`file`/`cloud_storage_url`、
  `model_id`）、`xi-api-key` header、STT words `start`/`end` 秒 → ms 換算；
  MiniMax `POST /v1/t2a_v2` Bearer、`voice_setting`/`audio_setting` 省略
  語意、`data.audio` hex、`base_resp.status_code != 0` 為 API error
  （1004 auth / 1002 rate limit 已入測試）；另核對 `extra_info` 欄位名
  （audio_length/audio_sample_rate/audio_size/audio_bitrate/audio_channel，
  名稱不符會靜默讀零）與 `data.status != 2` 是否可能伴隨可解碼 payload
  （目前不 gate status，只要求非空可解碼 hex）。ElevenLabs 另核對 STT
  multipart 檔名副檔名是否需精確（目前 `audio.<format 首段>`，假設
  container sniffing）。
- 收尾同步：`CLAUDE.md`、`README.todo`、`docs/CHANGELOG.md`。

## 5. 明確不做 (Non-goals)

- STS（voice conversion）——2026-08-03 review 移出，待有 consumer 再議。
- ElevenLabs Conversational AI（Agents 平台）adapter——推理迴圈屬本 SDK。
- Realtime WS（STT realtime / TTS `stream-input`）——customer_service M5
  有量測數據後再立計畫。
- MiniMax TTS streaming（SSE + hex chunk）——同上。
- 「scope 資料夾」機制不進 SDK。
