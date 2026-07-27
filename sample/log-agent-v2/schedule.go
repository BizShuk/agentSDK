package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var errScheduleClosed = errors.New("schedule tick channel closed")

type batchReader interface {
	Next(ctx context.Context) (Batch, []error, error)
}

var _ batchReader = (*Reader)(nil)

func readScheduledBatch(
	ctx context.Context,
	ticks <-chan time.Time,
	reader batchReader,
) (Batch, []error, error) {
	if ticks == nil {
		return Batch{}, nil, fmt.Errorf("schedule tick channel must not be nil")
	}
	if reader == nil {
		return Batch{}, nil, fmt.Errorf("scheduled batch reader must not be nil")
	}

	select {
	case <-ctx.Done():
		return Batch{}, nil, ctx.Err()
	case _, ok := <-ticks:
		if !ok {
			return Batch{}, nil, errScheduleClosed
		}
	}

	batch, warnings, err := reader.Next(ctx)
	if err != nil {
		return Batch{}, warnings, fmt.Errorf("read scheduled log batch: %w", err)
	}
	return batch, warnings, nil
}
