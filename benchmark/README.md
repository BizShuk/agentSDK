# benchmark — provider-model capability samples

以`預定義輸入` (text prompt / image / audio) 打真實 provider API, 觀察各
provider-model 的實際輸出 (text / image / audio / video), 並把每次結果落地保存。

## Flow

```text
run → iterate cases → query provider with (prompt, model) → store result
                          └─ 任一 case 失敗: 報告後跳到下一個 case
```

流程程式碼在 `./benchmark` root package (`benchmark.go` / `case.go` /
`store.go` / `media.go` / `catalog.go`); 每個 provider-model pair 是 `pkg/`
下一個獨立可執行套件。model 是每個 kind 都有的軸: chat model 由
`Target.Model` 釘住, media model (image/speech/transcribe/video/music) 由
`benchmark.WithModel` 釘在 case 上 —— 兩者一律取自該 provider 的
`DefaultCatalog`, off-catalog 的 model 會在執行時逐一印 warning。

`pkg/` 下的套件`全部由 `./gen` 產生` (每檔帶 `DO NOT EDIT` 標記, 勿手改):
registry × DefaultCatalog × `KindsOf` 展開成每個 runnable model 一個套件,
目前 84 個。catalog 或 `KindsOf` 變更後重新產生:

```bash
go run ./benchmark/gen        # 等價 go generate ./benchmark
```

重新產生只覆寫 main.go, 不動 `tmp/` 結果; 已離開 catalog 的 model 其產生檔
會被移除 (目錄與 tmp/ 歷史保留)。benchmark 無法驅動的 model `沒有`套件:
ElevenLabs STS (SDK 無 speech-to-speech surface), MiniMax S2V-01 (需 subject
image), music-cover (需 reference audio), Google TTS/Lyria (adapter 無對應
factory), Ollama embedding, 以及 grok 的 image model (catalog 未收錄)。

## Run

單一 provider-model (`pkg/` 下每 model 一個目錄):

```bash
export MINIMAX_API_KEY=...                        # 各 pair 的 credential 見其 main.go 註解
go run ./benchmark/pkg/minimax-m3
go run ./benchmark/pkg/elevenlabs-eleven-v3

for d in benchmark/pkg/*/; do                     # 跑全部 pair (缺 credential 的 case 會 FAIL 並被跳過)
  go run "./$d"
done
```

臨時組合走 `cmd` (任意 provider × model × kinds):

```bash
go run ./benchmark/cmd -list                                    # 每個 catalog model 標註可跑的 kind
go run ./benchmark/cmd -provider minimax -model MiniMax-M3      # kinds 省略 = 該 provider 支援的全部
go run ./benchmark/cmd -provider google -model gemini-2.5-pro -kinds chat
go run ./benchmark/cmd -provider elevenlabs -kinds speech -model eleven_v3
```

`-model` 作用於 chat cases; 當選定的 kinds `不含` chat 時 (audio/media-only
run), 它改作用於全部選定 cases —— audio model 也是 model。

`-model all` 一鍵掃描整個 provider 的 catalog (與逐一跑 `pkg/` 套件等價,
結果同樣按 model 分目錄):

```bash
go run ./benchmark/cmd -provider elevenlabs -model all   # scribe + 6 TTS; 2 個 STS model 報 skip
go run ./benchmark/cmd -provider minimax -model all      # chat×3, image×2, video×3, music×2, speech×8
```

每個 model 跑哪種 kind 由 `benchmark.KindsOf` 對照 (`-list` 可預覽);
無法驅動的 model 報 `skip`, 單一 model 失敗不會中斷整個 sweep。

credential env 對照可用 `go run . provider --list` 檢視完整
provider × capability × auth-env matrix。

## Result layout

每次執行 (session) 以日期命名, 存在該 pair 的 `pkg/<pair-slug>/tmp/` 下
(git ignored); `cmd` 依 `-provider`/`-model` 算出同一個 pair-slug, 與固定套件
共用結果位置:

```text
benchmark/pkg/<provider-model>/tmp/
└── 20260806-153000/            # session id (date)
    ├── case-01-text-basic/
    │   ├── meta.json           # provider/model/prompt/duration/status/error
    │   └── output.txt
    ├── case-04-text-to-image/
    │   ├── meta.json
    │   └── output-1.png        # 或 output-1.url (provider 只回 URL 時)
    └── summary.json            # 全部 case 的 Record 彙總
```

## Cases

| Set                | Kind       | 輸入                        | 輸出           |
| ------------------ | ---------- | --------------------------- | -------------- |
| `ChatCases`        | chat       | text ×2, image (vision) ×1  | text           |
| `ImageCases`       | image      | prompt                      | image / url    |
| `SpeechCases`      | speech     | text                        | audio          |
| `TranscribeCases`  | transcribe | `testdata/tone.wav`         | text           |
| `VideoCases`       | video      | prompt                      | mp4            |
| `MusicCases`       | music      | prompt + lyrics             | audio / url    |

`testdata/tone.wav` 是純正弦音, transcribe 的預期 transcript 為空 —— 該 case
驗證的是上傳與解碼管線; 要看辨識品質請把 `InputFile` 換成真實語音檔。
新增 model 不手寫 pair: 進了 provider 的 `DefaultCatalog` (必要時補 `KindsOf`
對照) 後跑 `go run ./benchmark/gen` 即自動出現對應套件。
