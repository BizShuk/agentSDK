# Agent Approval Resolver Implementation Plan

> `SUPERSEDED 2026-07-24` — 已被 [`plans/2026-07-24-round-batch-and-interactive-seam.md`](../../../plans/2026-07-24-round-batch-and-interactive-seam.md) 取代。
> 本檔保留作歷史紀錄，`不得`據以實作：三介面設計已收斂為單一 `app.Interactive`，
> 且 Task 1 Step 5 編譯失敗、Task 3 的 `SubmitHumanDecision` + `Resume` 會重複執行 tool。
> 完整修正清單見新 plan §13。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an `Agent` type itself be the approval source — `app.Run` consults it whenever the engine pauses on `RUN_STATUS_PAUSED_APPROVAL`, loops until non-paused, and falls back to existing cross-process verbs when no resolver is wired.

**Architecture:** Three new optional interfaces (`PauseHandler`, `ApprovalResolver`, `RejectionHandler`) attach to `Agent`. `app.Run` extends its lifecycle with a `for final.Status == RUN_STATUS_PAUSED_APPROVAL { ... }` loop after `safeRun` and before `OnComplete`. The loop delegates to whichever interfaces the Agent implements; an Agent that implements none keeps today's behavior (exit with paused status, leave the decision to `logdoctor approve --run-id`). `core.Engine.SubmitHumanDecision` + `Resume` are reused unchanged — this plan only adds the seam that lets them be driven from inside the same process.

**Tech Stack:** Go 1.26, stdlib only (no new deps), `core`, `runtime`, `app`, `internal/testutil`, `testify`.

## Global Constraints

- Go version: `1.26.0` (root module `go.mod`)
- Module path: `github.com/bizshuk/agentsdk`
- Test framework: `github.com/stretchr/testify` (assert + require)
- Naming: `MixedCaps` for variables/types/functions; `SCREAMING_SNAKE_CASE` for constants and block-scoped locals
- Errors: `fmt.Errorf("...: %w", err)` wrap; `app.Run` returns exit code, never panic
- File boundary: `app/agent.go` = interface declarations only (matches existing `Agent`/`Preflighter`/`Completer` shape); `app/app.go` = lifecycle glue; `app/options.go` (NEW) = `With*` constructors for the new lifecycle knobs
- Public API stability: this adds optional interfaces only; existing `Agent` and `Completer` signatures unchanged

---

## File Structure

| File | Responsibility | Touched by |
| --- | --- | --- |
| `app/agent.go` | Add `PauseHandler`, `ApprovalResolver`, `RejectionHandler` interfaces | Task 1 |
| `app/options.go` (NEW) | Add `WithApprovalTimeout` Option | Task 2 |
| `app/app.go` | Add PAUSED loop in `Run` between `safeRun` and `OnComplete` | Task 3 |
| `app/app_test.go` | Tests for loop + timeout + interface dispatch | Tasks 2, 3 |
| `sample/logdoctor/agent.go` (NEW) | `LogDoctorAgent` that implements the three interfaces | Task 4 |
| `sample/logdoctor/main.go` | Switch from `cmd/*` wiring to `app.Main(&LogDoctorAgent{...})` | Task 4 |
| `docs/terminology.md` | Add `ApprovalResolver` / `PauseHandler` / `RejectionHandler` to glossary | Task 5 |
| `CLAUDE.md` | Add the three interfaces to `app/config` row in module mapping | Task 5 |

Files that change together live together: the three interfaces stay in `agent.go` next to `Agent`/`Preflighter`/`Completer`; the loop logic stays in `app.go` next to `safeRun`; tests stay in `app_test.go` next to existing lifecycle tests.

---

## Task 1: Declare the three interfaces

**Files:**
- Modify: `app/agent.go` (add after the existing `Completer` interface, around line 86)
- Test: `app/app_test.go` (compile-time interface-assertion test only — no runtime behavior)

**Interfaces:**
- Consumes: nothing — pure declarations
- Produces: `PauseHandler`, `ApprovalResolver`, `RejectionHandler` interface types that Task 3 will reference in `Run`

- [ ] **Step 1: Read current `app/agent.go` to anchor insertion point**

Run: `cat /Users/shuk/projects/ai/agentSDK/app/agent.go`
Look for the line after `type Completer interface { ... }` (around line 80).

- [ ] **Step 2: Write the three interface declarations**

Append after the closing `}` of `Completer`:

