// Package loopguard detects "stuck" runs where the same tool is being
// dispatched repeatedly with no new observation between calls.
//
// Strategy:
//
//   1. Fingerprint each CALL_TOOL effect by (tool name, args-minus-volatile).
//   2. Track consecutive repeats; reset counter when a non-CALL_TOOL
//      observation is seen (MODEL_RESULT with new content, TOOL_RESULT
//      with a new CallID, NOTIFY, etc.).
//   3. After MaxRepeats without progress, rewrite the next CALL_TOOL into
//      REQUEST_APPROVAL with Reason="loop_detected" so the run pauses for
//      human review.
//
// Volatile argument keys (e.g. "offset", "cursor", "page") are stripped
// before fingerprinting — otherwise the sample's tailing loop would
// itself trip the guard.
package loopguard

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// Config controls loop detection.
type Config struct {
	// MaxRepeats is the threshold for triggering an approval request.
	MaxRepeats int
	// VolatileKeys are arg keys stripped from the fingerprint. Default
	// includes "offset", "cursor", "page", "tail_n", "ts" — common
	// pagination / time markers that vary between calls without changing
	// the actual tool intent.
	VolatileKeys []string
}

// DefaultVolatileKeys are the keys stripped from the fingerprint by
// default. Sample tools (read_log_tail, etc.) use "n" as a tail cursor
// in M2; we do NOT strip it because the value carries intent (n=5 vs
// n=20 are different requests). Adjust per-tool if needed.
var DefaultVolatileKeys = []string{"offset", "cursor", "page", "since", "tail_offset"}

// State is the per-run tracking state. Persisted in scratch so the
// guard survives across Loop.Resume (and across middleware re-entry
// after REQUEST_APPROVAL → resume).
type State struct {
	LastFP   string // last fingerprint seen
	Repeats  int    // consecutive repeats of LastFP with no progress
	LastObs  string // last non-CALL_TOOL observation signature
	Triggered bool  // guard has fired this run; remain armed after approval
}

// New constructs the loopguard Middleware.
//
// State is held in scratch under LOOPGUARD_STATE_KEY so the runtime's
// preStep scratch seeding can carry it across iterations without
// middleware keeping its own map.
func New(cfg Config) middleware.Middleware {
	if cfg.MaxRepeats <= 0 {
		cfg.MaxRepeats = 5
	}
	if cfg.VolatileKeys == nil {
		cfg.VolatileKeys = DefaultVolatileKeys
	}
	volatile := make(map[string]struct{}, len(cfg.VolatileKeys))
	for _, k := range cfg.VolatileKeys {
		volatile[k] = struct{}{}
	}
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
			gs := loadState(state.Scratch)

			// Anything that is NOT a CALL_TOOL counts as a fresh
			// observation — reset the repeat counter.
			if eff.Kind != core.EFFECT_CALL_TOOL {
				gs.LastObs = obsSignature(eff)
				gs.LastFP = ""
				gs.Repeats = 0
				saveState(&state, gs)
				return next(ctx, state, eff)
			}

			if eff.CallTool == nil {
				return next(ctx, state, eff)
			}

			fp := fingerprint(eff.CallTool.Call.Name, eff.CallTool.Call.Args, volatile)

			if gs.LastFP == fp && !gs.Triggered {
				gs.Repeats++
			} else if gs.LastFP != fp {
				gs.LastFP = fp
				gs.Repeats = 1
			}

			if gs.Repeats >= cfg.MaxRepeats && !gs.Triggered {
				// Rewrite to REQUEST_APPROVAL so the run pauses for human review.
				gs.Triggered = true
				saveState(&state, gs)
				newEff := core.Effect{
					Kind: core.EFFECT_REQUEST_APPROVAL,
					RequestApproval: &core.RequestApprovalEffect{
						ApprovalID: "loop-detected-" + fp[:6],
						Reason:     "loop_detected",
						Risk:       eff.CallTool.Call.Risk,
						Summary:    "loopguard: " + eff.CallTool.Call.Name + " dispatched " + itoa(gs.Repeats) + " times with no new observation",
						ToolCall:   &eff.CallTool.Call,
					},
				}
				return next(ctx, state, newEff)
			}
			saveState(&state, gs)
			return next(ctx, state, eff)
		}
	}
}

const LOOPGUARD_STATE_KEY = "loopguard.state"

// loadState retrieves the per-run tracking state from scratch.
func loadState(scratch map[string]any) State {
	if scratch == nil {
		return State{}
	}
	v, ok := scratch[LOOPGUARD_STATE_KEY]
	if !ok {
		return State{}
	}
	if gs, ok := v.(State); ok {
		return gs
	}
	return State{}
}

func saveState(state *core.State, gs State) {
	if state.Scratch == nil {
		state.Scratch = make(map[string]any, 4)
	}
	state.Scratch[LOOPGUARD_STATE_KEY] = gs
}

// fingerprint produces a stable hash of (tool name, args-minus-volatile).
// We deliberately use sha1 + hex prefix (cheap, no deps), matching the
// sample/logdoctor dedupe convention.
func fingerprint(name string, args map[string]any, volatile map[string]struct{}) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		if _, isVolatile := volatile[k]; isVolatile {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('\n')
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(toStable(args[k]))
		b.WriteByte('\n')
	}
	sum := sha1.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// toStable renders an arbitrary JSON value into a canonical string.
// Maps iterate in sorted-key order; arrays preserve order.
func toStable(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return ftoa(x)
	case int:
		return itoa(x)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(k)
			b.WriteByte(':')
			b.WriteString(toStable(x[k]))
		}
		b.WriteByte('}')
		return b.String()
	case []any:
		var b strings.Builder
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(toStable(e))
		}
		b.WriteByte(']')
		return b.String()
	}
	return ""
}

// obsSignature renders the "observation" form of an effect so the guard
// can detect progress (a different observation means the prior tool call
// has been answered).
func obsSignature(eff core.Effect) string {
	switch eff.Kind {
	case core.EFFECT_CALL_MODEL:
		if eff.CallModel != nil {
			return "model:" + eff.CallModel.RequestID
		}
	case core.EFFECT_CALL_TOOL:
		// Not normally reached (caller filters), but for safety.
		if eff.CallTool != nil {
			return "tool:" + eff.CallTool.Call.ID
		}
	case core.EFFECT_NOTIFY:
		if eff.Notify != nil {
			return "notify:" + eff.Notify.Message
		}
	case core.EFFECT_DONE:
		return "done"
	case core.EFFECT_REQUEST_APPROVAL:
		if eff.RequestApproval != nil {
			return "approval:" + eff.RequestApproval.ApprovalID
		}
	}
	return string(eff.Kind)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func ftoa(f float64) string {
	// Use a stable format that does not depend on locale. Avoid strconv
	// for portability — keep the helper independent of stdlib for tests.
	return itoa(int(f))
}