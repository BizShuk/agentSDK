package core_test

import (
	"context"
	"sync"
	"testing"
	"time"

	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/stretchr/testify/assert"
)

// stubObservationSource emits the given percepts in order then closes.
type stubObservationSource struct {
	mu    sync.Mutex
	out   []sdkcore.Observation
	emitN int
}

func (s *stubObservationSource) Name() string { return "stub" }
func (s *stubObservationSource) Observations(ctx context.Context) <-chan sdkcore.Observation {
	ch := make(chan sdkcore.Observation, len(s.out))
	go func() {
		defer close(ch)
		for _, p := range s.out {
			select {
			case <-ctx.Done():
				return
			case ch <- p:
			}
		}
	}()
	return ch
}

func TestDedupeDropsRepeatsWithinCooldown(t *testing.T) {
	src := &stubObservationSource{out: []sdkcore.Observation{
		{Payload: "ERROR: oom"},
		{Payload: "ERROR: oom"},    // same fingerprint
		{Payload: "ERROR: oom"},    // same fingerprint
		{Payload: "WARN: latency"}, // different
	}}
	d := domain.NewBurstSuppressor(src, "logline", 1*time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got := make([]string, 0, 4)
	for p := range d.Observations(ctx) {
		got = append(got, p.Payload.(string))
	}
	assert.Equal(t, []string{"ERROR: oom", "WARN: latency"}, got)
}

func TestDedupeFingerprintIsStable(t *testing.T) {
	a := domain.NewBurstSuppressor(&stubObservationSource{}, "r1", time.Minute)
	p := sdkcore.Observation{Payload: "abc"}
	assert.True(t, a.ShouldEmitForTest(p))
	assert.False(t, a.ShouldEmitForTest(p), "second call within cooldown must be suppressed")
}

func TestDedupeDifferentRulePasses(t *testing.T) {
	src := &stubObservationSource{out: []sdkcore.Observation{
		{Payload: "same text"},
	}}
	d := domain.NewBurstSuppressor(src, "rule-2", 1*time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	count := 0
	for range d.Observations(ctx) {
		count++
	}
	assert.Equal(t, 1, count)
}
