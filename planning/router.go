package planning

import "github.com/bizshuk/agentsdk/core"

// Router: multi-agent router pattern.
//
// STUB: returns DONE with a notification. Implementation deferred.
type Router struct{}

// NewRouter returns the stub pattern.
func NewRouter() *Router { return &Router{} }

// Kind returns THINK_ROUTER.
func (p *Router) Kind() core.ThinkingKind { return core.THINK_ROUTER }

// Decide emits DONE with a NOTIFY explaining the stub state.
func (p *Router) Decide(state core.State) (core.State, []core.Effect) {
	next := state.Clone()
	next.UpdatedAt = nowOrZero(state)
	return next, []core.Effect{
		{Kind: core.EFFECT_NOTIFY, Notify: &core.NotifyEffect{
			Level:   "warn",
			Message: "router pattern is a STUB; emitting DONE",
		}},
		doneEffect(),
	}
}
