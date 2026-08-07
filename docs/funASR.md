# FunASR Provider — 相容性評估與取捨 (Compatibility and Trade-offs)

`provider/funasr` 對接自架的 FunASR OpenAI-compatible HTTP server
（upstream `examples/openai_api/server.py`；docker 部署見
`~/projects/platform/inf` 的 `funasr` service）。本文記錄選擇 HTTP 接入的
優缺點，以及與 WebSocket runtime、sherpa-onnx 本地 in-process 兩條替代路徑的取捨。
sherpa-onnx 的完整評估另見 [`docs/sherpa-onnx.md`](sherpa-onnx.md)。

## 接入方式 (Integration Shape)

- Endpoint：`POST /v1/audio/transcriptions`（multipart：`file` / `model` /
  `language` / `response_format=verbose_json`）。
- 預設 `http://localhost:8000`，由 `FUNASR_BASE_URL` 覆寫；CLI 路徑經
  `Options.LookupEnv` 注入 viper-backed lookup，config 檔（`viper.Get*`）
  即可參與，provider package 本身不 import viper。
- Keyless by default（比照 ollama）：`FUNASR_API_KEY` 只在前面架 gateway 時使用，
  以 `Authorization: Bearer` 送出。
- Cost：`pricing.Estimate` 將 `funasr` 視為 local provider，一律回 `free`
  （與 ollama 同一條 policy）。

## 與現有 `provider.Transcriber` 的欄位對應 (Field Mapping)

| Transcriber 契約 | FunASR HTTP | 說明 |
| --- | --- | --- |
| `Audio.Bytes` + `Format` | multipart `file`（副檔名取自 Format 首段） | 完整對應 |
| `Audio.URL` | 無 | `明確報錯`，不靜默忽略 |
| `Model` | form `model` | server 端 `models.json` 的 registry key |
| `Language` | form `language` | 完整對應 |
| `Diarize` | 無 | `明確報錯`：diarization 由部署（spk model）決定，非 request-time 選項；server 有掛 spk 時 segments 的 speaker 仍會流入結果 |
| `Text` / `Language` | `text` / `language` | `auto` placeholder 摺疊為空字串 |
| `Words` | `segments[{text,start,end,speaker}]` | `句級` segments，非字級 timing |
| `Usage.AudioDurationMilliseconds` | 無（response `duration` 是`處理耗時`非音訊長度，刻意不採用） | 由 request 端 `DurationMilliseconds` 經 accounting fallback 補齊 |

## 優點 (Advantages)

- `零新依賴`：與 ElevenLabs 同型的 multipart HTTP 流程，root module 保持
  CGO-free、無 websocket client、可交叉編譯。
- `契約對齊度高`：OpenAI `/v1/audio/transcriptions` 是業界共用形狀
  （OpenAI/Groq/faster-whisper-server 皆同），adapter 不綁死單一 vendor 部署。
- `本地免費`：資料不出機器，無 per-minute 計費；隱私敏感音訊可用。
- `模型可換`：server 端 `models.json` 增列模型即可，SDK 端 catalog 只是宣告。
- `部署簡單`：單一 docker service + `/health`；模型由 modelscope/hf 自動下載進
  cache volume。

## 缺點 (Disadvantages)

- `hotwords / itn 喪失`：HTTP 參考實作沒有這兩個 form 欄位，中文 ASR 最重要的
  調校旋鈕無法從 request 端下達（WebSocket 協議有）。
- `句級 timing`：`segments` 是句級，無 WebSocket `stamp_sents` 的字級 `ts_list`。
- `SenseVoice 標記被剝除`：server 端 `clean_text()` 移除 `<|...|>` 情緒/事件
  標籤，adapter 拿不到。
- `無串流`：不支援 `online` / `2pass` 即時結果；SDK 目前也沒有
  `TranscribeStreamer` 對應面。
- `參考實作等級`：upstream 是 `examples/` 下的 FastAPI 單機 server，無認證、
  無多租戶，生產環境要自備 gateway 與維運。
- `Diarize 不可請求`：speaker 輸出取決於部署是否掛 spk model，非 API 開關。

## 三條路徑的取捨 (Trade-offs: HTTP vs WebSocket vs In-process)

| 面向 | HTTP（`已採用`） | FunASR WebSocket runtime | sherpa-onnx in-process |
| --- | --- | --- | --- |
| 依賴成本 | 無新依賴 | websocket client + 自簽 `wss` 信任設定（`ResolvedConfig` 無 TLS 縫） | cgo + 每平台 ~40-130MB prebuilt dylib，root module 失去交叉編譯 |
| 串流 | ❌ | ✅ `online`/`2pass`（即時 + 句末修正） | ✅ `OnlineRecognizer` |
| hotwords / itn | ❌ | ✅ | ✅（HotwordsFile） |
| timing 粒度 | 句級 | 字級（`ts_list`） | token 級 |
| 模型管理 | server 端集中，改 config 即換 | server 端集中（部署參數） | 呼叫端自管本地模型檔（encoder/decoder/tokens 路徑） |
| 延遲 | 每次 HTTP round-trip | handshake 一次後長連線 | 無網路，但模型載入昂貴且 recognizer 需自管生命週期（無 Close 縫） |
| 契約相容 | 幾乎無縫 | transport/credential/sample-rate 多處錯位 | 輸入（PCM-only）/model 定址/生命週期三處錯位 |

結論：以目前 `Transcriber` 是 blocking batch contract 而言，HTTP 是唯一
`不需要動介面`的路徑，故先採用。未來若需要即時字幕或 hotwords，正確的演進是
先設計 optional `TranscribeStreamer` capability（比照 `SpeechStreamer` 的
type-assertion discovery），屆時 WebSocket runtime 或 sherpa-onnx 再接入，
而不是在 batch contract 上硬塞串流語意。

## 預設 Catalog 與推薦 (Default Catalog)

Model id 是部署端 `models.json` 的 key；推薦理由見
`provider/funasr/models.go` 註解。摘要：

| Model | 大小 | 定位 |
| --- | --- | --- |
| `qwen3-asr-0.6b` | 0.6B | 🏆 最佳綜合性價比（中/英/粵 ⭐5，52 語言） |
| `qwen3-asr-1.7b` | 1.7B | 🏆 精度優先 |
| `fun-asr-nano` | 0.8B | 🏆 中文複雜場景（中/英/日+方言） |
| `fun-asr-mlt-nano` | 0.8B | FunASR 多語言（31 語言） |
| `sensevoice` | 234M | 🏆 CPU/低成本首選（`預設 model`） |
| `paraformer-zh` | 220M | 中文 + timestamp/hotword |
| `whisper-large-v3` | 1.55B | 老牌高精度 baseline |
| `whisper-large-v3-turbo` | ~809M | 🏆 Whisper 實用首選 |
| `parakeet-tdt` | 視版本 | 英文高速 ASR（需 NeMo runtime） |

## 手動測試 (Manual Test)

```bash
# 先啟動 server（見 ~/projects/platform/inf）
agentsdk provider transcribe --provider funasr --audio-file sample.wav
agentsdk provider transcribe --provider funasr --model paraformer-zh \
  --language zh --audio-file meeting.wav --json
```
