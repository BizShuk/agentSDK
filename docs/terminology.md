# 術語表 (Terminology)

> 領域名詞的單一定義來源。教學文件、commit message、`CLAUDE.md` 與註解一律引用此處用詞，不得同義漂移。狀態值以程式碼中的字面值為準。

## Agent Skeleton（`agent/`, `agent/spec/`, `prompt/`, `provider/registry/`）

| 術語 (Term)      | 英文 (English)          | 定義 (Definition)                                                                                                                               | 出處 (Source)                                              |
| ---------------- | ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| `tier`           | engagement ladder       | 應用層`宣告`要開哪些能力的四階單調集合：`oneshot` < `basic` < `standard` < `full`。tier 只決定 feature block 的開關預設，block 本身仍可獨立覆寫 | `agent/spec/tier.go`                                       |
| `feature block`  | (—)                     | `Config` 內的指標欄位（如 `*Safety`）：`nil` = 關閉，`&T{}` = 開啟且用預設值。層 1 opt-in 的載體                                                | `agent/spec/spec.go`                                       |
| `variant`        | (—)                     | feature block 內的具名字串欄位（如 `Safety.Mode = "default"`）：空字串 = 該 feature 的預設實作。層 2 opt-in 的載體                              | `agent/spec/spec.go`                                       |
| `Choice`         | (—)                     | 一筆可選值（`Value` + `Label` + `Note` + `Default`），純資料、可序列化、可列舉；wizard 必須能先枚舉才能套用                                     | `agent/spec/choice.go`                                     |
| `Option`         | (—)                     | `type Option func(*builder) error`：DI 用的 functional option；閉包、不可列舉、只活在本 process                                                 | `agent/options.go`                                         |
| `Style`          | reasoning style         | 這次跑哪個推理策略，seed `core.State.ReasoningStyle`；`core.NewDecide` 依此欄位派工                                                             | `agent/spec/spec.go::Reasoning`                            |
| `Enable`         | enabled reasoning rules | 註冊到 `core.NewDecide` map 的策略清單；只跑一個是預設，需要中途切換才註冊多個                                                                  | `agent/spec/spec.go::Reasoning`                            |
| `Build pipeline` | 8-stage build pipeline  | `agent` 的組裝順序：provider → tools → reasoning → prompt → safety → memory → output → assemble。每階段的不變式由 build.go doc comment 守護     | `agent/build.go::Bootstrap`                                |
| `Persona`        | (—)                     | 固定身分文字，`Config.Persona` 在所有 tier（含 `oneshot`）可用；對應 `cmd/provider.go` 的 `--system` 旗標                                       | `agent/spec/spec.go`                                       |
| `Source`         | prompt source           | 進 context window 的內容貢獻者（persona / contextfile / skill / env / reminder）；`prompt.Builder` 組裝、`Order` 排序                           | `prompt/prompt.go`                                         |
| `Slot`           | (—)                     | prompt 內容去向：`SLOT_SYSTEM`（seed 一次）、`SLOT_USER`（每回合）、`SLOT_REMINDER`（每回合隨 user message）                                    | `prompt/prompt.go`                                         |
| `Resolver`       | credential resolver     | 環境變數 + 設定檔 → 過期自動 refresh。`registry.Options.Resolve` 是公開版，供 preflight 與 wizard 預覽                                          | `provider/registry/registry.go`                            |
| `LookupEnv`      | (—)                     | `registry.Options.LookupEnv func(string) string`：CLI 傳 viper-backed lookup（`.env` 參與），library 用 `os.Getenv`                             | `provider/registry/registry.go`                            |
| `Once`           | (—)                     | `agent.Once(ctx, cfg, prompt)` facade：內部走 `one_shot` rule + 全 nil port，不繞過 Engine；目的是讓最低配的呼叫不用建 `AppConfig`              | `agent/once.go`                                            |
| `explicit wins`  | (—)                     | tier 展開後，呼叫端寫下的 block 不被覆蓋，只補其內空白的 variant 欄位                                                                           | `agent/spec/tier.go`                                       |
| `monotonic`      | tier monotonicity       | 高階 tier 開啟的 block 集合是高階 tier 的超集；低階有的高階必有                                                                                 | `agent/spec/spec_test.go::TestExpandTierLadderIsMonotonic` |

## Core / Runtime（既有，這裡只列出 skeleton 互動過的）

| 術語 (Term)      | 英文 (English) | 定義 (Definition)                                                                                       | 出處 (Source)      |
| ---------------- | -------------- | ------------------------------------------------------------------------------------------------------- | ------------------ |
| `DecisionRule`   | (—)            | 純函式規劃 FSM，介面 `Kind() ReasoningStyle` + `NextStep(state)`；本 repo 六個內建在 `planning/`        | `core/thinking.go` |
| `ReasoningStyle` | (—)            | 策略列舉常數（`REASON_REACT` 等），定義在 `core` 而非 `planning`，讓宣告層能不 import `planning` 而枚舉 | `core/thinking.go` |
| `EventSink`      | (—)            | 呈現流 port；`Output.Format=json` 自動綁 `wire.NewSink`，`text`/`tui` 由前端自己接                      | `core/stream.go`   |
| `round`          | round          | 一次 `CALL_MODEL` dispatch 及其引發的全部 tool call；使用者面的計量單位，由 `Budget.MaxRounds` 上限。`ReAct` 一個 round 約燒 3 個 `turn` | `runtime/loop.go`、`core/state.go::Budget` |
| `turn`           | turn           | 一次 `Decide` 迭代（`State.Turn`）；內部 runaway guard，由 `Budget.MaxTurns` 上限。不等於 request/response 輪次 | `runtime/loop.go`、`core/state.go::Budget` |
| `tool call batch`| tool call batch| 單一 `ModelResult.ToolCalls` 切片，一個 round 內的全部 operation；由 `Budget.MaxToolCalls` 限批量大小 | `core/input.go::ModelResult` |
| `settlement`     | settlement     | batch 內每個 call 恰好對應一個 `tool_result` message，無論它執行、被 hook 擋、被 approval 暫停或被 budget skip；違反則下次 `CALL_MODEL` transcript 非法 | `runtime/harness.go::settleSkipped` |
| `continue-gate`  | continue-gate  | `ToolCall == nil` 的 `PendingApproval`：整批工具因 `MaxToolCalls` 被 skip 後暫停，問的是「resume 整個 run」而非「跑這一個 call」 | `runtime/loop.go`、`runtime/loop.go::consumeApprovedPendingCall` |
| `pause reason`   | pause reason   | run 停下但非終局的原因：`PAUSE_APPROVAL`（含 continue-gate）/ `PAUSE_ROUND_END`；`app.Run` 據此問 `Interactive.NextRound` | `app/agent.go::PauseReason` |
| `Interactive`    | (—)            | 單一互動縫 `NextRound(ctx, Pause) (Resume, error)`：approval decision 與 follow-up input 同一個方法；不實作 = 退回外部 verb 語意 | `app/agent.go::Interactive` |

## 縮寫 (Abbreviations)

| 縮寫       | 全稱            | 說明                                                                               |
| ---------- | --------------- | ---------------------------------------------------------------------------------- |
| `spec`     | `agent/spec`    | 宣告層 package，僅 import core                                                     |
| `resolver` | (—)             | 環境變數 / 設定檔 → credential 的解析流程總稱                                      |
| `mode`     | permission mode | `default` / `acceptEdits` / `plan` / `bypassPermissions` 四種 `permission.Mode` 值 |
