// Package cmd hosts the demo-middleware CLI and its demos.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/harness"
	"github.com/bizshuk/agentsdk/middleware/loopguard"
)

// demo 描述一個可執行的 middleware 示範。
type demo struct {
	id    string
	title string
	blurb string
	run   func(w io.Writer) error
}

// demos 回傳 catalog,順序即展示順序(也是 chain 由外到內的順序)。
func demos() []demo {
	return []demo{
		{"retry", "Retry — 重試瞬時錯誤", "對 RetryableError 重試 N 次(指數退避,此處 Sleeper 抽掉);非 retryable 立即上拋。", demoRetry},
		{"budget", "Budget — 額度守門", "dispatch 前檢查 state.Budget;超額直接回 BudgetExceededError,不呼叫 inner。", demoBudget},
		{"timeout", "Timeout — 單一 effect 逾時", "以 context.WithTimeout 綁定每個 effect 的時限,逾時回 DeadlineExceeded。", demoTimeout},
		{"loopguard", "Loopguard — 迴圈偵測", "同指紋的 CALL_TOOL 連續 N 次無新觀察 → 改寫成 REQUEST_APPROVAL 暫停。", demoLoopguard},
		{"chain", "Chain — 由外到內組合", "retry → timeout → budget → loopguard → base;示範額度層擋在 base 之前。", demoChain},
	}
}

// baseState / callTool / callModel 是 demo 共用的素材。
func baseState() core.State { return core.State{RunID: "demo", Status: core.RUN_STATUS_RUNNING} }

func callTool() core.Instruction {
	return core.Instruction{
		Kind:     core.INSTRUCTION_CALL_TOOL,
		CallTool: &core.CallToolInstruction{Call: core.ToolCall{ID: "c1", Name: "read_log_tail", Args: map[string]any{"n": 20}}},
	}
}

// succeed 是「成功」的下游事件。
func okEvent() *core.Event { return &core.Event{Kind: core.EVENT_TOOL_RESULT} }

// --- retry ---------------------------------------------------------------

func demoRetry(w io.Writer) error {
	ctx := context.Background()

	fmt.Fprintln(w, "情境 A:前兩次瞬時失敗,第三次成功(N=5)")
	attempts := 0
	base := middleware.Next(func(_ context.Context, s core.State, _ core.Instruction) (core.State, *core.Event, bool, error) {
		attempts++
		if attempts <= 2 {
			fmt.Fprintf(w, "  attempt %d → transient error (503 upstream)\n", attempts)
			return s, nil, false, harness.SimpleRetryable{Reason: "503 upstream"}
		}
		fmt.Fprintf(w, "  attempt %d → success\n", attempts)
		return s, okEvent(), false, nil
	})
	next := harness.Retry(harness.RetryConfig{N: 5, Sleeper: func(time.Duration) {}})(base)
	_, ev, _, err := next(ctx, baseState(), callTool())
	fmt.Fprintf(w, "  結果:err=%v event=%v(共 %d 次嘗試)\n", err, evKind(ev), attempts)

	fmt.Fprintln(w, "\n情境 B:非 retryable 錯誤 → 只嘗試一次就上拋")
	attempts = 0
	base2 := middleware.Next(func(_ context.Context, s core.State, _ core.Instruction) (core.State, *core.Event, bool, error) {
		attempts++
		fmt.Fprintf(w, "  attempt %d → fatal error (400 bad request)\n", attempts)
		return s, nil, false, errors.New("400 bad request")
	})
	next2 := harness.Retry(harness.RetryConfig{N: 5, Sleeper: func(time.Duration) {}})(base2)
	_, _, _, err2 := next2(ctx, baseState(), callTool())
	fmt.Fprintf(w, "  結果:err=%v(共 %d 次嘗試)\n", err2, attempts)
	return nil
}

// --- budget --------------------------------------------------------------

func demoBudget(w io.Writer) error {
	ctx := context.Background()
	baseCalls := 0
	base := middleware.Next(func(_ context.Context, s core.State, _ core.Instruction) (core.State, *core.Event, bool, error) {
		baseCalls++
		return s, okEvent(), false, nil
	})
	next := harness.Budget()(base)

	healthy := baseState()
	healthy.Budget = core.Budget{MaxTurns: 3, UsedTurns: 1}
	_, _, _, err := next(ctx, healthy, callTool())
	fmt.Fprintf(w, "額度內  MaxTurns=3 UsedTurns=1 → err=%v(base 被呼叫,共 %d 次)\n", err, baseCalls)

	exhausted := baseState()
	exhausted.Budget = core.Budget{MaxTurns: 3, UsedTurns: 3}
	before := baseCalls
	_, _, _, err = next(ctx, exhausted, callTool())
	fmt.Fprintf(w, "額度滿  MaxTurns=3 UsedTurns=3 → err=%v\n", err)
	fmt.Fprintf(w, "  BudgetExceeded? %v;base 是否被呼叫:%v\n", harness.IsBudgetExceeded(err), baseCalls != before)
	return nil
}

