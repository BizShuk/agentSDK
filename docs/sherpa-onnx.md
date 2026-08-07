# sherpa-onnx 與 agentsdk transcribe 契約相容性評估

評估對象: [k2-fsa/sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx) 的 Go SDK
（`github.com/k2-fsa/sherpa-onnx-go`，Apache-2.0，評估時版本 v1.13.4）。
比對基準: `provider.Transcriber` 契約（`provider/transcribe.go`）、registry
construction pipeline（`provider/registry.go`、`registry_options.go`）與
accounting（`provider/accounting.go`、`provider/pricing`）。

本文是評估紀錄，不是已實作功能的描述。若日後落地 adapter，wire 細節歸
[`docs/providers.md`](providers.md)，boundary 決策歸根目錄 `CLAUDE.md`。

## SDK 形狀 (SDK Shape)

- Module 佈局: umbrella module `sherpa-onnx-go` 依 build tag 轉發到
  `sherpa-onnx-go-{linux,macos,windows}`；每個平台 module 內含 prebuilt
  dynamic libraries（macOS 約 129MB、Linux 94MB、Windows 40MB，umbrella
  一次 require 三平台，module cache 約 260MB）。
- 必須開 cgo；LDFLAGS 以 `${SRCDIR}/lib/<triple>` 設 rpath，binary 執行期
  綁 module cache 內的 dylib 路徑，發佈需自行隨附 shared libraries。
- 推論全在本地: `OfflineRecognizer`（batch）與 `OnlineRecognizer`
  （streaming）各配 `OfflineStream` / `OnlineStream`，pipeline 為
  `AcceptWaveform([]float32 PCM) → Decode → GetResult`。
- Model 是一組本地檔案路徑（encoder/decoder/tokens 等），`OfflineModelConfig`
  有 18 種 model 家族（Whisper、SenseVoice、Paraformer、Transducer、
  Moonshine、Canary…）擇一填寫，另有 `NumThreads`、`Provider`
  （cpu/cuda/coreml）、`DecodingMethod`、`HotwordsFile` 等旋鈕。
- 同一 SDK 另提供 `OfflineTts`、`VoiceActivityDetector`、
  `OfflineSpeakerDiarization`、punctuation、keyword spotting、denoiser、
  spoken language identification 等能力。

## 契約對應 (Contract Mapping)

| agentsdk 契約 | sherpa-onnx 對應 | 判定 |
| --- | --- | --- |
| `Transcribe(ctx, TranscribeRequest) (TranscribeResult, error)` | `NewOfflineStream → AcceptWaveform → Decode → GetResult` | 可實作 |
| `TranscribeResult.Text` | `OfflineRecognizerResult.Text` | 直接對應 |
| `TranscribeResult.Language` | `OfflineRecognizerResult.Lang`（SenseVoice/Whisper 類 model 才回填） | 條件對應 |
| `TranscribeResult.Words` | `Tokens` + `Timestamps` + `Durations` | 有損（token 級，見缺口） |
| `Usage.AudioDurationMilliseconds` | 由 `len(samples) / sampleRate` 換算 | 可補齊 |
| `TranscribeRequest.Language` | Whisper 的 `Language` 欄位；其餘家族由 model 本身決定 | 部分對應 |
| `TranscribeRequest.Diarize` | 無；需另跑 `OfflineSpeakerDiarization` | 不對應 |
| `TranscribeRequest.Auth` / `Decorator` | 本地推論無 credential | 空轉（無害） |
| `AudioSource.URL` | 無網路能力 | adapter 須自行抓取或報錯 |
| `AudioSource.Bytes`（encoded） | 只吃 `[]float32` PCM 或 `ReadWave(檔名)` | 需 adapter 自備解碼 |

## 主要缺口 (Gaps)

依阻力由大到小:

1. 建置成本: root module 目前零 cgo、可任意交叉編譯。收進 root module 會
   讓全部 consumer 付 cgo + 260MB module cache + dylib 發佈成本。
2. 輸入形狀: `AudioSource` 給 encoded bytes/URL，sherpa 只收 PCM float32
   或 wav 檔名，無 in-memory mp3/m4a decoder。adapter 必須自備 wav 解碼
   （或落 temp file），非 wav 格式須明確報錯或外掛轉檔。
