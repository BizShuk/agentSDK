package middleware_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/harness"
	"github.com/bizshuk/agentsdk/middleware/loopguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder captures all calls so tests can assert order and counts.
type recorder struct {
	calls int
	effs  []core.Instruction
	reply func(eff core.Instruction) (core.State, *core.Event, bool, error)
}

func (r *recorder) run(_ context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
	r.calls++
	r.effs = append(r.effs, eff)
	if r.reply != nil {
		return r.reply(eff)
	}
	return state, nil, false, nil
}

// dispatcherFunc adapts a plain func to middleware.Next.
type dispatcherFunc func(eff core.Instruction) (core.State, *core.Event, bool, error)

func (d dispatcherFunc) run(_ context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
	return d(eff)
}

// toNext wraps dispatcherFunc as middleware.Next.
func toNext(d dispatcherFunc) middleware.Next {
	return d.run
}

func TestChainComposesOuterToInner(t *testing.T) {
	rec := &recorder{}
	var order []string

	mw := func(name string) middleware.Middleware {
		return func(next middleware.Next) middleware.Next {
			return func(ctx context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
				order = append(order, name+":before")
				s, in, term, err := next(ctx, state, eff)
				order = append(order, name+":after")
				return s, in, term, err
			}
		}
	}

	chain := middleware.Chain(mw("outer"), mw("inner"))
	wrap := chain(middleware.Next(rec.run))

	_, _, _, err := wrap(context.Background(), core.State{}, core.Instruction{Kind: core.INSTRUCTION_CALL_TOOL})
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls)
	assert.Equal(t,
		[]string{"outer:before", "inner:before", "inner:after", "outer:after"},
		order)
}