// --- timeout -------------------------------------------------------------

func demoTimeout(w io.Writer) error {
	ctx := context.Background()

	fast := middleware.Next(func(_ context.Context, s core.State, _ core.Instruction) (core.State, *core.Event, bool, error) {
		return s, okEvent(), false, nil
	})
	next := harness.Timeout(harness.TimeoutConfig{PerEffect: 20 * time.Millisecond})(fast)
	_, _, _, err := next(ctx, baseState(), callTool())
	fmt.Fprintf(w, "快 effect(立即回) PerEffect=20ms → err=%v\n", err)

	slow := middleware.Next(func(cctx context.Context, s core.State, _ core.Instruction) (core.State, *core.Event, bool, error) {
		<-cctx.Done() // 尊重 deadline:阻塞到 context 被取消
		return s, nil, false, cctx.Err()
	})
	tripped := false
	next2 := harness.Timeout(harness.TimeoutConfig{
		PerEffect: 20 * time.Millisecond,
		OnTimeout: func(core.Instruction) { tripped = true },
	})(slow)
	_, _, _, err2 := next2(ctx, baseState(), callTool())
	fmt.Fprintf(w, "慢 effect(等 deadline) PerEffect=20ms → err=%v\n", err2)
	fmt.Fprintf(w, "  DeadlineExceeded? %v;OnTimeout 觸發:%v\n", errors.Is(err2, context.DeadlineExceeded), tripped)
	return nil
}

// --- loopguard -----------------------------------------------------------

func demoLoopguard(w io.Writer) error {
	ctx := context.Background()
	var received []core.InstructionKind
	base := middleware.Next(func(_ context.Context, s core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
		received = append(received, eff.Kind)
		return s, okEvent(), false, nil
	})
	next := loopguard.New(loopguard.Config{MaxRepeats: 3})(base)

	// loopguard 把重試計數存在 state.WorkingMemory(scratch),靠 runtime 的
	// preStep seed 跨迭代帶著走。demo 端要自己維持同一份非 nil 的 map,
	// 否則每次都從空 scratch 起算,計數永遠歸零、永不觸發。
	state := baseState()
	state.WorkingMemory = map[string]any{}
	for i := 1; i <= 5; i++ {
		received = received[:0]
		_, _, _, err := next(ctx, state, callTool())
		if err != nil {
			return err
		}
		note := ""
		if received[0] == core.INSTRUCTION_REQUEST_APPROVAL {
			note = "  ← 迴圈被攔截,暫停待人工審核"
		}
		fmt.Fprintf(w, "  第 %d 次 CALL_TOOL(read_log_tail n=20) → base 收到:%-16s%s\n", i, received[0], note)
	}
	fmt.Fprintln(w, "  → 第 3 次達到 MaxRepeats,loopguard 把 call_tool 改寫成 request_approval(Reason=loop_detected);之後維持 armed。")
	return nil
}

// --- chain ---------------------------------------------------------------

func demoChain(w io.Writer) error {
	ctx := context.Background()
	baseCalls := 0
	base := middleware.Next(func(_ context.Context, s core.State, _ core.Instruction) (core.State, *core.Event, bool, error) {
		baseCalls++
		return s, okEvent(), false, nil
	})
	chain := middleware.Chain(
		harness.Retry(harness.RetryConfig{N: 3, Sleeper: func(time.Duration) {}}),
		harness.Timeout(harness.TimeoutConfig{PerEffect: time.Second}),
		harness.Budget(),
		loopguard.New(loopguard.Config{MaxRepeats: 5}),
	)
	next := chain(base)

	fmt.Fprintln(w, "組合:retry → timeout → budget → loopguard → base")

	healthy := baseState()
	healthy.Budget = core.Budget{MaxTurns: 10, UsedTurns: 1}
	_, ev, _, err := next(ctx, healthy, callTool())
	fmt.Fprintf(w, "健康請求 → err=%v event=%v;base 呼叫次數=%d\n", err, evKind(ev), baseCalls)

	before := baseCalls
	exhausted := baseState()
	exhausted.Budget = core.Budget{MaxTurns: 10, UsedTurns: 10}
	_, _, _, err = next(ctx, exhausted, callTool())
	fmt.Fprintf(w, "額度已滿 → err=%v;base 是否被呼叫:%v(budget 層擋在 base 前)\n", err, baseCalls != before)
	return nil
}

// --- helpers -------------------------------------------------------------

func evKind(ev *core.Event) string {
	if ev == nil {
		return "<nil>"
	}
	return string(ev.Kind)
}