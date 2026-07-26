# 術語表 (Terminology)

> 領域名詞的單一定義來源。教學文件、commit message、`CLAUDE.md` 與註解一律引用此處用詞，不得同義漂移。狀態值以程式碼中的字面值為準。

## Agent Skeleton（`agent/`, `agent/spec/`, `prompt/`, `provider/`）

| 術語 (Term)         | 英文 (English)               | 定義 (Definition)                                                                                                                                                                                | 出處 (Source)                                                                     |
| ------------------- | ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------- |
| `tier`              | engagement ladder            | 應用層宣告要開哪些能力的四階單調集合：`oneshot` < `basic` < `standard` < `full`。tier 只決定 feature block 的開關預設，block 本身仍可獨立覆寫                                                    | `agent/spec/tier.go`                                                              |
| `feature block`     | (—)                          | `Config` 內的指標欄位（如 `*Safety`）：`nil` = 關閉，`&T{}` = 開啟且用預設值。層 1 opt-in 的載體                                                                                                 | `agent/spec/spec.go`                                                              |
| `variant`           | (—)                          | feature block 內的具名字串欄位（如 `Safety.Mode = "default"`）：空字串 = 該 feature 的預設實作。層 2 opt-in 的載體                                                                               | `agent/spec/spec.go`                                                              |
| `Choice`            | (—)                          | 一筆可選值（`Value` + `Label` + `Note` + `Default`），純資料、可序列化、可列舉；wizard 以此呈現設定選項                                                                                          | `agent/spec/choice.go`、`cmd/agent/wizard/wizard.go`                              |
| `Option`            | (—)                          | `type Option func(*builder) error`：DI 用的 functional option；閉包、不可列舉、只活在本 process                                                                                                  | `agent/builder_options.go`                                                        |
| `Style`             | reasoning style              | 這次跑哪個推理策略，seed `core.State.ReasoningStyle`；`reasoning.NewDecide` 依此欄位派工                                                                                                         | `agent/spec/spec.go::Reasoning`、`agent/build.go::seedState`                      |
| `Enable`            | enabled reasoning rules      | 註冊到 `reasoning.NewDecide` map 的策略清單；只跑一個是預設，需要中途切換才註冊多個                                                                                                              | `agent/spec/spec.go::Reasoning`、`agent/build.go::buildDecide`                    |
| `build pipeline`    | 8-stage build pipeline       | `agent` 的組裝順序：provider → tools → reasoning → prompt → safety → memory → output → assemble                                                                                                  | `agent/build.go::Bootstrap`                                                       |
| `wizard stage`      | 9-stage configuration flow   | wizard 的設定流程：tier → model → reasoning → tools → safety → subagents → memory → output and limits → review；其階段數不等同 8-stage build pipeline                                            | `cmd/agent/wizard/wizard.go::run`                                                 |
| `Persona`           | (—)                          | 固定 system identity 文字，`Config.Persona` 在所有 tier（含 `oneshot`）都可用；wizard 在空白時依序以 `CLAUDE.md`、`AGENTS.md` 檔名字串作預設值                                                   | `agent/spec/spec.go::Config`、`cmd/agent/wizard/helper.go::detectDefaultPersona`  |
| `Source`            | prompt source                | 進 context window 的內容貢獻者；`prompt.Builder` 依 `Order` 排序並組裝                                                                                                                           | `prompt/prompt.go`                                                                |
| `Slot`              | (—)                          | prompt 內容去向：`SLOT_SYSTEM`（seed 一次）、`SLOT_USER`（每回合）、`SLOT_REMINDER`（每回合隨 user message）                                                                                     | `prompt/prompt.go`                                                                |
| `provider registry` | provider registry            | provider 名稱到 adapter constructor 的唯一映射；`Names`、`Entries`、`Lookup` 與 `New` 供 CLI 與 agent 共用                                                                                       | `provider/registry.go`                                                            |
| `provider catalog`  | bundled model catalog        | 每個 adapter 的 `DefaultCatalog()` 所提供的靜態 `core.ModelSpec` 清單；`agent.ModelChoices` 將其轉成 wizard 可列舉的 `Choice`                                                                    | `agent/provider_catalog.go::ModelChoices`、`provider/*/models.go::DefaultCatalog` |
| `Resolve`           | environment resolution       | `registry.Options.Resolve` 依 provider entry 的環境變數優先序補齊空白的 API key 與 base URL；不負責設定檔載入或 token refresh                                                                    | `provider/registry.go::Options.Resolve`                                           |
| `LookupEnv`         | environment lookup injection | `registry.Options.LookupEnv func(string) string`：CLI 可傳 viper-backed lookup，library 預設用 `os.Getenv`                                                                                       | `provider/registry.go::Options`                                                   |
| `Once`              | one-shot facade              | `agent.Once(ctx, cfg, prompt)` 強制使用 `oneshot` tier 與 `OneShotReasoning`，仍經 `runtime.Engine` 執行；tools 與 persistence 關閉，但可注入 provider、sink 或 notifier，且不需建立 `AppConfig` | `agent/once.go::Once`、`agent/once.go::oneshot`                                   |
| `explicit wins`     | (—)                          | tier 展開後，呼叫端寫下的 block 不被覆蓋，只補其內空白的 variant 欄位                                                                                                                            | `agent/spec/tier.go`                                                              |
| `monotonic`         | tier monotonicity            | 高階 tier 開啟的 block 集合是低階 tier 的超集                                                                                                                                                    | `agent/spec/spec_test.go::TestExpandTierLadderIsMonotonic`                        |