3. Model 定址: `TranscribeRequest.Model` 與 `ResolvedConfig.Model` 都是單一
   字串，sherpa 需要「家族 + 多個檔案路徑」。需要一條約定（例如 Model =
   本地模型目錄名，root 由 env 提供，家族由目錄內容推斷），這是唯一可能
   反壓 `ResolvedConfig` 的設計點。
4. 生命週期: recognizer 是 C 指標，模型載入昂貴、必須跨請求重用，但
   `Transcriber` 沒有 Close/Shutdown 縫。建議以 type assertion 發現
   optional `io.Closer`（沿用 `SpeechStreamer` 的 optional capability 慣例），
   並在 adapter 內自行序列化並發。
5. Words 語意: sherpa 的 timestamps 是 token 級（BPE 或漢字），映射到
   `TranscribedWord` 是有損的；`Speaker` 欄位在純 ASR 路徑恆空。
6. Diarize: 需要 segmentation + embedding 兩個額外模型，再與 ASR 結果自行
   對齊，不是開關即得。第一版應對 `Diarize: true` 回明確錯誤。
7. 計費: `pricing.Estimate` 目前硬編只有 `ollama` 回 `free`，其他本地
   provider 會落成 `unpriced`。落地前應把 free policy 從單一名稱一般化為
   本地/免費 provider 集合。

## 可延伸項 (Extension Candidates)

與 FunASR、ElevenLabs、Whisper 系服務交叉比對後，以下擴充已達跨 vendor
的 canonical 門檻，不算為單一 vendor 特化:

- `AudioSource.SampleRate` 明確欄位: sherpa `AcceptWaveform` 與 FunASR
  `audio_fs` 都需要，取代 `"pcm_16000"` 這種字串約定。
- `TranscribeRequest.Hotwords`: sherpa `HotwordsFile`、FunASR `hotwords`、
  ElevenLabs keyterms、Whisper prompt 四家共有。
- Optional `TranscribeStreamer` interface: sherpa `OnlineRecognizer` 與
  FunASR 2pass 都能實作，與 TTS 側 `SpeechStreamer` 對稱，以 type assertion
  發現。
- 句級 `Segments`（Text/StartMs/EndMs/Speaker）: 讓 diarization 與句級
  timestamp 不必壓進 word 級 `Words`。

超出現有 capability 詞彙的能力（VAD、punctuation、denoiser、keyword
spotting、language identification）不建議為其擴張 `core` 或
`provider.Capability`。

## 建議落點 (Recommendation)

- 做成獨立 module 的 out-of-tree adapter，不進 root module。registry 本來
  就是 adapter 從自身 `init()` 呼叫 `provider.Register` 自註冊、`provider`
  不 import 任何 adapter 的設計，out-of-tree 掛載無須改動 SDK；root module
  維持 CGO-free 與可交叉編譯。
- Credential 照 ollama 慣例: 宣告 `APIKeyEnv` 但不設 `CredentialRequired`。
- 第一版範圍: wav-only 輸入 + 無 diarize + `Diarize`/非 wav 格式明確報錯，
  端到端可跑後再考慮 streaming、diarization 與 TTS
  （`OfflineTts.GeneratedAudio.ToBuffer()` 可順帶填 `CAPABILITY_SPEECH`）。

## 對照: FunASR 路徑 (Alternative: FunASR)

若目標只是「本地/自架中文 ASR」而非嵌入式推論，FunASR 的 OpenAI 相容
HTTP 服務（`examples/openai_api`，`/v1/audio/transcriptions` multipart）
與現有契約幾乎零缺口: 不需 cgo、不需 websocket、`Language` 與 `Diarize`
（server 掛 spk model 時）都有真實對應，且該端點形狀與 OpenAI/Groq/
faster-whisper-server 共通，值得抽成 `provider/protocol/openaiaudio` 共用
codec。代價是 hotwords/itn 在 HTTP 表面不可用、需自行維運 server。

sherpa-onnx 的價值在單一 binary 免部署與離線推論；FunASR HTTP 的價值在
零契約摩擦。兩者不互斥: 前者是 embedded adapter，後者是 protocol codec。
