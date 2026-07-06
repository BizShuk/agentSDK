// Package perception is the input side of the agent loop: external sources
// emit Percepts, which the runtime folds into Inputs and feeds to Step.
package perception

import (
	"context"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// Source emits percepts on a channel. The runtime reads until ctx is done
// or the channel is closed. Implementations live in sample/* (e.g. log
// tailer) or domain adapters.
type Source interface {
	// Name is a stable identifier for diagnostics ("logfile:/var/log/sys").
	Name() string
	// Percepts returns a channel. Closing the channel signals "no more for now".
	Percepts(ctx context.Context) <-chan core.Percept
}

// Multi fans several Sources into one channel. Used when one agent watches
// several streams. Order is best-effort: each goroutine pushes as data
// arrives. Determinism is not promised across sources.
type Multi struct {
	Sources []Source
}

// Percepts implements Source.
//
// Closes the returned channel once every source has finished (its own
// channel was closed). Order is best-effort — each source goroutine pushes
// as data arrives; cross-source order is non-deterministic.
func (m *Multi) Percepts(ctx context.Context) <-chan core.Percept {
	out := make(chan core.Percept, 32)
	if len(m.Sources) == 0 {
		close(out)
		return out
	}
	var wg sync.WaitGroup
	for _, s := range m.Sources {
		s := s
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := s.Percepts(ctx)
			for {
				select {
				case <-ctx.Done():
					return
				case p, ok := <-ch:
					if !ok {
						return
					}
					select {
					case out <- p:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// Name returns "multi" for diagnostics.
func (m *Multi) Name() string { return "multi" }