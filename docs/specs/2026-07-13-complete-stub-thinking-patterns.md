# Plan — 補完三個 STUB ThinkingPattern (OneShot / LearnFromFailure / ChooseAgent)

## Context

`planning/` 套件宣稱提供 6 種 `ThinkingPattern`,但 3 個是 STUB(README.md / CLAUDE.md / `docs/specs/2026-07-07-planning-thinking-patterns.md` 皆以 🟡 標記):`OneShotReasoning`、`LearnFromFailure`、`ChooseAgent`。目前三者只 emit `CALL_MODEL + DONE` 或 `NOTIFY + DONE` 佔位,與 spec 宣稱的「6 種 pattern」有交付落差。

本計畫把三者補成**有意義的純函式 FSM**,對齊既有 `ThinkThenAct` / `PlanThenRun` / `DoThenReview` 的深度:phase + working memory keys + `Seed*` helper + 單元測試。方向經用戶確認為「純函式 FSM + runtime seed」,且兩處取捨已拍板(見 §2.3、§2.4)。

範圍嚴格限定在 `planning/` + `runtime/loop.go` preStep seed + `planning_test.go`;**不動** `core/`、`memory/`、`action/`(sub-agent registry 與 memory reflection 基礎設施屬另一波)。

## 既有契約(不變)

- `core.DecisionRule`(`core/thinking.go:32`):`Kind() ReasoningStyle` + `NextStep(state State) (State, []Instruction)`。**純函式,不能 inspect `Event`**。
- runtime preStep seed 在 `runtime/loop.go:384-389`,目前 ReAct-only(折 `think_then_act.pending_call` = `event.ModelResult.ToolCalls[0]`、`think_then_act.last_result` = `event.ToolResult.CallID`)。
- end_turn short-circuit(`loop.go:377-383`):`len(ToolCalls)==0` → COMPLETED,跳過 Decide。**所有 pattern 共用,不動**。
- 既有 helpers(`planning/helpers.go`):`scratchString/Int/Call/Blueprint/Set`、`callModelFromMessages(state)`、`callToolInstruction(call)`、`doneInstruction()`、`startsWithPassed(s)`(認 `"OK:"` 首三字)、`newID()`。
- 既有 `Seed*` 風格:`SeedDispatch(s, call)`、`SeedBlueprint(s, blueprint)`、`SeedReviewPassed/Failed(s, text)` — 傳 `*core.State`,單一職責。
- 命名慣例:scratch key `<pattern_name>.<field>` 點號分隔;常數 `SCREAMING_SNAKE_CASE`;phase 值小寫。

## 2. 三個 pattern 的最終 FSM

### 2.1 OneShotReasoning — `think → done` 雙相

學術出處 Wei 2022 CoT。語意:單步 LLM call 然後結束,不迭代。

**為何需要 phase FSM**(非沿用舊 STUB 的 `[CALL_MODEL, DONE]`):舊 STUB 每次 `NextStep` 都吐兩條 effect,違反純函式不變式(`core/thinking.go:35`「給定 state 回同 result」)— 若 runtime 因 retry / WAL replay 重叫 `NextStep`,會無腦再吐一次 `CALL_MODEL`。補完後 `think` 只走一次就推到 `done`,後續一律 DONE。

Scratch keys(export 常數):

| 常數 | 值 | 用途 |
|------|----|------|
| `ONE_SHOT_PHASE` | `"one_shot.phase"` | FSM 階段;預設 `ONE_SHOT_THINK` |
| `ONE_SHOT_THINK` | `"think"` | 起步 phase |
| `ONE_SHOT_DONE` | `"done"` | 終態 phase |

FSM:

| Phase | Emit | 推到 |
|-------|------|------|
| `think`(預設) | `CALL_MODEL` 帶 `state.Messages` | `done` |
| `done` / `default` | `DONE`(fail-closed) | (terminal) |

不 dispatch tool call,不需 HITL resume 支援。

`Seed*`:`SeedOneShotThinking(s *core.State)`、`SeedOneShotDone(s *core.State)`(對稱性提供,各一行 `scratchSet`)。

**runtime preStep seed:不需新分支**(走 default no-op)。

### 2.2 LearnFromFailure — `act → reflect → retry → done` 四相

學術出處 Shinn 2023 Reflexion。語意:失敗 → 反思 → 帶累積反思 retry。

