package core

import (
	"context"
	"time"
)

// Observation is one reading fed into the loop.
type Observation struct {
	ID         string    `json:"id"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
	Payload    any       `json:"payload"`
}

// ObservationSource supplies observations to a runtime.
type ObservationSource interface {
	Observations(ctx context.Context) <-chan Observation
}