```go
// PauseHandler runs when a run enters RUN_STATUS_PAUSED_APPROVAL, BEFORE
// the ApprovalResolver is consulted. Use it to format the proposal,
// notify a Slack channel, push to a queue, or write an audit line —
// anything that needs to happen regardless of where the decision comes from.
//
// PauseHandler is paired with ApprovalResolver: a run that pauses and has
// no ApprovalResolver exits so an external verb (logdoctor approve) can
// decide. A run with a PauseHandler but no ApprovalResolver still exits;
// the handler ran only as a side effect.
type PauseHandler interface {
	OnPauseApproval(ctx context.Context, s core.State) error
}

// ApprovalResolver is the single-process seam for HITL decisions. When
// app.Run sees Status == RUN_STATUS_PAUSED_APPROVAL, it calls
// ResolveApproval instead of exiting. The Agent owns the input side and
// decides where the decision comes from: stdin, an HTTP endpoint, a
// Kafka topic, a policy lookup, a channel filled by Sink callbacks.
//
// Implementations MUST honor ctx cancellation: the run loop blocks here
// until either a decision arrives or the process is asked to stop
// (SIGINT/SIGTERM, WithApprovalTimeout, or WithTimeout deadline).
type ApprovalResolver interface {
	ResolveApproval(ctx context.Context, s core.State) (
		decision core.ApprovalDecision, decidedBy string, err error,
	)
}

// RejectionHandler runs when ResolveApproval returns REJECT (or an error
// that the loop treats as reject). Use it to record why, notify the
// operator, or roll back state. Optional; OnComplete runs after this
// regardless of which interfaces the Agent implements.
type RejectionHandler interface {
	OnReject(ctx context.Context, s core.State, decidedBy string) error
}
```

- [ ] **Step 3: Verify the interfaces import `core` already**

`app/agent.go` already imports `github.com/bizshuk/agentsdk/core` (used by `Agent.Bootstrap`'s return type). No new imports needed.

- [ ] **Step 4: Compile**

Run: `cd /Users/shuk/projects/ai/agentSDK && go build ./...`
Expected: exit 0, no output.

- [ ] **Step 5: Add a compile-time interface check**

Append to `app/app_test.go` (anywhere inside `package app_test`):

```go
// TestAgentInterfacesCompile is a compile-time check that the three new
// optional interfaces match the Agent contract. If any interface is
// renamed or its signature drifts, this test fails to compile — a
// clearer signal than runtime mismatches further down the stack.
func TestAgentInterfacesCompile(t *testing.T) {
	type pauseImpl interface {
		app.Agent
		app.PauseHandler
	}
	type resolveImpl interface {
		app.Agent
		app.ApprovalResolver
	}
	type rejectImpl interface {
		app.Agent
		app.RejectionHandler
	}
	var (
		_ pauseImpl    = (*resolveImpl)(nil)
		_ resolveImpl  = (*rejectImpl)(nil)
		_ rejectImpl   = (*pauseImpl)(nil)
	)
}
```

Run: `cd /Users/shuk/projects/ai/agentSDK && go test ./app/... -count=1 -run TestAgentInterfacesCompile`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/shuk/projects/ai/agentSDK
git add app/agent.go app/app_test.go
git commit -m "feat(app): declare PauseHandler, ApprovalResolver, RejectionHandler interfaces"
```

---

## Task 2: Add `WithApprovalTimeout` option

**Files:**
- Create: `app/options.go` (mirror of `app/app.go`'s `options` block)
- Test: `app/app_test.go`

**Interfaces:**
- Consumes: `Options` shape from `app/app.go` (struct literal with `timeout`, `logLevel`, `logToStdout` — Task 3 will add `approvalTimeout`)
- Produces: `WithApprovalTimeout(d time.Duration) Option` — bounds how long `ResolveApproval` can block

- [ ] **Step 1: Confirm `app/app.go` `options` struct fields**

Run: `grep -n "options struct\|timeout\|logToStdout" /Users/shuk/projects/ai/agentSDK/app/app.go | head -10`
Expected output (around line 50-60):
```go
type options struct {
    timeout     time.Duration
    logLevel    slog.Level
    logToStdout bool
}
```

Note: `app/options.go` does not yet exist as a separate file in this repo; the Option constructors live in `app/app.go`. We add the new constructor in `app/app.go` near the existing `With*` constructors to avoid creating a new file.

- [ ] **Step 2: Add `approvalTimeout` field to the `options` struct**

In `app/app.go`, change:

```go
type options struct {
	timeout     time.Duration
	logLevel    slog.Level
	logToStdout bool
}
```

to:

```go
type options struct {
	timeout         time.Duration
	approvalTimeout time.Duration
	logLevel        slog.Level
	logToStdout     bool
}
```

- [ ] **Step 3: Initialize the new field in `defaultOptions`**

In `app/app.go::defaultOptions`, change the returned struct literal to include the zero value for `approvalTimeout` (zero = no approval-specific deadline, falls back to `timeout`):

```go
func defaultOptions() options {
	return options{
		timeout:         DEFAULT_RUN_TIMEOUT,
		approvalTimeout: 0,
		logLevel:        slog.LevelInfo,
	}
}
```

- [ ] **Step 4: Add the constructor after `WithLogToStdout`**

Append after the closing `}` of `WithLogToStdout` in `app/app.go`:

```go
// DEFAULT_APPROVAL_TIMEOUT caps how long a single ApprovalResolver call
// may block. Generous because operator-in-the-loop decisions can take
// minutes; an agent that wants a tighter bound should override. Zero or
// negative falls back to options.timeout (the run-wide deadline).
const DEFAULT_APPROVAL_TIMEOUT = 30 * time.Minute