**失敗判定(用戶拍板):LLM critique `OK:` prefix**,複用既有 `startsWithPassed(s)` — 不靠 `ToolResult.OK`。這與 `DoThenReview` 的 `RUN_THEN_REVIEW_NOTE` 機制一致;**LFF 的差異是 reflection 跨迭代累積進 `LFF_REFLECTIONS` slice**(verbal reinforcement 載體),DoThenReview 不累積。reflection 文字從 `state.Messages` 末則 assistant 純文字取(新 helper `latestAssistantText`)。

Scratch keys:

| 常數 | 值 | 用途 |
|------|----|------|
| `LEARN_FROM_FAILURE_PHASE` | `"learn_from_failure.phase"` | 預設 `LFF_ACT` |
| `LEARN_FROM_FAILURE_REFLECTIONS` | `"learn_from_failure.reflections"` | `[]string` append-only |
| `LEARN_FROM_FAILURE_ITERATION` | `"learn_from_failure.iteration"` | int,對齊 `RUN_THEN_REVIEW_ITERATION` |
| `LFF_ACT` / `LFF_REFLECT` / `LFF_RETRY` / `LFF_DONE` | `"act"`/`"reflect"`/`"retry"`/`"done"` | phase 值 |

(常數全名對齊 `THINK_THEN_ACT_PHASE` 風格;phase 值常數用 `LFF_` 縮寫,因常被呼叫於 switch case,短名可讀性佳。Seed helper 同理用 `LFF` 前綴。)

FSM:

| Phase | Emit | 推到 |
|-------|------|------|
| `act`(預設) | `CALL_MODEL` 帶 `state.Messages` | `reflect` |
| `reflect` | `CALL_MODEL`(請 LLM 產反思)→ 下一輪進 `retry` 時,`latestAssistantText(state)` 取反思 append 進 `LFF_REFLECTIONS` | `retry` |
| `retry`(進入時取 `latestAssistantText`):文字 `OK:` 開頭 | `DONE` | (terminal) |
| `retry`:非 `OK:` 開頭 | `CALL_MODEL` 帶 `state.Messages`(含累積反思) | `LFF_ITERATION++`,留在 `retry` |
| `retry`:無 assistant 文字(無法判定) | `DONE`(保守結束) | (terminal) |
| `done` / `default` | `DONE`(fail-closed) | (terminal) |

iteration hard cap 由 `state.Budget` 控管(對齊 DoThenReview,pattern 不擋)。

`Seed*`:`SeedLFFAct(s *core.State)`、`SeedLFFReflection(s *core.State, text string)`、`SeedLFFCritiquePassed(s *core.State, text string)`(在 `reflect` phase 寫一則 `OK:` 開頭的 assistant 文字進 `state.Messages` 末尾,模擬 LLM 給通過 critique,讓 `retry` 走 DONE 分支)、`SeedLFFCritiqueFailed(s *core.State, text string)`(寫非 `OK:` 文字,讓 `retry` 走 retry 分支)。

**runtime preStep seed:不需新分支**。LFF 靠 `state.Messages` 流轉 critique,與 DoThenReview 一樣不依賴 runtime 折 `ToolResult`。**這是採用「LLM critique」方案的最大好處 — runtime preStep seed 完全不動,ReAct 維持原樣。**

### 2.3 ChooseAgent — `select → delegate → done` 三相(用戶拍板:delegate inline system msg)

學術出處 Router/Orchestrator。語意:選 agent → delegate。無 sub-agent registry,做到「FSM 完整 + 選 agent 寫進 scratch + 帶 system prompt CALL_MODEL 一次」的範圍內最佳近似。

Scratch keys:

| 常數 | 值 | 用途 |
|------|----|------|
| `CHOOSE_AGENT_PHASE` | `"choose_agent.phase"` | 預設 `CA_SELECT` |
| `CHOOSE_AGENT_AGENT_LIST` | `"choose_agent.agent_list"` | `[]string`,test/fixture seed-only |
| `CHOOSE_AGENT_CHOSEN` | `"choose_agent.chosen_agent"` | string,選中的 agent 名 |
| `CA_SELECT` / `CA_DELEGATE` / `CA_DONE` | `"select"`/`"delegate"`/`"done"` | phase 值 |

FSM:

| Phase | 條件 | Emit | 推到 |
|-------|------|------|------|
| `select`(預設) | `CHOOSE_AGENT_AGENT_LIST` 非空 | 取 `[0]` 寫 `CHOOSE_AGENT_CHOSEN`;`NOTIFY`(`info`,`"router chose agent: <name>"`) | `delegate` |
| `select` | list 空 | `CALL_MODEL` 帶 `state.Messages`(保留 LLM-routing hook) | `delegate` |
| `delegate` | 已選 agent | `CALL_MODEL`,**inline 建構**:`Messages` 開頭塞 system message `{Role: ROLE_SYSTEM, Parts: [{PART_KIND_PLAIN_TEXT, "You are agent <chosen>. Address the user task in that agent's voice."}]}` 後接原 `state.Messages`。用 `core.Instruction{CallModel: &CallModelInstruction{RequestID: newID(), Messages: msgs}}` 直接建,不透過 `callModelFromMessages`。 | `done` |
| `done` / `default` | | `DONE`(fail-closed) | (terminal) |

**inline system msg(用戶拍板)**:這是第一個繞過 `callModelFromMessages` 的 pattern,接受一次性偏差(不擴 helper 工廠)。

`Seed*`:`SeedAgents(s *core.State, agents []string)`。

**runtime preStep seed:不需新分支**(seed 完全來自 test/fixture,runtime 不知 agent registry)。

### 2.4 runtime preStep seed — 最終結論:完全不動

採用「LFF 走 LLM critique」方案後,**三個補完 pattern 都不需 runtime preStep seed**:
- OneShot / ChooseAgent:不靠 event 折疊推進。
- LearnFromFailure:靠 `state.Messages` 流轉 critique(同 DoThenReview)。

`runtime/loop.go:384-389` 的 ReAct seed **維持原樣**,不改成 switch。這比 Plan agent 原方案(折 `ToolResult.OK` 進 LFF key)更小侵入,且讓 LFF 與 DoThenReview 的 critique 機制一致。**`runtime/loop.go` 因此不在改動清單內。**

## 3. helpers.go 新增

| 函式 | 用途 |
|------|------|
| `scratchStringSlice(state, key) []string` | 型別斷言讀 `[]string`,缺回 nil(給 `LFF_REFLECTIONS` 用) |
| `scratchAppendString(s *State, key, val string)` | 讀舊 slice → append → 寫回(給 `LFF_REFLECTIONS` 累積用) |
| `latestAssistantText(state core.State) string` | 從 `state.Messages` 尾反掃找 `ROLE_ASSISTANT` + `PART_KIND_PLAIN_TEXT`,回 text,無則 `""` |

讀 `state.Messages` 不違反純函式契約(state 的一部分,等同讀 scratch)。

## 4. 測試 case(`planning_test.go` 既有風格:每 phase 一 case,seed-based,不靠 fake model)

### OneShotReasoning(3 case)
- `TestOneShotThinkEmitsCallModel` — 空 state → `CALL_MODEL`;scratch[PHASE]==`"done"`
- `TestOneShotDoneEmitsDone` — `SeedOneShotDone` → `DONE`
- `TestOneShotUnknownPhaseEmitsDone` — scratch[PHASE]=`"garbage"` → `DONE`(fail-closed)

### LearnFromFailure(5 case)
- `TestLFFActEmitsCallModel` — 空 state → `CALL_MODEL`;scratch[PHASE]==`"reflect"`
- `TestLFFReflectEmitsCallModel` — phase=`"reflect"` → `CALL_MODEL`;scratch[PHASE]==`"retry"`
- `TestLFFRetryPassedEmitsDone` — phase=`"retry"`;`SeedLFFCritiquePassed` → `DONE`
- `TestLFFRetryFailedEmitsCallModel` — phase=`"retry"`;`SeedLFFCritiqueFailed` → `CALL_MODEL`;scratch[ITERATION]==1
- `TestLFFReflectionAccumulates` — scratch[REFLECTIONS]=["old"];`SeedLFFReflection(s,"new")` → scratch[REFLECTIONS]==["old","new"]

