# CLI 參考 (CLI Reference)

root binary `agentsdk` 掛兩個子指令：`provider`（手動測試 provider surface，
`不`走 Agent/Engine/harness）與 `wizard`／`w`（產生 `agent.Config`，不打 provider、
不驗憑證）。兩者的 ownership 邊界見 [`CLAUDE.md`](../CLAUDE.md)。

## `provider` — provider surface 手動測試

```bash
go run . provider --list                                     # capability matrix
go run . provider --list-providers
go run . provider --list-models --provider minimax
go run . provider "ping" --provider minimax
go run . provider --stream "say hi in one word" --provider minimax
go run . provider "ping" --provider minimax --json | jq
go run . provider "a paper fox" --provider google --type image
go run . provider --provider elevenlabs --type transcribe --audio-file ./clip.mp3
```

- `--type` 選 API surface（預設 `chat`）：`chat` / `image` / `music` / `speech` /
  `transcribe`。不支援的 provider 回 typed `provider.ErrUnsupportedCapability`。
- `--list-models` 優先打 live `provider.ModelLister`，失敗才 fallback `Entry.Catalog`；
  audio-only entry 改由 speech client 取得 lister。
- catalog output 同時列出 model capabilities、input modalities 與 output modalities。
- credential 由 env 提供，對應 env 名稱在 `--list` 的 `AUTH ENV` 欄。

per-adapter 的 endpoint、預設 model 與 base override 見
[`providers.md`](providers.md)。

### Pricing snapshot refresh

```bash
go run . provider pricing refresh          # fetch + diff preview，不改檔案
go run . provider pricing refresh --write  # 明示更新 checked-in snapshot
```

refresh 從 OpenRouter public model manifest 讀取 per-unit USD prices，正規化後才顯示
差異。預設是 read-only preview；只有 `--write` 會更新 snapshot。成本狀態與計算邊界見
[`providers.md`](providers.md#usage-and-cost-accounting)。

## `wizard` — 設定產生器

```bash
go run . w                                  # 互動：逐階段問，Enter 收預設，寫 ./agent.yaml
go run . w -y --tier full -o -              # 非互動：全採預設，輸出 stdout
go run . w -y --tier oneshot -o agent.json  # 副檔名決定格式
go run . w --edit agent.yaml                # 以既有設定當預設值（round-trip 無損）
go run . w -o - --print-go                  # 額外印出等價的 Go literal
go run . w --list reasoning.style           # 列出單一欄位的選項
```

設定詞彙來自 `agent/spec`，provider 資料直接來自 `provider.Entries` / `Catalog`；
model picker 只列能作為 agent runtime 的 `chat` models。

## Sample 執行

```bash
export MINIMAX_API_KEY=...
go run ./sample/log-agent-v2 --interval 1m     # 先等 interval 再掃描
(cd sample/logdoctor-agent && go run . watch)  # 啟動即掃描

cd sample/code-agent
go run . --fake -p "看看這個專案"        # print 模式（進度走 stderr）
go run . --fake --json -p "test"         # stream-json envelope（wire）
go run . --fake                          # 互動 TUI（執行中輸入 = Steer）
go run . --fake --sessions               # 列出本目錄 sessions；-c / -r / --fork 續跑
go run . --provider anthropic -p "..."   # 改讀 ANTHROPIC_API_KEY
```

| Sample | 形態 | 備註 |
| ------ | ---- | ---- |
| `code-agent` | 全 harness 組合 CLI | `--provider minimax`（預設，`MINIMAX_API_KEY` / `MINIMAX_BASE_URL`）或 `--provider anthropic`；`--model` 留空用 adapter flagship 預設；`--api-key` / `--base-url` 為顯式覆寫 |
| `log-agent-v2` | scheduled listener | 固定 MiniMax，無 provider selector / fake mode / tools / approval / session UI；cursor 位於 `~/.config/log-agent-v2/data/log-cursor.json` |
| `logdoctor-agent` | 單一 `watch` command | 比較用的精簡 `agent.OnceStream` 路徑 |
| `file-agent`、`greet-agent` | 內建工具展示 | Anthropic-compatible adapter + `preset.Secure` |
| `skeleton-agent` | `wizard --print-go` 樣板 | 單檔；`stdinAgent` 把 stdin 塞進 Bootstrap 回傳的 opening state，對比 shape 見該目錄 README |
| `demo-*` | 單一 SDK 元件展示 | memory / middleware / strategy |

agent samples 皆將 Markdown 寫 stdout、`core.StreamEvent` JSONL 寫 stderr：

```json
{"kind":"run_start","run_id":"once-..."}
{"kind":"message","run_id":"once-...","turn":1,"text":"# Diagnosis\n..."}
{"kind":"run_end","run_id":"once-...","status":"completed"}
```

## Benchmark runner

```bash
go run ./benchmark/cmd -list
go run ./benchmark/cmd -provider minimax -model all
go run ./benchmark/cmd -provider google -model gemini-2.5-pro -capabilities chat
go run ./benchmark/gen                      # 重新產生 benchmark/pkg/* （亦掛 go:generate）
```

`-model all` 是全 catalog sweep（單一 model 失敗不中斷）；`-capabilities` 留空時，
已選 catalog model 依 provider support、model capabilities、input modalities 與
benchmark applicability 自動選擇 cases。顯式指定 unsupported capability 會執行並記錄
typed failure。結果落在 `benchmark/pkg/<pair-slug>/tmp/<session-id>/`（gitignored），
每筆 `Record` 以 `capability` 欄保存 operation。

## 依賴圖分析（外部 CLI）

```bash
go-dependency-analysis --workspace ./go.work --format text
```

`go-tool-fact` 來自當次 Go toolchain/build context，`policy-heuristic` 才是建議。
`unused-direct-candidate` 必須先檢查 tests、build tags、platform files 與 generated
code，不能直接刪 require。完整 flags 見該 repo 的 README。