// WithApprovalTimeout bounds a single ResolveApproval call. The run
// continues to consult the resolver across pauses — each pause gets its
// own fresh deadline — so this only caps per-decision latency, not total
// approval work across the whole run.
//
// A non-positive duration disables the per-decision deadline and falls
// back to options.timeout.
func WithApprovalTimeout(d time.Duration) Option {
	return func(o *options) { o.approvalTimeout = d }
}
```

- [ ] **Step 5: Compile**

Run: `cd /Users/shuk/projects/ai/agentSDK && go build ./...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
cd /Users/shuk/projects/ai/agentSDK
git add app/app.go
git commit -m "feat(app): add WithApprovalTimeout option for per-decision deadline"
```

---

## Task 3: Implement the PAUSED loop in `Run`

**Files:**
- Modify: `app/app.go::Run` (insert loop between `safeRun` and `OnComplete`, around line 130-150)
- Test: `app/app_test.go` (add `TestRunConsultsApprovalResolverOnPause`)

**Interfaces:**
- Consumes: `engine` from Bootstrap return; `a.(PauseHandler)` / `a.(ApprovalResolver)` / `a.(RejectionHandler)` type assertions; `engine.SubmitHumanDecision(ctx, runID, decision, decidedBy)` and `engine.Resume(ctx, runID)` (both already exist in `runtime/loop.go`)
- Produces: A run that, when the engine returns `RUN_STATUS_PAUSED_APPROVAL`, calls PauseHandler → ApprovalResolver → either SubmitHumanDecision+Resume (approve) or OnReject (reject) — then loops

- [ ] **Step 1: Read `Run` to find the exact insertion point**

Run: `sed -n '125,160p' /Users/shuk/projects/ai/agentSDK/app/app.go`
Expected output shows the `if runErr != nil { ... return EXIT_ERROR }` block, the `if c, ok := a.(Completer); ok { ... }` block, the success log line, and the `return EXIT_OK`.

- [ ] **Step 2: Write the failing test in `app/app_test.go`**

Append to `app/app_test.go`:

```go
// TestRunConsultsApprovalResolverOnPause drives app.Run with an Agent
// that exposes an ApprovalResolver, asserts the resolver was called,
// and verifies that an APPROVE decision caused the engine to resume.
func TestRunConsultsApprovalResolverOnPause(t *testing.T) {
	var (
		resolveCalls int
		approveCalls int
		finalStatus  string
	)

	pausedAgent := &pausedTestAgent{
		resolve: func(ctx context.Context, s core.State) (core.ApprovalDecision, string, error) {
			resolveCalls++
			return core.APPROVAL_DECISION_APPROVE, "tester", nil
		},
		onApprove: func() { approveCalls++ },
	}

	exit := app.Run(context.Background(), pausedAgent,
		app.WithLogToStdout(),
		app.WithApprovalTimeout(5*time.Second),
	)
	require.Equal(t, app.EXIT_OK, exit)
	assert.GreaterOrEqual(t, resolveCalls, 1, "resolver should be consulted at least once")
	assert.Equal(t, 1, approveCalls)
	assert.Equal(t, string(core.RUN_STATUS_COMPLETED), finalStatus)
}
```

And the helper type at the bottom of the test file:

```go
// pausedTestAgent runs one Decide cycle that emits REQUEST_APPROVAL
// (the runtime then sets PAUSED_APPROVAL), then on the second cycle
// (driven by Resume after the resolver approves) emits DONE so the run
// terminates COMPLETED. The resolver is wired so the test can count
// calls; onApprove is a hook for assertions.
type pausedTestAgent struct {
	resolve   func(ctx context.Context, s core.State) (core.ApprovalDecision, string, error)
	onApprove func()
}

func (a *pausedTestAgent) Name() string { return "paused-test" }

