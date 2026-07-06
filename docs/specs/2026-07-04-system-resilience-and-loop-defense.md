# M2 Spec — 系統韌性 + 循環防禦

## 目標

在 M1 的純狀態機基礎上,加入:
1. **持久化** — State snapshot + WAL append-only,支援 crash-recovery
2. **Middleware 鏈** — retry / timeout / budget / loopguard 守護每次 effect dispatch
3. **Dedupe** — sample 層級的指紋 + cooldown 抑制重複告警
4. **Resume** — sample CLI 從上次 WAL turn 續跑

## 設計原則

- **核心純粹性**:`core/` 不引入 M2 的 I/O 概念;檔案格式 (JSON snapshot / JSONL WAL) 由 `memory/filestore` 負責
- **Middleware 為可選注入**:runtime 預設組合 `retry → timeout → budget → loopguard`,呼叫者可覆寫 (`loop.Middleware = ...`)
- **WAL 記錄 Input 而非 Effect**:replay 不需重發 LLM 呼叫;WAL 已含原來的 ModelResult / ToolResult
- **Dedupe 為 Source 層級**:包裝既有 Source,不改變下游契約

## 套件結構

| 套件 | 角色 | 關鍵型別 |
|------|------|---------|
| `memory/` | 滾動窗口 / 壓縮介面 | `Window`, `CharHeuristicCounter`, `Compactor`, `HeadlineCompactor` |
| `memory/checkpoint/` | 持久化組合根 | `Checkpointer{Checkpoint, Recover}` |
| `memory/filestore/` | 檔案 I/O 實作 | `FileStateStore`, `FileWAL` |
| `middleware/` | 鏈組合 | `Middleware`, `Next`, `Chain` |
| `middleware/harness/` | 政策類 middleware | `Retry`, `Budget`, `Timeout` |
| `middleware/loopguard/` | 循環防禦 | `Config`, fingerprint + scratch state |
| `runtime/` | 整合點 | `Loop.Middleware`, `DefaultMiddleware`, `Loop.Resume`, `IsBudgetExceeded` |
| `sample/logdoctor/core/` | 領域 dedupe | `Dedupe{Inner, RuleID, Cooldown}` |

## 關鍵介面 (Key Interfaces)

### Middleware

```go
type Next func(ctx context.Context, state core.State, eff core.Effect) (core.State, *core.Input, bool, error)
type Middleware func(Next) Next
func Chain(mws ...Middleware) Middleware
```

`Next` 回傳 `(state, *input, terminal, error)`。state 是 mutate 後的版本,需由下一層傳回呼叫者。

### Checkpointer

```go
type Checkpointer struct {
    Store core.StateStore
    WAL   core.WAL
}
func (c *Checkpointer) Checkpoint(ctx, s State) error
func (c *Checkpointer) Recover(ctx, runID) (RecoverResult, error)
```

`Recover` 回傳 `(State, []Input)` — caller 不重發 LLM,因為 WAL 已含 `ModelResult`。

### Dedupe

```go
type Dedupe struct {
    Inner    sdkcore.Source
    RuleID   string
    Cooldown time.Duration
}
```

指紋 = `sha1(ruleId + "|" + payload)[:12]`,命中後 cooldown 內不重發。

## 行為保證 (Behavioral Guarantees)

### 1. Budget 守衛

當 `state.Budget.UsedTurns >= state.Budget.MaxTurns` 時,Budget middleware 直接回傳 `*BudgetExceededError` (typed error),runtime 標記 `Status=FAILED`。呼叫者用 `runtime.IsBudgetExceeded(err)` 判斷。

### 2. Retry 守衛

只有實作 `RetryableError` interface 的 error (`Retryable() bool` 為 true) 才會被重試。預設 3 次,指數 backoff 100ms → 5s。Retry 在 middleware 鏈最外層,涵蓋整個下游 dispatch。

### 3. Timeout 守衛

每個 effect 包 `context.WithTimeout(ctx, PerEffect)`。`cctx.Err() == DeadlineExceeded` 時 middleware 改寫 err 為 deadline,即使 inner 已返回 nil。

### 4. Loopguard 守衛

