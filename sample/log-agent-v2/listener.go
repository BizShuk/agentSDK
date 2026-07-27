package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// OBSERVATION_SOURCE identifies batches originating from app log files.
const OBSERVATION_SOURCE = "config-logs"

type batchListener struct {
	observation core.Observation
}

var _ core.ObservationSource = (*batchListener)(nil)

func newBatchListener(batch Batch, observedAt time.Time) (*batchListener, error) {
	if observedAt.IsZero() {
		return nil, fmt.Errorf("observation time must not be zero")
	}
	payload, err := formatBatch(batch)
	if err != nil {
		return nil, err
	}
	observedAt = observedAt.UTC()
	return &batchListener{
		observation: core.Observation{
			ID:         fmt.Sprintf("logs-%d", observedAt.UnixNano()),
			Source:     OBSERVATION_SOURCE,
			ObservedAt: observedAt,
			Payload:    payload,
		},
	}, nil
}

func (l *batchListener) Observations(ctx context.Context) <-chan core.Observation {
	out := make(chan core.Observation, 1)
	if ctx.Err() == nil {
		out <- l.observation
	}
	close(out)
	return out
}

func formatBatch(batch Batch) (string, error) {
	if err := validateBatchForObservation(batch); err != nil {
		return "", err
	}

	var prompt strings.Builder
	if _, err := fmt.Fprintf(
		&prompt,
		"Analyze this log batch. raw_bytes=%d backlog=%t sources=%d\n"+
			"<UNTRUSTED_LOG_DATA>\n",
		batch.Bytes,
		batch.Backlog,
		len(batch.Parts),
	); err != nil {
		return "", fmt.Errorf("write observation header: %w", err)
	}

	for _, part := range batch.Parts {
		sourceJSON, err := json.Marshal(part.Source)
		if err != nil {
			return "", fmt.Errorf("encode source %q: %w", part.Source, err)
		}
		contentJSON, err := json.Marshal(sanitizeLog(part.Content))
		if err != nil {
			return "", fmt.Errorf("encode content %q: %w", part.Source, err)
		}
		if _, err := fmt.Fprintf(
			&prompt,
			"source=%s offsets=%d-%d\ncontent_json=%s\n",
			sourceJSON,
			part.StartOffset,
			part.EndOffset,
			contentJSON,
		); err != nil {
			return "", fmt.Errorf("write source %q: %w", part.Source, err)
		}
	}
	prompt.WriteString("</UNTRUSTED_LOG_DATA>\n")
	return prompt.String(), nil
}

func validateBatchForObservation(batch Batch) error {
	if batch.Bytes <= 0 || len(batch.Parts) == 0 {
		return fmt.Errorf("log batch must contain bytes")
	}
	if batch.Bytes > MAX_BATCH_BYTES {
		return fmt.Errorf(
			"log batch has %d bytes, limit is %d",
			batch.Bytes,
			MAX_BATCH_BYTES,
		)
	}

	total := 0
	seen := make(map[string]struct{}, len(batch.Parts))
	for _, part := range batch.Parts {
		if !validSource(part.Source) {
			return fmt.Errorf("log source %q is invalid", part.Source)
		}
		if _, exists := seen[part.Source]; exists {
			return fmt.Errorf("log source %q is duplicated", part.Source)
		}
		seen[part.Source] = struct{}{}
		if len(part.Content) == 0 {
			return fmt.Errorf("log source %q has no content", part.Source)
		}
		if part.StartOffset < 0 ||
			part.EndOffset-part.StartOffset != int64(len(part.Content)) {
			return fmt.Errorf("log source %q has invalid offsets", part.Source)
		}
		total += len(part.Content)
	}
	if total != batch.Bytes {
		return fmt.Errorf("log batch content is %d bytes, metadata says %d", total, batch.Bytes)
	}
	return nil
}
