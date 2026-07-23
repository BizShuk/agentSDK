# Approval Mechanism Flow Diagram

> `SUPERSEDED 2026-07-24` — 已被 [`plans/2026-07-24-round-batch-and-interactive-seam.md`](../../../plans/2026-07-24-round-batch-and-interactive-seam.md) 取代。
> 修正版流程圖見新 plan §14；本檔的 §5 sequence diagram 畫的 `SubmitHumanDecision` → `Resume` 兩段驅動是錯的。

> 對應 plan：[`2026-07-23-agent-approval-resolver.md`](2026-07-23-agent-approval-resolver.md)
>
> 涵蓋三條流：input（listener → engine）、output（engine → sink）、approval（pause → resolver → resume），以及 agent 結構與 `app.Run` lifecycle。

---

## 1. 三流合一總覽

```mermaid
flowchart TB
    subgraph INPUT["Input flow (continuous)"]
        direction TB
        L["ObservationSource<br/>(file tail / journal / sensor / stdin)"] -->|"Observations(ctx) → chan Observation"| P["pumpListener<br/>(goroutine, ctx-aware)"]
        P -->|"payloadToString → Steer(text)"| Q["Engine.steerQueue"]
    end

    subgraph OUTPUT["Output flow (real-time)"]
        direction TB
        S["core.StreamEvent<br/>(STREAM_MESSAGE /<br/>STREAM_TOOL_START /<br/>STREAM_TOOL_RESULT)"] -->|"emitFolded"| SK["Engine.Sink"]
        SK -->|"OnStreamEvent(ev)"| APP_OUT["Application<br/>(stdout / file /<br/>websocket / logger)"]
    end

    subgraph APPROVAL["Approval flow (mid-run HITL)"]
        direction TB
        PA["Engine status =<br/>RUN_STATUS_PAUSED_APPROVAL"] -->|"if implements"| PH["Agent.OnPauseApproval<br/>(notify: Slack /<br/>PagerDuty / audit)"]
        PH -->|"per-decision ctx<br/>(WithApprovalTimeout)"| AR["Agent.ResolveApproval<br/>(stdin / HTTP / Kafka /<br/>policy / decisionCh)"]
        AR -->|"APPROVE"| SD["engine.SubmitHumanDecision<br/>+ engine.Resume"]
        AR -->|"REJECT"| RJ["Agent.OnReject<br/>(audit / rollback)"]
        SD -->|"loop back to Decide"| L
        RJ -.->|"exit loop"| OC
    end

    subgraph LOOP["Engine.runStep loop (Decide ⇄ Dispatch)"]
        direction LR
        D["Decide(state, event)<br/>(planning/ 6 rules)"] -->|"[]Instruction"| MW["Middleware chain<br/>retry → timeout →<br/>budget → loopguard"]
        MW -->|"dispatch"| I["runInstruction<br/>(CALL_MODEL / CALL_TOOL /<br/>REQUEST_APPROVAL / NOTIFY /<br/>CHECKPOINT / EMIT / DONE)"]
        I -->|"state' + Event"| D
    end

    Q -.->|"drainSteering<br/>(下一輪 Decide 前)"| D
    I -.->|"STREAM_TOOL_START<br/>for propose_fix"| S

    L --> LOOP
    S --> LOOP
    PA --> LOOP

    OC["Agent.OnComplete<br/>(final summary / cleanup)"]
```

---

## 2. Agent 結構（單一 type 整合三介面）

