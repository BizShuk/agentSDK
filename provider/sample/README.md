# Provider Sample

這個 sample 直接呼叫 `provider` package，不經 `agent`、`runtime.Engine` 或 tool loop。
它展示三個正交選擇：

- provider：`anthropic`、`antigravity`、`codex`、`google`、`grok`、
  `minimax`、`ollama`。
- auth：`auto`、`api_key`、`oauth`。
- API type：`chat`、`image`、`music`、`audio`。

## 查看 capability matrix

```bash
go run ./provider/sample --list
```

輸出會列出每個 linked provider 的 `chat / image / music / audio` 支援狀態，以及實際查詢的
API key / OAuth environment variables。

`provider/all` 只負責 blank-import adapters；provider metadata 與 capability 仍以
`provider.Entry` 為唯一真相。

## Auth

sample 不接受 command-line secret，避免 API key 出現在 shell history 或 process list。
請使用 provider 自己的 environment variable：

```bash
export ANTHROPIC_API_KEY=...
export ANTHROPIC_OAUTH_TOKEN=...
export GOOGLE_API_KEY=...
export XAI_API_KEY=...
export MINIMAX_API_KEY=...
```

模式：

- `--auth auto`：OAuth env 優先，再查 API key env。
- `--auth api_key`：只接受該 provider 的 API key env，缺少時立即報錯。
- `--auth oauth`：只接受該 provider 的 OAuth env；不具 OAuth metadata 的 provider
  立即報錯。

這裡示範的是 environment credential。需要 stored credential refresh 時，production
composition root 應建立 `provider/credential.Source`，再把 `Source.Decorator()` 傳給
`provider.Options.Decorator`。

## Chat

```bash
# Local Ollama，不需要 credential
go run ./provider/sample \
  --provider ollama \
  --type chat \
  --model llama3.2 \
  "用一句話介紹新加坡"

# Anthropic API key
ANTHROPIC_API_KEY=... go run ./provider/sample \
  --provider anthropic \
  --auth api_key \
  --type chat \
  --model claude-sonnet-5 \
  "summarize this design"

# Anthropic pre-issued OAuth token
ANTHROPIC_OAUTH_TOKEN=... go run ./provider/sample \
  --provider anthropic \
  --auth oauth \
  --type chat \
  "reply with one word"

# 完整 ModelResult
GOOGLE_API_KEY=... go run ./provider/sample \
  --provider google \
  --type chat \
  --json \
  "hello"
```

## Image

目前 registry 中 `google` 與 `grok` 實作 `provider.ImageGenerator`：

```bash
GOOGLE_API_KEY=... go run ./provider/sample \
  --provider google \
  --type image \
  --model gemini-2.5-flash-image \
  "新加坡雨夜的電影感街景"

XAI_API_KEY=... go run ./provider/sample \
  --provider grok \
  --type image \
  --model grok-imagine-image-quality \
  --json \
  "a geometric fox"
```

非 JSON 模式只顯示 URL、base64 長度、MIME type 與 usage，避免把大型 base64 payload
直接灌進 terminal；需要完整 payload 時才加 `--json`。

## Music

目前 registry 中 `minimax` 實作 non-streaming `provider.MusicGenerator`。以下指令對應
MiniMax one-step cover request：

```bash
MINIMAX_API_KEY=... go run ./provider/sample \
  --provider minimax \
  --auth api_key \
  --type music \
  --model music-cover \
  --audio-url https://example.com/original-song.mp3 \
  --output-format url \
  --sample-rate 44100 \
  --bitrate 256000 \
  --audio-format mp3 \
  "Jazz, smooth, late night lounge, saxophone"
```

非 JSON 模式只印 `music.url` 或 hex 字元數與 metadata，不把大型 audio payload 直接灌進
terminal。Music endpoint 可用 `MINIMAX_MUSIC_BASE_URL` 獨立覆寫；它不會沿用指向
Anthropic-compatible model endpoint 的 `MINIMAX_BASE_URL`。

## Audio

```bash
go run ./provider/sample \
  --provider google \
  --type audio \
  "say hello"
```

目前會回傳可由 `errors.Is(err, provider.ErrUnsupportedCapability)` 判斷的 typed error。
原因是 audio 尚未定義單一 production contract：`speech synthesis`、`transcription` 與
`audio-chat input` 是三種不同 wire API；現有 adapters 也尚未轉譯
`core.Part{Kind: PART_KIND_AUDIO}`。在語意與真實 adapter consumer 確定前，sample
不會靜默丟棄音訊或把其中一種 API 假裝成全部 audio。已落地的 music generation 是
獨立 `MusicGenerator`，不代表 generic audio contract 已定義。