## Memory（`memory/`, `memory/compaction/`）

| 術語 (Term)            | 英文 (English)          | 定義 (Definition)                                                                                                                                                   | 出處 (Source)                                                                         |
| ---------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `Window`               | context window          | 依 `MaxMessages` 與可選的 `MaxTokens` 保留最新訊息；token trimming 至少保留一則訊息                                                                                 | `memory/window.go::Window.Trim`                                                       |
| `CharHeuristicCounter` | heuristic token counter | 無 provider-native token counter 時的可決定性 fallback，以每個純文字 part 的 `len(text)/4 + 1` 估算                                                                 | `memory/window.go::CharHeuristicCounter.Count`                                        |
| `Compactor`            | context compactor       | 把一段 `[]core.Message` 壓縮成一則較小 `core.Message` 的介面；root `memory` package 只保留型別別名相容層                                                            | `memory/compaction/compaction.go::Compactor`、`memory/compactor.go`                   |
| `HeadlineCompactor`    | headline compactor      | 無 I/O、可決定性地擷取每個純文字 part 的第一行，串成一則 assistant summary；目前只由測試與 `memory-demo` 呼叫，尚未接入 `agent.Memory.Compaction` 的 runtime wiring | `memory/compaction/headline.go::HeadlineCompactor`、`sample/demo-memory/cmd/demos.go` |

## Core / Runtime

| 術語 (Term)       | 英文 (English)   | 定義 (Definition)                                                                                                                        | 出處 (Source)                                                             |
| ----------------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| `DecisionRule`          | (—)              | 純函式規劃 FSM，介面 `Kind() string` + `NextStep(state)`；dispatcher 與六個內建實作都位於 `reasoning/`                                    | `reasoning/decision.go`                                                   |
| `State.ReasoningStyle`  | reasoning style  | 持有策略名稱的普通 `string` 欄位；可用值由 `core.REASON_*` 字串常數列舉，不另建 named type                                                 | `core/state.go`、`core/decision.go`                                       |
| `EventSink`       | (—)              | 呈現流 port；`Output.Format=json` 自動綁 `wire.NewSink`，`text`/`tui` 由前端接管                                                         | `core/stream.go`、`agent/build.go::buildSink`                             |
| `round`           | round            | 一次 `CALL_MODEL` dispatch 及其引發的全部 tool call；使用者面的計量單位，由 `Budget.MaxRounds` 上限                                      | `runtime/loop.go::runInstruction`、`core/budget.go::Budget`               |
| `turn`            | turn             | 一次 `Decide` 迭代（`State.Turn`）；內部 runaway guard，由 `Budget.MaxTurns` 上限，不等於 model request/response 輪次                    | `runtime/loop.go::runStep`、`core/budget.go::Budget`                      |
| `tool call batch` | tool call batch  | 單一 `ModelResult.ToolCalls` 切片，即一個 round 內 model 一次要求的全部 operation；`Budget.MaxToolCalls` 限制其批量大小                  | `core/model.go::ModelResult`、`runtime/loop.go::runStep`                  |
| `settlement`      | settlement       | batch 內每個 call 在下一次 `CALL_MODEL` 前恰好對應一個 `tool_result`，無論它已執行、被 hook 擋、因 pause 未執行或被 budget skip          | `runtime/harness.go::settleSkipped`、`runtime/harness.go::settleUnrun`    |
| `continue-gate`   | continue-gate    | `ToolCall == nil` 的 `PendingApproval`：整批工具因 `MaxToolCalls` 超限而 skip 後暫停，決策是 resume 或 stop 整個 run，不是執行單一 call  | `runtime/loop.go::runStep`、`runtime/loop.go::consumeApprovedPendingCall` |
| `pause reason`    | pause reason     | run 停下但應用仍可續行的原因：`approval`（含 continue-gate）或 `round_end`；`agent.Run` 依此呼叫 `Interactive.NextRound`                   | `agent/contract.go::PauseReason`、`agent/lifecycle.go`                     |
| `Interactive`     | interactive seam | 單一互動縫 `NextRound(ctx, Pause) (Resume, error)`，統一承接 approval decision 與 follow-up input；未實作時保留 out-of-process verb 語意 | `agent/contract.go::Interactive`                                          |
| `Steer`           | steering input   | 把 user message 排入佇列，在下一次 `Decide` 前注入 conversation；可與正在執行的 Engine 並行呼叫                                          | `runtime/harness.go::Engine.Steer`                                        |
| `FollowUp`        | follow-up input  | 當 run 原本要完成時，每次取一則排隊的 user message 續跑，並重設 `think_then_act.phase`                                                   | `runtime/harness.go::Engine.FollowUp`、`runtime/loop.go::runStep`         |