```mermaid
classDiagram
    direction LR

    class Agent {
        <<interface>>
        +Name() string
        +Bootstrap(ctx, ac) (Engine, State, err)
    }

    class Preflighter {
        <<optional>>
        +Preflight(ctx, ac) error
    }

    class PauseHandler {
        <<optional>>
        +OnPauseApproval(ctx, state) error
    }

    class ApprovalResolver {
        <<optional>>
        +ResolveApproval(ctx, state) (decision, by, err)
    }

    class RejectionHandler {
        <<optional>>
        +OnReject(ctx, state, by) error
    }

    class Completer {
        <<optional>>
        +OnComplete(ctx, state) error
    }

    Agent <|.. Preflighter : implements
    Agent <|.. PauseHandler : implements
    Agent <|.. ApprovalResolver : implements
    Agent <|.. RejectionHandler : implements
    Agent <|.. Completer : implements

    class LogDoctorAgent {
        +Fixture string
        +Fake bool
        +DecisionCh chan ApprovalDecision
        +Out io.Writer
        +listener ObservationSource
        +sink EventSink
        -pump goroutine
    }

    LogDoctorAgent ..|> Agent
    LogDoctorAgent ..|> Preflighter
    LogDoctorAgent ..|> PauseHandler
    LogDoctorAgent ..|> ApprovalResolver
    LogDoctorAgent ..|> RejectionHandler
    LogDoctorAgent ..|> Completer
```

**關鍵設計**：

- 五個 optional interface 全是 `func` 單一 method，可獨立 stub
- 不實作任何 optional interface = 既有行為（exit + 外部 verb）
- 實作 `ApprovalResolver` 但不實作 `PauseHandler` = 仍會通知但沒 Slack 推播
- 實作 `RejectionHandler` = 拒絕時觸發 audit；不實作 = 仍 reject 但無 audit hook

---

## 3. Input flow 細節

```mermaid
sequenceDiagram
    autonumber
    participant Src as "ObservationSource<br/>(LogFileListener / JournalWatcher)"
    participant Pump as "pumpListener<br/>(goroutine)"
    participant Eng as runtime.Engine
    participant FSM as "Decide FSM<br/>(think_then_act)"

    Note over Src,Pump: Spawned by Bootstrap<br/>after engine fully assembled
    Src->>Pump: chan Observation (capacity 1)
    loop for each obs
        Pump->>Pump: payloadToString(obs.Payload)
        alt text == "" (skip)
            Pump-->>Pump: drop
        else text != ""
            Pump->>Eng: eng.Steer(text)
            Note over Eng: append to steerQueue<br/>(concurrent-safe)
        end
    end
    Note over Src: channel closed or ctx cancelled → pump exits

    Note over Eng,FSM: Each Decide cycle:
    Eng->>FSM: drainSteering()
    FSM->>FSM: insert user msg into state.Messages
    FSM->>FSM: NextStep() → emit CALL_MODEL
    FSM-->>Eng: continue loop
```

**資料形狀對應**：

```text
core.Observation.Payload  (any)              core.Engine.Steer (string)
   │                                                  │
   └─ payloadToString  ──────────────────────────────→┘
        ├─ nil        → "" (drop)
        ├─ string     → 原文
        ├─ Stringer   → .String()
        └─ 其他         → fmt.Sprintf("%v", ...)
```

---

## 4. Output flow 細節

```mermaid
sequenceDiagram
    autonumber
    participant FSM as Decide FSM
    participant Run as runInstruction
    participant Hook as "fireHook<br/>(PreToolUse block)"
    participant Sink as Engine.Sink
    participant App as Application

    Note over FSM,Run: runStep iter: Decide → runInstruction
    FSM->>Run: []Instruction (e.g. CALL_TOOL propose_fix)
    Run->>Hook: PreToolUse(event)
    alt hook blocks
        Hook-->>Run: HookDecision{Block, Reason}
        Run->>Run: synthesize failed ToolResult
    end
    Run->>Run: dispatch via middleware chain
    Run->>Run: actually CALL_TOOL or CALL_MODEL

    Note over Run,Sink: emitFolded translates Event → StreamEvent
    Run->>Sink: STREAM_TOOL_START (call name + args)
    Run->>Sink: STREAM_TOOL_RESULT (ok / err)
    Run->>Sink: STREAM_MESSAGE (assistant text)
    Sink->>App: OnStreamEvent(ev)
    App->>App: switch on ev.Kind

    alt propose_fix blocked by ApprovalGate
        Run-->>FSM: REQUEST_APPROVAL instruction
        Note over FSM: → next section (approval flow)
    end
```

