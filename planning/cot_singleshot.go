package planning

import "github.com/bizshuk/agentsdk/core"

// COTSingleshot is the one-shot chain-of-thought pattern.
//
// STUB: emits exactly one CALL_MODEL and DONE. Implementation deferred to
// a later milestone — goal here is interface compliance + no-panic.
type COTSingleshot struct{}

// NewCOTSingleshot returns the stub pattern.
func NewCOTSingleshot() *COTSingleshot { return &COTSingleshot{} }

// Kind returns THINK_COT_SINGLESHOT.
func (p *COTSingleshot) Kind() core.ThinkingKind { return core.THINK_COT_SINGLESHOT }

// Decide emits a single CALL_MODEL followed by DONE.
func (p *COTSingleshot) Decide(state core.State) (core.State, []core.Effect) {
	next := state.Clone()
	next.UpdatedAt = nowOrZero(state)
	return next, []core.Effect{
		callModelFromMessages(state.Clone()),
		doneEffect(),
	}
}