## 狀態值 (Status Values)

| 狀態                         | 字面值 (Literal)        | 語意                                              | 出處                                 |
| ---------------------------- | ----------------------- | ------------------------------------------------- | ------------------------------------ |
| `RUN_STATUS_RUNNING`         | `"running"`             | run 正在執行                                      | `core/run_status.go::RunStatus`      |
| `RUN_STATUS_PAUSED_APPROVAL` | `"paused_for_approval"` | run 等待 approval 或 continue-gate 決策           | `core/run_status.go::RunStatus`      |
| `RUN_STATUS_COMPLETED`       | `"completed"`           | run 正常完成                                      | `core/run_status.go::RunStatus`      |
| `RUN_STATUS_FAILED`          | `"failed"`              | run 因錯誤失敗                                    | `core/run_status.go::RunStatus`      |
| `RUN_STATUS_ABORTED`         | `"aborted"`             | run 已中止                                        | `core/run_status.go::RunStatus`      |
| `PAUSE_APPROVAL`             | `"approval"`            | `Interactive` 收到 approval 類 pause              | `agent/contract.go::PauseReason`     |
| `PAUSE_ROUND_END`            | `"round_end"`           | `Interactive` 收到 round 完成後的 follow-up pause | `agent/contract.go::PauseReason`     |
| `APPROVAL_DECISION_APPROVE`  | `"approve"`             | 核准 tool call 或 continue-gate resume            | `core/approval.go::ApprovalDecision` |
| `APPROVAL_DECISION_REJECT`   | `"reject"`              | 拒絕 tool call 或結束 continue-gate run           | `core/approval.go::ApprovalDecision` |
| `APPROVAL_DECISION_ASK`      | `"ask"`                 | 要求更多資訊並重新排隊                            | `core/approval.go::ApprovalDecision` |
| turn budget reason           | `"turn_budget"`         | `UsedTurns` 已達 `MaxTurns`                       | `core/budget.go::Budget.Exceeded`    |
| round budget reason          | `"round_budget"`        | `UsedRounds` 已達 `MaxRounds`                     | `core/budget.go::Budget.Exceeded`    |
| token budget reason          | `"token_budget"`        | `UsedTokens` 已達 `MaxTokens`                     | `core/budget.go::Budget.Exceeded`    |
| wall-time budget reason      | `"wall_time_budget"`    | run wall time 已達 `MaxWallTime`                  | `core/budget.go::Budget.Exceeded`    |
| tool-call budget reason      | `"tool_call_budget"`    | 單一 round 的 tool call batch 超過 `MaxToolCalls` | `runtime/loop.go::runStep`           |

## 縮寫 (Abbreviations)

| 縮寫   | 全稱                 | 說明                                                                               |
| ------ | -------------------- | ---------------------------------------------------------------------------------- |
| `spec` | `agent/spec`         | 宣告層 package，僅 import `core`                                                   |
| `FSM`  | finite-state machine | `DecisionRule` 使用的純函式狀態機形式；狀態透過 `WorkingMemory` 傳遞               |
| `HITL` | human in the loop    | `PendingApproval`、`SubmitHumanDecision` 與 `Interactive` 所構成的人工作業決策流程 |
