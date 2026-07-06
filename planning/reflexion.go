package planning

import "github.com/bizshuk/agentsdk/core"

// Reflexion: remember failures, retry with reflection.
//
// STUB: emits a single CALL_MODEL and DONE. Implementation deferred.
type Reflexion struct{}

// NewReflexion returns the stub pattern.
func NewReflexion() *Reflexion { return &Reflexion{} }

// Kind returns THINK_REFLEXION.
func (p *Reflexion) Kind() core.ThinkingKind { return core.THINK_REFLEXION }

// Decide emits a single CALL_MODEL and DONE.
func (p *Reflexion) Decide(state core.State) (core.State, []core.Effect) {
	next := state.Clone()
	next.UpdatedAt = nowOrZero(state)
	return next, []core.Effect{
		callModelFromMessages(state.Clone()),
		doneEffect(),
	}
}