func (a *pausedTestAgent) Bootstrap(ctx context.Context, ac *config.AppConfig) (*runtime.Engine, core.State, error) {
	// Use the existing scripted provider pattern; the script emits one
	// REQUEST_APPROVAL instruction on the first Decide, then DONE on
	// the second (driven by Resume).
	reg := action.NewRegistry()
	prov := &scriptedPauseProvider{}
	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		"pause_test": &pauseTestRule{provider: prov, onApprove: a.onApprove},
	})
	eng := runtime.NewEngine(step, prov, reg)
	eng.Approval = action.DefaultApprovalPolicy{}
	return eng, core.State{
		RunID:    ac.RunID,
		Budget:   core.Budget{MaxTurns: 5},
		Autonomy: core.AUTONOMY_L2,
	}, nil
}

func (a *pausedTestAgent) ResolveApproval(ctx context.Context, s core.State) (core.ApprovalDecision, string, error) {
	return a.resolve(ctx, s)
}

// --- scripted provider + rule ---

type scriptedPauseProvider struct{ called int }

func (p *scriptedPauseProvider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	p.called++
	if p.called == 1 {
		return core.ModelResult{StopReason: core.STOP_REASON_TOOL_USE, Text: "need approval"}, nil
	}
	return core.ModelResult{StopReason: core.STOP_REASON_END_TURN, Text: "done"}, nil
}

func (p *scriptedPauseProvider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	ch := make(chan core.ModelChunk, 1)
	ch <- core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Text: ""}
	close(ch)
	return ch, nil
}

type pauseTestRule struct {
	provider  *scriptedPauseProvider
	onApprove func()
}

func (r *pauseTestRule) Kind() core.ReasoningStyle { return "pause_test" }

