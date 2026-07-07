// Package perception is the input side of the agent loop: external sources
// emit Observations, which the runtime folds into Events and feeds to Decide.
//
// TODO(M3): this package is currently a root-level scaffolding with no
// consumer (perception_test.go is the only importer; core/runtime keep
// their own ObservationSource shim). Decide at M3 boundary whether to
// (a) wire runtime/loop.go to consume FanIn, (b) inline into a sample
// adapter, or (c) delete outright.
package perception

import (
	"context"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// ObservationSource emits observations on a channel. The runtime reads until
// ctx is done or the channel is closed. Implementations live in sample/*
// (e.g. log tailer) or domain adapters.
type ObservationSource interface {
	// Name is a stable identifier for diagnostics ("logfile:/var/log/sys").
	Name() string
	// Observations returns a channel. Closing the channel signals "no more for now".
	Observations(ctx context.Context) <-chan core.Observation
}

// FanIn merges several ObservationSources into one channel. Used when one
// agent watches several streams. Order is best-effort: each goroutine
// pushes as data arrives. Determinism is not promised across sources.
type FanIn struct {
	Sources []ObservationSource
}

// Observations implements ObservationSource.
//
// Closes the returned channel once every source has finished (its own
// channel was closed). Order is best-effort — each source goroutine
// pushes as data arrives; cross-source order is non-deterministic.
func (f *FanIn) Observations(ctx context.Context) <-chan core.Observation {
	out := make(chan core.Observation, 32)
	if len(f.Sources) == 0 {
		close(out)
		return out
	}
	var wg sync.WaitGroup
	for _, s := range f.Sources {
		s := s
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := s.Observations(ctx)
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

// Name returns "fan_in" for diagnostics.
func (f *FanIn) Name() string { return "fan_in" }
