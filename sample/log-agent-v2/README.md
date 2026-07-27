# log-agent-v2

`log-agent-v2` 是使用完整 `agent/` framework 的持續日誌分析範例。它固定
使用 MiniMax，每分鐘讀取 `~/.config/*/logs/*` 的新增內容，並只提供診斷與
安全修復建議，不會修改檔案或執行修復指令。

## 業務領域 (Business Domain)

### 持續日誌診斷 (Continuous Log Diagnosis)

排程器在 interval 到期後建立一個最多 `1 MiB` 的 log batch，再透過
`agent.WithListener` 將該批內容送入新的 `agent.Run`。分析成功且輸出完成後
才提交 cursor，因此失敗的 batch 會在下一個 interval 重試。

`領域流程 (Domain Flow):`

1. `time.Ticker` 等待下一個 interval；啟動時不立即掃描。
2. `Reader.Next` 讀取尚未處理的 direct regular log files。
3. `batchListener` 建立一筆 `core.Observation`。
4. `agent.New` 以 `oneshot` tier 組裝 MiniMax、listener 與 output sink。
5. `agent.Run` 使用該 batch 的唯一 `RunID` 完成一次分析。
6. agent 與輸出都成功後，`Reader.Commit` 才原子更新 cursor。

`核心實體 (Key Entities):` `Batch`、`LogPart`、`core.Observation`

`相關進入點 (Entry Points):` `run()`、`watchLoop()`、`runBatch()`

## 執行方式 (Usage)

在 agentsdk repo 根目錄執行：

```bash
export MINIMAX_API_KEY=...
go run ./sample/log-agent-v2 --interval 1m
```

v2 沒有 `watch` 子指令；唯一選項是 `--interval`，預設值為 `1m`。第一次
掃描會在第一個 interval 到期後發生。按 `Ctrl-C` 或傳送 `SIGTERM` 即可停止。

查看 CLI：

```bash
go run ./sample/log-agent-v2 -h
```

## 輸出 (Output)

| Stream | 內容 |
| ------ | ---- |
| stdout | 每個非空 batch 的 MiniMax Markdown diagnosis |
| stderr | `core.StreamEvent` JSON、`agent.Run` lifecycle JSON、掃描與執行錯誤 |

一般 terminal 會同時顯示 stdout 與 stderr。`message` event 的 JSON 也包含
diagnosis，因此畫面會看到 JSON event 與可讀 Markdown；它們不是同一個 stream
的重複輸出。

要分開保存：

```bash
go run ./sample/log-agent-v2 --interval 1m \
  > diagnoses.log \
  2> events.log
```

## 讀取與游標 (Reading and Cursor)

- 只讀取 `~/.config/<app>/logs/<direct-regular-file>`。
- 不跟隨 app directory、`logs/` directory 或 log file symlink。
- 排除 `~/.config/log-agent-v2/`，避免分析自己的輸出。
- 每個 batch 的 raw log 合計最多 `1 << 20` bytes。
- 沒有新增內容時不呼叫 MiniMax。
- cursor 位於 `~/.config/log-agent-v2/data/log-cursor.json`。
- log truncate 後，該檔案從 offset `0` 重新讀取。
- agent、provider 或輸出失敗時不提交 cursor；下一輪會重讀相同內容。

這是 `at-least-once` 行為：錯誤時可能再次分析同一批，但不會因 provider
失敗而跳過尚未成功處理的 bytes。

## v1 與 v2 (V1 vs V2)

| 項目 | `sample/logdoctor-agent` v1 | `sample/log-agent-v2` |
| ---- | --------------------------- | --------------------- |
| Model path | `agent.OnceStream` 直接接 prompt | `agent.New` + `agent.Run` |
| Log injection | prompt argument | `agent.WithListener` |
| Output | callback + caller 寫 stdout | `agent.WithSink` |
| 第一次掃描 | 啟動時立即執行 | 先等待一個 interval |
| CLI | `watch --interval 1m` | `--interval 1m` |
| Run boundary | one-shot facade，不建立 `Agent`/`Host` | 每個 batch 一個完整且獨立的 agent run |

v1 適合閱讀最小的 model-call 路徑；v2 用來理解 scheduler 如何先管理 batch，
再經 listener 進入完整 agent lifecycle。兩者都固定使用 MiniMax，且不提供
provider selector、fake mode、tools、approval 或 session UI。

## 安全界線 (Safety Boundary)

- log 以 `<UNTRUSTED_LOG_DATA>` 包住，model 被要求只把內容視為 evidence。
- 常見 authorization、bearer token、password、secret 與 API key 會先遮罩。
- 遮罩是 best effort，不保證涵蓋所有私有格式；log 仍會送到 MiniMax。
- 本 agent 沒有 tools，因此只能提出建議，不能自行套用修復。

在使用真實 production logs 前，仍應確認內容符合你的隱私與資料傳輸政策。

## 驗證 (Verification)

以下指令不會讀取真實 `~/.config`，也不會呼叫 MiniMax：

```bash
go test -race ./sample/log-agent-v2/...
go vet ./sample/log-agent-v2/...
staticcheck ./sample/log-agent-v2/...
go test ./...
```
