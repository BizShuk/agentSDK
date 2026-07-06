# M1 Spec — 核心範式 + Sample 骨架

## 目標

建立 `agentsdk` 的最小可行骨架:
1. **核心狀態機**:`core/` 純 stdlib,實作 State / Input / Effect / Step + 7 個 DI 介面
2. **6 種 ThinkingPattern**:3 個實作 (ReAct / Planner-Executor / Executor-Critic) + 3 個 STUB
3. **Action Tooling**:`TypedTool[TArgs, TOut]` 泛型 + 記憶體 Registry
4. **Runtime Shell**:`Loop.Run` 直接 dispatch,無 middleware (M2 補上)
5. **Sample logdoctor**:cobra CLI + Listener + ReadLogTail / Notify tools,無 provider/無 middleware/無 dedupe

## 設計原則

- **核心純粹性**:`core/` 零 vendor 依賴 (連 gosdk 都不 import),可獨立發佈
- **純函式 Step**:`Decide(state) → (state, []Effect)` 不讀 Input、不呼叫 I/O;runtime 把 Input 資訊壓成 scratch 再餵入 (例如 `react.last_call_id` 從 `ModelResult.ToolCalls[0]` 寫入)
- **Tagged union Effect**:Go 沒有 sum type,用 Kind discriminator + 7 個 optional pointer 表達;JSON round-trip 靠 `omitempty`
- **Notifier 結構性相容**:`core.Notifier` 方法集與 `gosdk/notify.Notifier` 完全相同,可直接傳入不需 adapter
- **命名衝突**:`agentsdk/core` 與 `sample/logdoctor/core` 屬不同 module path,編譯安全;sample 端以 `sdkcore` / `domain` 別名區分

## 套件結構

| 套件 | 角色 | 關鍵型別 |
|------|------|---------|
| `core/` | 純狀態機 | `State`, `Input`, `Effect`, `Step`, `ModelProvider`, `StateStore`, `WAL`, `ToolRegistry`, `Notifier` |
| `perception/` | 輸入側 | `Source`, `Multi`, `Normalizer` |
| `planning/` | 6 thinking patterns | `ReAct`, `PlannerExecutor`, `ExecutorCritic`, 3 STUB |
| `action/` | 輸出側 | `TypedTool[TArgs,TOut]`, `Registry`, `ToolSource` 介面 |
| `runtime/` | Shell | `Loop{Step, Model, Tools, ...}`, `Run`, `Resume`, `SubmitApproval` |
| `internal/testutil/` | 測試 helper | `FakeProvider`, `MemStore`, `MemWAL`, `CapturingNotifier` |
| `sample/logdoctor/` | 驗證 sample | `cmd/{root, run}.go`, `core/listener.go`, `tool/{read_log_tail, notify}.go`, `internal/fake/fake.go` |

## 關鍵介面

```go
// 純狀態機
type AutonomyLevel int  // L0-L4
type State struct { RunID, Turn, Autonomy, ThinkingKind, Messages,
                     Scratch, PendingApprovals, Budget, Status, UpdatedAt }
type Budget struct { MaxTurns, UsedTurns, MaxTokens, UsedTokens,
                     MaxWallTime, StartedAt }
type Input struct { Kind InputKind, Percept?, ModelResult?, ToolResult?,
                    ApprovalDecision?, Seq, ReceivedAt }
type Effect struct { Kind EffectKind, CallModel?, CallTool?,
                      RequestApproval?, Notify?, Checkpoint?, Emit? }

type ThinkingPattern interface {
    Kind() ThinkingKind
    Decide(state State) (State, []Effect)  // 純函式,no I/O
}

type Step func(state State, input Input) (State, []Effect)
func NewStep(patterns map[ThinkingKind]ThinkingPattern) Step

// DI ports
type ModelProvider interface { Generate / Stream / CountTokens }
type Notifier interface { Notify(ctx, msg) error }  // 與 gosdk 完全相同

// Action
type Tool interface { Name / Description / Schema / Risk / Call }
type TypedTool[TArgs, TOut any] struct { ... }    // 泛型 wrapper

// Perception
type Source interface { Percepts(ctx) <-chan Percept }
```

## 行為保證

- **End-turn short-circuit**:runtime 收到 `ModelResult.StopReason=end_turn` 且 `ToolCalls` 為空時,直接 COMPLETED;跳過 Step,避免 ReAct 在 act phase 對 stale scratch 呼叫 CALL_TOOL
- **scratch 作為 pattern/middleware 通訊介面**:runtime 在 Step 前 pre-populate (`react.last_call_id`),pattern 透過 Decide 讀 scratch
- **State 深拷貝**:`State.Clone()` 深拷貝 Messages、PendingApprovals、Messages 深拷貝 Chunks,但 Scratch 是 shallow (視為 opaque blob)
- **JSON round-trip 等價**:所有 exported 欄位都有 json tag,State → JSON → State 不丟資訊

## 範例

### E2E JSONL 流程 (FakeProvider)

```
$ logdoctor --fake run --once --fixture error.log
{"type":"effect","kind":"call_model",...}      ← 第一次思考
{"type":"effect","kind":"call_tool",...}       ← read_log_tail n=5
{"type":"effect","kind":"call_model",...}      ← 觀察結果
{"type":"effect","kind":"call_tool",...}       ← notify
{"type":"effect","kind":"call_model",...}      ← 最終回應
{"type":"effect","kind":"done"}                ← end_turn
```

## 測試驗證

| 驗收項 | 測試位置 |
|--------|---------|
| Budget.Exceeded 各種條件 | `core/state_test.go::TestBudgetExceeded` |
| State.Clone 深拷貝 | `core/state_test.go::TestStateClone` |
| State JSON round-trip | `core/state_test.go::TestStateJSONRoundTrip` |
| Step 依 ThinkingKind dispatch | `core/step_test.go::TestNewStepDispatchesByKind` |
| ReAct think → act → observe 轉移 | `planning/planning_test.go` (3 tests) |
| PlannerExecutor blueprint 派發 | `planning/planning_test.go` (2 tests) |
| ExecutorCritic OK/Reject | `planning/planning_test.go` (2 tests) |
| 6 STUB pattern 不 panic | `planning/planning_test.go::TestStubPatternsDoNotPanic` |
| TypedTool unmarshal/wrap | `action/action_test.go` (5 tests) |
| Registry Get/Call | `action/action_test.go` (3 tests) |
| Loop 與 FakeProvider 整合 | `runtime/loop_test.go` (7 tests) |
| Store + WAL round-trip | `runtime/loop_test.go::TestStoreAndWAL` |
| Listener 讀檔一次性 | `sample/logdoctor/core/listener_test.go` |
| ReadLogTail / Notify tool | `sample/logdoctor/tool/tool_test.go` |

## M2 銜接

M2 加入:
- `memory/{window, compactor, checkpoint/filestore}/` — 持久化
- `middleware/{harness, loopguard}/` — 守衛鏈
- `runtime.Loop.Middleware` 注入點
- `sample/logdoctor/core/dedupe.go` — 指紋抑制

M1 的契約與介面不變,M2 從外部擴充。

## 對應原始 plan

本 spec 對應 `plans/plan-only-and-plan-breezy-pike.md` 的 M1 區段。Plan 文件保留作為歷史決策紀錄。