**三種 `core.StreamEvent` 對應**：

| `ev.Kind` | 欄位 | 應用拿到什麼 |
| --- | --- | --- |
| `STREAM_MESSAGE` | `Text` | 完整 assistant 文字（一次一則） |
| `STREAM_TOOL_START` | `ToolCall` (含 Name + Args) | model 決定要呼叫的工具（**pre-dispatch**，可在 approval gate 前觀察） |
| `STREAM_TOOL_RESULT` | `ToolResult` (含 OK + Output + Error) | 工具實際回傳（含 hook 阻擋的 failed 結果） |

**關鍵時點**：`STREAM_TOOL_START` 是 **pre-dispatch**，所以 sink 可以在 engine pause 之前就先通知 operator。`STREAM_TOOL_RESULT` 是 **post-dispatch**，所以 hook 阻擋會反映在這裡（OK=false）。

---

## 5. Approval flow 細節（Q13 提案的核心）

```mermaid
sequenceDiagram
    autonumber
    participant FSM as Decide FSM
    participant StepLoop as runStep loop
    participant Status as state.Status
    participant Run as app.Run
    participant Agent as "Agent<br/>(LogDoctorAgent)"
    participant Store as StateStore

    Note over FSM: emit REQUEST_APPROVAL<br/>(propose_fix HIGH risk →<br/>L2 autonomy → ASK)
    FSM->>StepLoop: INSTRUCTION_REQUEST_APPROVAL
    StepLoop->>Status: append PendingApproval<br/>set RUN_STATUS_PAUSED_APPROVAL
    StepLoop-->>Run: safeRun returns final

    Note over Run: NEW: PAUSED loop in app.Run
    loop while Status == PAUSED_APPROVAL
        Run->>Agent: OnPauseApproval(ctx, final)? [optional]
        Agent-->>Run: notify Slack / format proposal

        alt Agent implements ApprovalResolver
            Run->>Run: ctx' = WithApprovalTimeout
            Run->>Agent: ResolveApproval(ctx', final)
            Agent->>Agent: block on DecisionCh / HTTP / Kafka
            Agent-->>Run: APPROVE / REJECT

            alt APPROVE
                Run->>Store: engine.SubmitHumanDecision<br/>(decision → PendingApprovals[i])
                Run->>StepLoop: engine.Resume(ctx, runID)
                StepLoop->>StepLoop: runStep continues<br/>(approved tool executes,<br/>model sees result,<br/>may emit another REQUEST_APPROVAL)
                StepLoop-->>Run: final (new status)
            else REJECT
                Run->>Agent: OnReject(ctx, final, by)? [optional]
                Agent-->>Run: audit / rollback
                Run->>Store: engine.SubmitHumanDecision (REJECT)
                Note over Run: break out of PAUSED loop
            end
        else Agent does NOT implement
            Note over Run: break - fallback to<br/>external verb (approve --run-id)
        end
    end

    Run->>Agent: OnComplete(ctx, final)
    Agent-->>Run: final summary
    Run->>Run: exit code 0
```

**State 機狀態轉換**：

```text
COMPLETED ←────── (DONE instruction) ──────────── Decide FSM
   │
   │
FAILED ←────────── (panic / error) ──────────────── safeRun
   │
   │
PAUSED_APPROVAL ←─ (REQUEST_APPROVAL) ───────────── Decide FSM
   │
   ├─→ [PauseHandler] OnPauseApproval
   ├─→ [ApprovalResolver] ResolveApproval
   │     ├─ APPROVE → SubmitHumanDecision + Resume
   │     │     ├─ approved tool runs → next Decide → may PAUSE again
   │     │     └─ done naturally → COMPLETED
   │     └─ REJECT  → [RejectionHandler] OnReject → exit
   └─→ (no resolver) → exit, leave PendingApprovals for external verb
```