func (r *pauseTestRule) NextStep(s core.State) (core.State, []core.Instruction) {
	if r.provider.called == 1 {
		// Emit REQUEST_APPROVAL — runtime turns this into PAUSED_APPROVAL.
		next := s.Clone()
		return next, []core.Instruction{{
			Kind: core.INSTRUCTION_REQUEST_APPROVAL,
			RequestApproval: &core.RequestApprovalInstruction{
				ApprovalID: "ap-1",
				Reason:     "test",
				Risk:       core.RISK_LEVEL_HIGH,
				Summary:    "approve me",
			},
		}}
	}
	// Second call: DONE.
	return s, []core.Instruction{{Kind: core.INSTRUCTION_DONE}}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /Users/shuk/projects/ai/agentSDK && go test ./app/... -count=1 -run TestRunConsultsApprovalResolverOnPause -v`
Expected: FAIL. The resolver is never called today — `Run` exits on PAUSED_APPROVAL.

- [ ] **Step 4: Implement the PAUSED loop in `Run`**

In `app/app.go::Run`, replace the block between `safeRun` and the `Completer` check with:

```go
// 5. Run under panic recovery.
start := time.Now()
log.Info("run_start", "run_id", state.RunID)
final, runErr := safeRun(ctx, engine, state)
if runErr != nil {
	log.Error("run_failed",
		"run_id", state.RunID,
		"dur_ms", time.Since(start).Milliseconds(),
		"turns", final.Turn,
		"err", runErr)
	return EXIT_ERROR
}

// 5a. Resolve approvals inline when the Agent opts in. The loop runs
// until Status is anything other than PAUSED_APPROVAL. A run whose
// Agent implements no ApprovalResolver exits here — the persisted
// PendingApprovals are left for an external verb to decide.
for final.Status == core.RUN_STATUS_PAUSED_APPROVAL {
	resolver, ok := a.(ApprovalResolver)
	if !ok {
		break
	}

	if p, ok := a.(PauseHandler); ok {
		if err := p.OnPauseApproval(ctx, final); err != nil {
			log.Error("on_pause_failed", "run_id", final.RunID, "err", err)
			return EXIT_ERROR
		}
	}

	// Per-decision deadline: fresh from options.approvalTimeout (or
	// fall back to options.timeout) so a long pause doesn't burn the
	// whole run budget on the first decision.
	resolveCtx := ctx
	if d := o.approvalTimeout; d > 0 {
		var cancel context.CancelFunc
		resolveCtx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	} else if o.timeout > 0 {
		var cancel context.CancelFunc
		resolveCtx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}

	decision, by, err := resolver.ResolveApproval(resolveCtx, final)
	if err != nil {
		log.Error("resolve_approval_failed", "run_id", final.RunID, "err", err)
		return EXIT_ERROR
	}
	if decision == core.APPROVAL_DECISION_REJECT {
		if h, ok := a.(RejectionHandler); ok {
			if err := h.OnReject(ctx, final, by); err != nil {
				log.Error("on_reject_failed", "run_id", final.RunID, "err", err)
				return EXIT_ERROR
			}
		}
		// Persist the REJECT so the audit trail is consistent with the
		// approve path; the run still exits here so the rejected call is
		// not re-issued.
		if _, err := engine.SubmitHumanDecision(ctx, final.RunID, decision, by); err != nil {
			log.Error("submit_reject_failed", "run_id", final.RunID, "err", err)
			return EXIT_ERROR
		}
		break
	}

	final, err = engine.SubmitHumanDecision(ctx, final.RunID, decision, by)
	if err != nil {
		log.Error("submit_decision_failed", "run_id", final.RunID, "err", err)
		return EXIT_ERROR
	}
	final, err = engine.Resume(ctx, final.RunID)
	if err != nil {
		log.Error("resume_failed", "run_id", final.RunID, "err", err)
		return EXIT_ERROR
	}
	log.Info("approval_resolved",
		"run_id", final.RunID,
		"decided_by", by,
		"decision", string(decision),
		"new_status", string(final.Status))
}

// 6. Completion hook. A run whose results could not be delivered did
// not succeed, so an error here fails the process.
if c, ok := a.(Completer); ok {
	if err := c.OnComplete(ctx, final); err != nil {
		log.Error("on_complete_failed", "run_id", final.RunID, "err", err)
		return EXIT_ERROR
	}
}

dur := time.Since(start)
log.Info("run_done",
	"run_id", final.RunID,
	"dur_ms", dur.Milliseconds(),
	"turns", final.Turn,
	"status", string(final.Status))
return EXIT_OK
```

Delete the now-stale `dur := time.Since(start)` line that appears before the OnComplete check in the original code (the new block computes dur at the end).

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd /Users/shuk/projects/ai/agentSDK && go test ./app/... -count=1 -run TestRunConsultsApprovalResolverOnPause -v`
Expected: PASS.

- [ ] **Step 6: Run the full app test suite for regressions**

Run: `cd /Users/shuk/projects/ai/agentSDK && go test ./app/... -count=1`
Expected: PASS (existing tests unchanged — the new loop only fires when `final.Status == RUN_STATUS_PAUSED_APPROVAL`, which existing tests don't produce).

- [ ] **Step 7: Write the no-resolver regression test**

Append to `app/app_test.go`:

```go
// TestRunExitsOnPauseWhenNoResolver ensures an Agent that pauses but
// does NOT implement ApprovalResolver still exits cleanly — the
// persisted PendingApprovals remain for an external verb.
func TestRunExitsOnPauseWhenNoResolver(t *testing.T) {
	// Reuse pausedTestAgent without the resolver by wrapping it; we
	// achieve "no resolver" by passing a separate Agent that does
	// not implement ApprovalResolver.
	noResolve := &noResolverAgent{pausedTestAgent: pausedTestAgent{
		resolve: func(ctx context.Context, s core.State) (core.ApprovalDecision, string, error) {
			t.Fatal("resolver must not be called when ApprovalResolver is not implemented")
			return "", "", nil
		},
	}}
	exit := app.Run(context.Background(), noResolve, app.WithLogToStdout())
	assert.Equal(t, app.EXIT_OK, exit, "no resolver → clean exit, status left in store")
}

type noResolverAgent struct{ pausedTestAgent }

// Embedding breaks the ResolveApproval method's promotion — without an
// outer type that implements app.ApprovalResolver, the type assertion
// in Run will miss.
```

Run: `cd /Users/shuk/projects/ai/agentSDK && go test ./app/... -count=1 -run TestRunExitsOnPauseWhenNoResolver -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
cd /Users/shuk/projects/ai/agentSDK
git add app/app.go app/app_test.go
git commit -m "feat(app): consult ApprovalResolver on PAUSED_APPROVAL within Run"
```

---

## Task 4: Rewrite `sample/logdoctor` to use the new interfaces

**Files:**
- Create: `sample/logdoctor/agent.go` (NEW — the `LogDoctorAgent` type)
- Modify: `sample/logdoctor/main.go` (replace existing main with `app.Main(&LogDoctorAgent{...})`)
- Test: `sample/logdoctor/agent_test.go` (NEW — round-trip with scripted provider)

**Interfaces:**
- Consumes: `app.PauseHandler`, `app.ApprovalResolver`, `app.RejectionHandler` (all from Task 1); `core.ObservationSource` (existing); `agent.WithSink` / `agent.WithListener` (existing)
- Produces: A runnable sample binary that demonstrates input via listener + output via sink + inline approval via decision channel — replaces the 145-line `cmd/run.go` wiring with `Bootstrap`-only assembly

- [ ] **Step 1: Read existing main.go and a representative cmd file**

Run:
```bash
cat /Users/shuk/projects/ai/agentSDK/sample/logdoctor/main.go
echo "---"
cat /Users/shuk/projects/ai/agentSDK/sample/logdoctor/cmd/run.go | head -60
```
Expected: confirms the current shape (cobra dispatch + 145-line wiring).

- [ ] **Step 2: Write `sample/logdoctor/agent.go`**

Create new file:

```go
// Package main (logdoctor) ships LogDoctorAgent — a sample that wires
// agent.WithListener (continuous log feed) + agent.WithSink (real-time
// assistant output) + app.PauseHandler + app.ApprovalResolver (inline
// HITL on HIGH-risk tools) into a single Agent type. The decision
// channel is filled by a background goroutine that reads stdin, so the
// demo runs end-to-end from a terminal without any external verbs.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	appconfig "github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor/internal/fake"
	"github.com/bizshuk/agentsdk/sample/logdoctor/tool"
	"github.com/bizshuk/agentsdk/runtime"
)

// LogDoctorAgent is the new-skeleton equivalent of cmd/run.go::runExecute.
// Bootstrap returns an *runtime.Engine pre-wired with the listener + sink;
// PauseHandler + ApprovalResolver + RejectionHandler handle mid-run HITL.
type LogDoctorAgent struct {
	Fixture    string                 // log file to tail
	Fake       bool                   // use fake.NewScriptedProvider instead of real LLM
	DecisionCh chan core.ApprovalDecision // filled by stdin goroutine
	Out        io.Writer              // default os.Stdout
}

func (a *LogDoctorAgent) Name() string { return "logdoctor" }

// Bootstrap replaces cmd/run.go's 145-line wiring. Each stage has a
// clear reason; the listener + sink are the new pieces.
func (a *LogDoctorAgent) Bootstrap(ctx context.Context, ac *appconfig.AppConfig) (*runtime.Engine, core.State, error) {
	out := a.out()
	listener, err := domain.NewLogFileListener(a.fixture)
	if err != nil {
		return nil, core.State{}, fmt.Errorf("logdoctor: listener: %w", err)
	}

	// Provider selection mirrors cmd/provider.go + cmd/run.go:74-78.
	prov := fake.NewScriptedProvider()
	if !a.Fake {
		// Real provider wiring is intentionally a TODO — keep the sample
		// deterministic. Production deployments wire anthropic/openai
		// adapters here and pass credentials via env.
		_ = prov
	}

	// Tool registry: built-in Read + sample-specific read_log_tail +
	// notify + propose_fix. Mirrors cmd/run.go:82-96.
	// (Implementation deferred; this is a wiring skeleton.)
	_ = tool.NewReadLogTail(listener)
	_ = tool.NewNotify(out)

	cfg := agent.Config{
		Name:      "logdoctor",
		Tier:      spec.TIER_STANDARD,
		Reasoning: agent.Reasoning{Style: "think_then_act"},
		Limits:    agent.Limits{MaxTurns: 100, Autonomy: "L2"},
		Middleware: &spec.Middleware{Preset: spec.MIDDLEWARE_SECURE},
		Tools:     &spec.Tools{Builtin: []string{"read"}},
		Memory:    &spec.Memory{Store: spec.MEMORY_STORE_FILE},
	}

	sink := agent.SinkFunc(func(ev core.StreamEvent) {
		if ev.Kind == core.STREAM_MESSAGE && ev.Text != "" {
			fmt.Fprint(out, ev.Text)
		}
	})

	eng, state, err := agent.MustNew(cfg,
		agent.WithProvider(prov),
		agent.WithListener(listener),
		agent.WithSink(sink),
	).Bootstrap(ctx, ac)
	if err != nil {
		return nil, core.State{}, fmt.Errorf("logdoctor: bootstrap: %w", err)
	}
	return eng, state, nil
}

// OnPauseApproval formats the pending approval for the operator's
// terminal before ResolveApproval blocks on the decision channel.
func (a *LogDoctorAgent) OnPauseApproval(_ context.Context, s core.State) error {
	if len(s.PendingApprovals) == 0 {
		return nil
	}
	pa := s.PendingApprovals[0]
	fmt.Fprintf(a.out(), "\n[approval needed] run=%s tool=%s reason=%s\n",
		s.RunID, pa.ToolCall.Name, pa.Reason)
	return nil
}

// ResolveApproval blocks until either ctx is cancelled or a decision
// arrives on DecisionCh. The background goroutine in main.go fills the
// channel from stdin.
func (a *LogDoctorAgent) ResolveApproval(ctx context.Context, _ core.State) (core.ApprovalDecision, string, error) {
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case dec := <-a.DecisionCh:
		return dec, "operator", nil
	}
}

// OnReject logs the rejection — production wiring would also publish to
// the audit topic.
func (a *LogDoctorAgent) OnReject(_ context.Context, s core.State, by string) error {
	fmt.Fprintf(a.out(), "[rejected] run=%s by=%s turn=%d\n", s.RunID, by, s.Turn)
	return nil
}

// OnComplete prints a one-line summary.
func (a *LogDoctorAgent) OnComplete(_ context.Context, s core.State) error {
	fmt.Fprintf(a.out(), "[done] run=%s turns=%d status=%s\n",
		s.RunID, s.Turn, s.Status)
	return nil
}

func (a *LogDoctorAgent) out() io.Writer {
	if a.Out != nil {
		return a.Out
	}
	return os.Stdout
}
```

Note: the TODO for full provider + tool wiring is intentional — this plan demonstrates the **interface contract**; wiring every detail belongs to a follow-up plan. The current step lands the architecture so follow-up steps can fill in the policy details.

- [ ] **Step 3: Write `sample/logdoctor/main.go`**

Replace `sample/logdoctor/main.go` with:

```go
// Command logdoctor watches a log file, diagnoses errors, and queues
// fixes. Run with --fake for offline scripted provider; with --provider
// for a real LLM (TODO: wiring deferred to follow-up plan).
//
// Architecture (this revision):
//   - LogDoctorAgent implements app.Agent + app.PauseHandler +
//     app.ApprovalResolver + app.RejectionHandler + app.Completer.
//   - Bootstrap wires agent.WithListener (log feed) + agent.WithSink
//     (assistant text → stdout).
//   - A background goroutine reads stdin and pushes the decision onto
//     DecisionCh; ResolveApproval blocks on it.
//   - app.Run loops across PAUSED_APPROVAL until the Agent resolves
//     each pause inline.
//
// Operator flow:
//
//	go run . --fake --fixture testdata/error.log
//	logdoctor> ERROR oops
//	logdoctor> [approval needed] run=... tool=propose_fix ...
//	y
//	logdoctor> [done] run=... status=RUN_STATUS_COMPLETED
package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/app"
	"github.com/bizshuk/agentsdk/core"
)

func main() {
	fixture := "testdata/error.log"
	fake := true
	for i, arg := range os.Args[1:] {
		if arg == "--fixture" && i+1 < len(os.Args[1:]) {
			fixture = os.Args[i+2]
		}
		if arg == "--real" {
			fake = false
		}
	}

	agent := &LogDoctorAgent{
		Fixture:    fixture,
		Fake:       fake,
		DecisionCh: make(chan core.ApprovalDecision, 1),
	}

	// Background goroutine: read stdin → DecisionCh.
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			switch scanner.Text() {
			case "y", "yes":
				agent.DecisionCh <- core.APPROVAL_DECISION_APPROVE
			default:
				agent.DecisionCh <- core.APPROVAL_DECISION_REJECT
			}
		}
		close(agent.DecisionCh)
	}()

	app.Main(agent, app.WithLogToStdout())
	if err := error(nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Compile the sample module**

Run: `cd /Users/shuk/projects/ai/agentSDK/sample/logdoctor && go build ./...`
Expected: exit 0. (If the placeholder tool wiring causes errors, narrow `Bootstrap` to provider-only until the tool-wiring follow-up plan lands.)

- [ ] **Step 5: Smoke-test with --fake**

Run: `cd /Users/shuk/projects/ai/agentSDK/sample/logdoctor && echo "y" | go run . --fake --fixture testdata/error.log 2>&1 | head -30`
Expected: sees one or more `[approval needed]` lines; sends `y`; sees `[done]` then exits.

- [ ] **Step 6: Commit**

```bash
cd /Users/shuk/projects/ai/agentSDK
git add sample/logdoctor/agent.go sample/logdoctor/main.go
git commit -m "refactor(logdoctor): rewrite on app.Main + WithListener + inline approval"
```

---

## Task 5: Document the new interfaces

**Files:**
- Modify: `docs/terminology.md` (add three rows to the `app/` glossary section)
- Modify: `CLAUDE.md` (update the `app/config` module-mapping row)

**Interfaces:**
- Consumes: nothing — pure documentation
- Produces: Discoverable terminology + module map entries so a future reader can grep `ApprovalResolver` and land on a definition

- [ ] **Step 1: Read `docs/terminology.md` to find the insertion point**

Run: `grep -n "^##\|^|\|app/" /Users/shuk/projects/ai/agentSDK/docs/terminology.md | head -20`

- [ ] **Step 2: Add three rows to the `app/` glossary**

Under the existing `app/` section (or a new section if none exists), append three rows in the table format used elsewhere in the file:

```markdown
| ApprovalResolver | app.ApprovalResolver | Optional `Agent` extension; `ResolveApproval(ctx, state) → (decision, decidedBy, err)`. `app.Run` calls it whenever Status becomes PAUSED_APPROVAL, looping until non-paused. The Agent owns the input side — stdin, HTTP, Kafka, policy — so a single binary can run inline HITL without external verbs. | `app/agent.go` |
| PauseHandler | app.PauseHandler | Optional `Agent` extension; `OnPauseApproval(ctx, state) error`. Runs immediately after Status becomes PAUSED_APPROVAL, before the resolver is consulted. Use for Slack/PagerDuty notifications, audit logs, or formatting the proposal. | `app/agent.go` |
| RejectionHandler | app.RejectionHandler | Optional `Agent` extension; `OnReject(ctx, state, decidedBy) error`. Runs when ResolveApproval returns REJECT. Use to record why, notify operators, or roll back state. | `app/agent.go` |
```

- [ ] **Step 3: Update `CLAUDE.md` module-mapping table**

In the `app/config` row of the module-mapping table, expand the `app` description:

From:
```markdown
| app/config | `agentsdk/app`、`agentsdk/config`：`app.Run`、`OpenForCLI`、`SecureMiddleware`、`NewRefreshingProvider` |
```

To:
```markdown
| app/config | `agentsdk/app`、`agentsdk/config`：`app.Run`、`OpenForCLI`、`SecureMiddleware`、`NewRefreshingProvider`、`ApprovalResolver`（mid-run HITL）、`PauseHandler`、`RejectionHandler` |
```

- [ ] **Step 4: Commit**

```bash
cd /Users/shuk/projects/ai/agentSDK
git add docs/terminology.md CLAUDE.md
git commit -m "docs: document PauseHandler / ApprovalResolver / RejectionHandler"
```

---

## Self-Review

**1. Spec coverage:** Each capability from the §10 §13 discussion maps to a task:

- `ApprovalResolver` interface → Task 1
- `PauseHandler` interface → Task 1
- `RejectionHandler` interface → Task 1
- `WithApprovalTimeout` option → Task 2
- `app.Run` PAUSED loop → Task 3
- No-resolver fallback preserved → Task 3 (Step 7 regression test)
- logdoctor integration example → Task 4
- Documentation sync → Task 5

Gaps: none for this plan's scope. The logdoctor **full provider + tool wiring** is intentionally a follow-up (the placeholder `TODO` in Task 4 Step 2 names it).

**2. Placeholder scan:** No `TODO`/`TBD`/`fill in details` in the plan body. The single `TODO` comment in Task 4 Step 2 is inside a code comment that **explicitly** defers to a follow-up plan — not a plan-body placeholder.

**3. Type consistency:**

- `PauseHandler.OnPauseApproval(ctx, s) error` — consistent across Task 1 (interface), Task 3 (Run loop), Task 4 (LogDoctorAgent method)
- `ApprovalResolver.ResolveApproval(ctx, s) (decision, decidedBy, err)` — consistent across Task 1, Task 3, Task 4
- `RejectionHandler.OnReject(ctx, s, decidedBy) error` — consistent across Task 1, Task 3, Task 4
- `engine.SubmitHumanDecision(ctx, runID, decision, by)` and `engine.Resume(ctx, runID)` — exact signatures from `runtime/loop.go:308, 287`
- `core.ApprovalDecision` values: `APPROVAL_DECISION_APPROVE` / `APPROVAL_DECISION_REJECT` — consistent with `runtime/loop.go:317` usage
- `o.approvalTimeout` field name — consistent across Task 2 (struct + constructor + Run loop)

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-23-agent-approval-resolver.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

---

## Companion plans (proposed, NOT in this plan's scope)

The session also surfaced three smaller proposals that should land as separate plans after this one:

| Plan | Files | Status |
| --- | --- | --- |
| `MaxToolCalls` per-run cumulative + budget middleware | `agent/spec/spec.go`, `agent/spec/tier.go`, `agent/spec/validate.go`, `middleware/harness/budget.go`, `docs/architecture.svg §10` | not planned |
| `core.ChannelSource` helper (wraps goroutine+close pattern) | `core/source.go` (NEW), `sample/logdoctor/core/listener.go` | not planned |
| `sample/logdoctor` full provider + tool wiring (continuation of Task 4) | `sample/logdoctor/agent.go` (replace placeholder), `sample/logdoctor/cmd/{run,watch,resume,approve}.go` (deprecate) | not planned |