func TestRetryRecoversTransientError(t *testing.T) {
	attempts := 0
	d := func(eff core.Instruction) (core.State, *core.Event, bool, error) {
		attempts++
		if attempts < 3 {
			return core.State{}, nil, false, harness.SimpleRetryable{Reason: "5xx"}
		}
		return core.State{}, &core.Event{Kind: core.EVENT_MODEL_REPLY}, false, nil
	}
	mw := harness.Retry(harness.RetryConfig{
		N: 5, BaseBackoff: time.Microsecond, MaxBackoff: time.Microsecond,
		Sleeper: func(time.Duration) {},
	})

	_, _, _, err := mw(toNext(d))(context.Background(), core.State{}, core.Instruction{Kind: core.INSTRUCTION_CALL_MODEL})
	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestRetrySurfacesNonRetryable(t *testing.T) {
	attempts := 0
	d := func(eff core.Instruction) (core.State, *core.Event, bool, error) {
		attempts++
		return core.State{}, nil, false, errors.New("invalid args")
	}
	mw := harness.Retry(harness.RetryConfig{N: 3, Sleeper: func(time.Duration) {}})

	_, _, _, err := mw(toNext(d))(context.Background(), core.State{}, core.Instruction{Kind: core.INSTRUCTION_CALL_MODEL})
	require.Error(t, err)
	assert.Equal(t, 1, attempts)
}

func TestBudgetStopsDispatchWhenExceeded(t *testing.T) {
	rec := &recorder{}
	mw := harness.Budget()

	_, _, _, err := mw(middleware.Next(rec.run))(
		context.Background(),
		core.State{Budget: core.Budget{MaxTurns: 3, UsedTurns: 3}},
		core.Instruction{Kind: core.INSTRUCTION_CALL_MODEL},
	)
	require.Error(t, err)
	var be *harness.BudgetExceededError
	require.True(t, errors.As(err, &be))
	assert.Equal(t, "turn_budget", be.Reason)
	assert.Equal(t, 0, rec.calls)
}

func TestTimeoutCancelsSlowDispatch(t *testing.T) {
	d := func(state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
		// Slow inner — even if it doesn't honor ctx, the timeout layer
		// surfaces a deadline error when the deadline fires.
		time.Sleep(50 * time.Millisecond)
		return state, nil, false, nil
	}
	mw := harness.Timeout(harness.TimeoutConfig{PerEffect: 5 * time.Millisecond})

	_, _, _, err := mw(loopguardDispatcher(d))(context.Background(), core.State{},
		core.Instruction{Kind: core.INSTRUCTION_CALL_MODEL})
	require.Error(t, err)
	// Error is context.DeadlineExceeded (or wraps it) — the timeout
	// layer always reports a deadline breach.
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

func TestLoopguardRewritesToApproval(t *testing.T) {
	mw := loopguard.New(loopguard.Config{MaxRepeats: 5})
	var lastSeen core.Instruction
	d := func(state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
		lastSeen = eff
		return state, &core.Event{Kind: core.EVENT_TOOL_RESULT}, false, nil
	}
	toolCall := core.ToolCall{ID: "c1", Name: "read_log_tail", Args: map[string]any{"n": 5}}
	state := core.State{}
	for i := 0; i < 5; i++ {
		var err error
		state, _, _, err = mw(loopguardDispatcher(d))(context.Background(), state,
			core.Instruction{Kind: core.INSTRUCTION_CALL_TOOL, CallTool: &core.CallToolInstruction{Call: toolCall}})
		require.NoError(t, err)
	}
	require.Equal(t, core.INSTRUCTION_REQUEST_APPROVAL, lastSeen.Kind)
	require.NotNil(t, lastSeen.RequestApproval)
	assert.Equal(t, "loop_detected", lastSeen.RequestApproval.Reason)
}

func TestLoopguardResetsAfterObservation(t *testing.T) {
	mw := loopguard.New(loopguard.Config{MaxRepeats: 3})
	var lastSeen core.Instruction
	d := func(state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
		lastSeen = eff
		return state, &core.Event{Kind: core.EVENT_TOOL_RESULT}, false, nil
	}
	toolCall := core.ToolCall{ID: "c1", Name: "echo", Args: map[string]any{"msg": "hi"}}
	state := core.State{}

	for i := 0; i < 2; i++ {
		var err error
		state, _, _, err = mw(loopguardDispatcher(d))(context.Background(), state,
			core.Instruction{Kind: core.INSTRUCTION_CALL_TOOL, CallTool: &core.CallToolInstruction{Call: toolCall}})
		require.NoError(t, err)
	}
	assert.NotEqual(t, core.INSTRUCTION_REQUEST_APPROVAL, lastSeen.Kind)

	var err error
	state, _, _, err = mw(loopguardDispatcher(d))(context.Background(), state,
		core.Instruction{Kind: core.INSTRUCTION_CALL_MODEL, CallModel: &core.ModelRequest{RequestID: "r1"}})
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		state, _, _, err = mw(loopguardDispatcher(d))(context.Background(), state,
			core.Instruction{Kind: core.INSTRUCTION_CALL_TOOL, CallTool: &core.CallToolInstruction{Call: toolCall}})
		require.NoError(t, err)
	}
	assert.NotEqual(t, core.INSTRUCTION_REQUEST_APPROVAL, lastSeen.Kind)
}

func TestLoopguardStripsVolatileArgs(t *testing.T) {
	mw := loopguard.New(loopguard.Config{MaxRepeats: 5})
	sawApproval := false
	d := func(state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
		if eff.Kind == core.INSTRUCTION_REQUEST_APPROVAL {
			sawApproval = true
		}
		return state, &core.Event{Kind: core.EVENT_TOOL_RESULT}, false, nil
	}
	state := core.State{}
	for i := 0; i < 10; i++ {
		toolCall := core.ToolCall{
			ID: "c1", Name: "list_items",
			Args: map[string]any{"offset": i * 10, "limit": 10},
		}
		var err error
		state, _, _, err = mw(loopguardDispatcher(d))(context.Background(), state,
			core.Instruction{Kind: core.INSTRUCTION_CALL_TOOL, CallTool: &core.CallToolInstruction{Call: toolCall}})
		require.NoError(t, err)
	}
	assert.True(t, sawApproval, "loopguard must fire REQUEST_APPROVAL when only volatile args vary")
}

// loopguardDispatcher adapts a "preserve state" dispatcher into middleware.Next.
type loopguardDispatch func(state core.State, eff core.Instruction) (core.State, *core.Event, bool, error)

func loopguardDispatcher(d loopguardDispatch) middleware.Next {
	return func(_ context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
		return d(state, eff)
	}
}

func TestRetryableWrapping(t *testing.T) {
	transient := &harness.TransientError{Class: harness.RetryClassRateLimit, Cause: errors.New("429")}
	assert.True(t, harness.IsRetryable(transient))
	plain := errors.New("bad input")
	assert.False(t, harness.IsRetryable(plain))
}