---

## 6. `app.Run` lifecycle 全圖（含 PAUSED loop）

```mermaid
flowchart TB
    Start([Process start]) --> Config

    subgraph S1["1. config"]
        Config["config.OpenForCLI(name)<br/>→ AppConfig<br/>(DataDir / LogDir /<br/>StateStore / WAL / RunID)"]
    end

    subgraph S2["2. preflight (optional)"]
        Pre["Agent.Preflight<br/>(validate API key,<br/>check dependencies)"]
    end

    subgraph S3["3. deadline"]
        DL["WithTimeout(d) →<br/>ctx with deadline"]
    end

    subgraph S4["4. bootstrap"]
        Boot["Agent.Bootstrap(ctx, ac)<br/>→ *Engine + State<br/>+ spawn pumpListener"]
    end

    subgraph S5["5. run"]
        Safe["safeRun(ctx, eng, state)<br/>(panic recovery → markFailed)"]
    end

    subgraph S5a["5a. PAUSED loop (NEW)"]
        Check{Status ==<br/>PAUSED_APPROVAL?}
        Pause["Agent.OnPauseApproval"]
        Resolve["Agent.ResolveApproval<br/>(WithApprovalTimeout)"]
        Decide{APPROVE?}
        Submit["engine.SubmitHumanDecision<br/>+ engine.Resume"]
        Reject["Agent.OnReject"]
    end

    subgraph S6["6. complete (optional)"]
        Comp["Agent.OnComplete"]
    end

    Exit([exit code 0 or 1])

    Config --> Pre --> DL --> Boot --> Safe
    Safe --> Check
    Check -->|"yes + resolver"| Pause --> Resolve --> Decide
    Decide -->|"APPROVE"| Submit --> Check
    Decide -->|"REJECT"| Reject --> Comp
    Check -->|"no / no resolver"| Comp
    Comp --> Exit
```

**每段契約**：

| 段 | 必跑？ | 失敗時 | 輸出 |
| --- | --- | --- | --- |
| 1. config | 必 | exit 1 + slog error | AppConfig |
| 2. preflight | optional | exit 1（早於所有副作用）| — |
| 3. deadline | optional（default 30min）| ctx cancel | bounded ctx |
| 4. bootstrap | 必 | exit 1 | Engine + State |
| 5. run | 必 | panic → markFailed(FAILED) + exit 1 | final State |
| 5a. PAUSED loop | optional（resolver 存在才跑）| ResolveApproval error → exit 1 | 繼續 ResolveApproval 直到 non-paused |
| 6. complete | optional | exit 1 | final State |

---

## 7. 三流的 timing 對齊（同一個 run 內）

```mermaid
gantt
    title Input / Output / Approval timing within one run
    dateFormat X
    axisFormat %s

    section Input
    listener emits obs1      :a1, 0, 2
    listener emits obs2      :a2, 3, 5
    listener emits obs3      :a3, 6, 8

    section Decide cycle
    Decide round 1           :b1, 1, 2
    Decide round 2           :b2, 4, 5
    Decide round 3           :b3, 7, 10
    Decide round 4 (post-approval) :b4, 13, 14

    section Output
    sink emits msg           :c1, 1, 2
    sink emits tool_start    :c2, 7, 8
    sink emits tool_result   :c3, 9, 10

    section Approval
    pause + resolve          :d1, 11, 13
    submit + resume          :crit, 13, 14
```

時間軸：