- 5 次連續 CALL_TOOL (同 `tool+args 指紋`) → 第 5 次被改寫為 `REQUEST_APPROVAL{Reason: "loop_detected"}`,`Triggered=true` 後不再觸發
- 指紋排除 `offset / cursor / page / since / tail_offset` 預設 volatile keys
- State 透過 `scratch["loopguard.state"]` 持久化,跨 iteration 累積
- 任何非 CALL_TOOL effect (尤其 CALL_MODEL) 會 reset Repeats 計數

### 5. Crash Recovery

- `Checkpointer.Recover` 後,caller 拿到 (State, []Input) 完全等價於 crash 前的 state
- `FakeProvider.CallCount` 在 `Recover` 期間不增加 (replay 帶入的 ModelResult 不重發)
- `Loop.Resume` 走 `Load + Replay + runWithInput*` 路徑,串接每個 replay 進來的 Input 直到完成

## 範例 (Examples)

### M2 預設 Middleware 鏈

```go
loop := runtime.NewLoop(step, model, tools)
// 等同於 runtime.DefaultMiddleware(): retry → timeout → budget → loopguard
loop.Run(ctx, state)
```

### 自訂鏈 (略過 loopguard)

```go
loop.Middleware = middleware.Chain(
    harness.Retry(harness.RetryConfig{N: 5}),
    harness.Budget(),
)
```

### Resume CLI

```bash
# 第一次跑
LOGDOCTOR_DATA=./data logdoctor --fake run --once --fixture app.log

# kill 後 resume
LOGDOCTOR_DATA=./data logdoctor --fake resume --run-id run-1783...
```

## 測試驗證 (Verification)

| 計畫驗收項 | 測試位置 |
|------------|---------|
| Budget 到頂即停 | `runtime/loop_test.go::TestBudgetExceededStopsLoop`<br>`runtime/middleware_integration_test.go::TestRuntimeBudgetExceededExitsRun` |
| Retry N 次後 surface | `middleware/middleware_test.go::TestRetryRecoversTransientError`<br>`TestRetrySurfacesNonRetryable` |
| FileStateStore round-trip | `memory/memory_test.go::TestFileStateStoreRoundTrip` |
| Checkpointer.Recover JSON 等價原 State | `runtime/crash_recovery_test.go::TestCrashRecoveryFullCycle` |
| Recover 期間不重發 LLM 呼叫 | `memory/memory_test.go::TestRecoverDoesNotReissueModelCalls` |
| Loopguard 第 5 次連續觸發 | `middleware/middleware_test.go::TestLoopguardRewritesToApproval`<br>`TestLoopguardStripsVolatileArgs` |
| 整體鏈 (retry→budget→loopguard) | `runtime/crash_recovery_test.go::TestChainComposesOverRetryThroughLoopguard` |
| Dedupe 同指紋抑制 | `sample/logdoctor/core/dedupe_test.go::TestDedupeDropsRepeatsWithinCooldown` |
| CLI resume 從 WAL 續跑 | 手動驗證 (`logdoctor run` → `kill -9` → `logdoctor resume`) |

## 已知限制 (Known Limitations)

1. **Timeout 不會 preempt 阻塞型 inner**:`time.Sleep` 不讀 ctx;M3 若需要嚴格 deadline,inner 須改用 `select { case <-ctx.Done(): }` 形式
2. **Loopguard fingerprint 排除預設 volatile keys**;若特定 tool 用 `n` 當 cursor,需自訂 `Config.VolatileKeys`
3. **Dedupe 沒去重 windows**:同一 percept 在 cooldown 內完全抑制,即使中間有其他事件
4. **Recover 的 State 比對用 `assert.JSONEq`**:`reflect.DeepEqual` 對 `*ToolResultChunk` pointer 敏感 (新 unmarshal 後 pointer 不同),JSON 比較保證語意等價

## M3 銜接 (M3 Hooks)

- **Tracing**:Middleware 鏈最外層加 `tracing.SpanMW`,runtime 介面不變
- **Sandbox**:在 loopguard 之後,approval gate 之前;`SandboxMW` 可 DENY CALL_TOOL
- **Schema 反射**:`action/schema.go` 引入 `invopop/jsonschema`,`TypedTool.Schema()` 從 struct tags 自動產生
- **MCP**:`mcp.Client` 實作 `action.ToolSource`,M2 已預留介面

## 對應原始 plan 段落

本 spec 對應 `plans/plan-only-and-plan-breezy-pike.md` 的 M2 區段。Plan 文件保留作為歷史決策紀錄,本 spec 為實作當下真實狀態。