### ChooseAgent(4 case)
- `TestChooseAgentSelectWithListEmitsNotify` — `SeedAgents(s,["a","b"])` → `NOTIFY`(info);scratch[CHOSEN]==`"a"`;scratch[PHASE]==`"delegate"`
- `TestChooseAgentSelectEmptyEmitsCallModel` — 空 state → `CALL_MODEL`(hook);scratch[PHASE]==`"delegate"`
- `TestChooseAgentDelegateEmitsCallModelWithSystemMsg` — phase=`"delegate"`;scratch[CHOSEN]=`"a"` → `CALL_MODEL`;`Messages[0].Role==ROLE_SYSTEM`;text 含 `"agent a"`
- `TestChooseAgentDoneEmitsDone` — phase=`"done"` → `DONE`

### helpers(2 case)
- `TestLatestAssistantTextFindsLast` — Messages=[user "u",assistant "a1",user "u2",assistant "a2"] → `"a2"`
- `TestLatestAssistantTextEmpty` — Messages=[user "u"] → `""`

## 5. `TestStubRulesDoNotPanic` 調整

現況(`planning_test.go:99-126`)斷言三 STUB「必 emit DONE」。補完後三 pattern 是 phase FSM,起步 phase 不吐 DONE(如 LFF 起步吐 `CALL_MODEL`)。採**替代方案(最小侵入)**:把 `TestStubRulesDoNotPanic` 改名 `TestRulesReachDone`,改成驗證三個補完 pattern「能抵達 DONE 終態」— 各自用 `Seed*` 推進 phase 直到最後一次 `NextStep` 吐 `DONE`。3 個完整 pattern(ReAct/PlanThenRun/DoThenReview)繼續由各自既有 FSM 測試覆蓋,不納入此 table。

## 6. 檔案改動清單

| 檔案 | 改動 |
|------|------|
| `planning/helpers.go` | 新增 `scratchStringSlice` / `scratchAppendString` / `latestAssistantText` |
| `planning/one_shot.go` | 加 `ONE_SHOT_*` 常數;重寫 `NextStep` 為 `think→done` FSM;新增 `SeedOneShotThinking` / `SeedOneShotDone` |
| `planning/learn_from_failure.go` | 加 `LEARN_FROM_FAILURE_*` / `LFF_*` 常數;重寫 `NextStep` 為 `act→reflect→retry→done` FSM;新增 `SeedLFFAct` / `SeedLFFReflection` / `SeedLFFCritiquePassed` / `SeedLFFCritiqueFailed` |
| `planning/choose_agent.go` | 加 `CHOOSE_AGENT_*` / `CA_*` 常數;重寫 `NextStep` 為 `select→delegate→done` FSM;`delegate` inline 建 system-msg `CALL_MODEL`;新增 `SeedAgents` |
| `planning/planning_test.go` | 改寫 `TestStubRulesDoNotPanic` → `TestRulesReachDone`;新增 14 個 case(3+5+4+2) |
| `runtime/loop.go` | **不動**(LFF 走 critique 方案,runtime seed 維持 ReAct-only) |

**不動**:`core/`、`memory/`、`action/`、`consumeApprovedPendingCall`、runtime preStep seed block。

## 7. 驗收(Verification)

```bash
cd /Users/bytedance/projects/agentSDK
go test ./planning/... -count=1 -timeout=30s   # 新增 14 case + 改寫的 TestRulesReachDone 全綠
go test ./... -count=1 -timeout=120s             # root module 全綠,確認 runtime 未回歸
go vet ./planning/... ./runtime/...
```

語意驗收:
- 三個 pattern 從 `README.md` / `CLAUDE.md` / spec 的 🟡 STUB 標記可移除(文件同步另列 TODO,本計畫聚焦程式碼)。
- `TestRulesReachDone` 證明三者能抵達 DONE 終態(不死循環)。
- LFF 的 reflection 累積跨迭代保留(`scratchAppendString` 驗證)。
- ChooseAgent 的 system message 注入正確(role + text)。

## 8. 實作順序

1. `planning/helpers.go` — 加 3 個 helper(配 helpers 測試)。
2. `planning/one_shot.go` — FSM + 2 Seed + 3 case。
3. `planning/learn_from_failure.go` — FSM + 4 Seed + 5 case。
4. `planning/choose_agent.go` — FSM + 1 Seed + inline system msg + 4 case。
5. `planning/planning_test.go` — 改寫 `TestStubRulesDoNotPanic` → `TestRulesReachDone`。
6. `go test ./planning/... -count=1` + `go test ./... -count=1` 全綠。