```text
t=0   listener.emit(obs1) ──────────────┐
t=1   Decide(round1)                    │
t=2     emit CALL_MODEL                 │
t=3   listener.emit(obs2)               │ drainSteering 在 Decide 前
t=4   Decide(round2)                    │
t=5     append obs2 to state.Messages   │
t=6   listener.emit(obs3)               │
t=7   Decide(round3)                    │
t=8     CALL_TOOL propose_fix           │
        sink emits STREAM_TOOL_START    │
t=9     approval_gate → REQUEST_APPROVAL
        sink emits STREAM_TOOL_RESULT (failed)
t=10    Status = PAUSED_APPROVAL
        app.Run enters PAUSED loop
t=11  OnPauseApproval → Slack notify
t=12  ResolveApproval blocks on DecisionCh
        operator types "y"
t=13  SubmitHumanDecision(APPROVE)
      Resume → next Decide
t=14  Decide(round4)
      approved tool runs
      model emits DONE
      Status = COMPLETED
t=15  OnComplete → exit 0
```

---

## 8. 失敗 mode 與 fallback 對照

| 失敗 mode | 觸發條件 | 既有行為 | 新行為（有 resolver） |
| --- | --- | --- | --- |
| background process 無人值守 | `ResolveApproval` 阻塞中 | n/a（無 resolver） | `WithApprovalTimeout` 觸發 ctx cancel → exit 1 + 已 persist 的 PendingApprovals 留在 Store |
| operator 拒絕 | `ResolveApproval` 回 REJECT | n/a | `OnReject` 跑 + exit 0（audit 一致） |
| Resolver panic | user code panic | n/a | `safeRun` 已有 panic recovery；新 loop 在 safeRun 外，**需擴充 panic recovery**（見 plan Task 3） |
| Hook blocked + Approval 雙重阻擋 | PreToolUse block + ApprovalGate ASK | hook 阻擋優先 → failed ToolResult → model 看到 | 同上；resolver 不會被呼叫 |
| 跨 process 審批 | Agent 不實作 ApprovalResolver | exit，operator 跑 `approve --run-id` + `resume --run-id` | **保留不變**——loop 偵測無 resolver 就 break |
| 多重 pending approval | 短時間內 2 個 REQUEST_APPROVAL | 第一次 pause 後，runtime 不再處理（直到 resume） | loop 連續 resolve：第一個 approve → resume → 下一輪 decide → 第二個 pause → 第二次 resolve |

---

## 9. 對應程式碼位置速查

| 概念 | 檔案 | 行（plan 對應） |
| --- | --- | --- |
| `PauseHandler` interface | `app/agent.go` | Task 1 |
| `ApprovalResolver` interface | `app/agent.go` | Task 1 |
| `RejectionHandler` interface | `app/agent.go` | Task 1 |
| `WithApprovalTimeout` option | `app/app.go` | Task 2 |
| PAUSED loop | `app/app.go::Run` | Task 3 |
| `agent.WithListener` | `agent/options.go` | 既有 |
| `agent.WithSink` / `SinkFunc` | `agent/options.go` | 既有 |
| `pumpListener` goroutine | `agent/build.go::Bootstrap` | 既有 |
| `Engine.Steer` / `drainSteering` | `runtime/harness.go` | 既有 |
| `Engine.SubmitHumanDecision` | `runtime/loop.go:308-334` | 既有 |
| `Engine.Resume` | `runtime/loop.go` | 既有 |
| `REQUEST_APPROVAL` instruction 觸發 pause | `runtime/loop.go:154-168` | 既有 |
| `LogDoctorAgent` 範例 | `sample/logdoctor/agent.go` | Task 4 |
| 架構圖 §10 三軸 | `docs/architecture.svg` | 既有 |

---

## 10. 一句話總結

> Agent = input listener（goroutine → Steer queue）+ output sink（nil-safe StreamEvent 出口）+ approve resolver（block on decision channel）三流的**單一 type 整合點**；`app.Run` 在 `RUN_STATUS_PAUSED_APPROVAL` 時 loop 呼叫 `OnPauseApproval` → `ResolveApproval` → `SubmitHumanDecision + Resume`，直到非 paused；不實作 `ApprovalResolver` = 退回既有「exit + 外部 verb」語意，跨 process 場景自動 fallback。