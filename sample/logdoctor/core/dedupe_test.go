package core_test

import (
	"context"
	"sync"
	"testing"
	"time"

	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor/core"
	"github.com/stretchr/testify/assert"
)

// stubPerceptSource emits the given percepts in order then closes.
type stubPerceptSource struct {
	mu    sync.Mutex
	out   []sdkcore.Percept
	emitN int
}

func (s *stubPerceptSource) Name() string { return "stub" }
func (s *stubPerceptSource) Percepts(ctx context.Context) <-chan sdkcore.Percept {
	ch := make(chan sdkcore.Percept, len(s.out))
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
	src := &stubPerceptSource{out: []sdkcore.Percept{
		{Payload: "ERROR: oom"},
		{Payload: "ERROR: oom"}, // same fingerprint
		{Payload: "ERROR: oom"}, // same fingerprint
		{Payload: "WARN: latency"}, // different
	}}
	d := domain.NewDedupe(src, "logline", 1*time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got := make([]string, 0, 4)
	for p := range d.Percepts(ctx) {
		got = append(got, p.Payload.(string))
	}
	assert.Equal(t, []string{"ERROR: oom", "WARN: latency"}, got)
}

func TestDedupeFingerprintIsStable(t *testing.T) {
	a := domain.NewDedupe(&stubPerceptSource{}, "r1", time.Minute)
	p := sdkcore.Percept{Payload: "abc"}
	assert.True(t, a.ShouldEmitForTest(p))
	assert.False(t, a.ShouldEmitForTest(p), "second call within cooldown must be suppressed")
}

func TestDedupeDifferentRulePasses(t *testing.T) {
	src := &stubPerceptSource{out: []sdkcore.Percept{
		{Payload: "same text"},
	}}
	d := domain.NewDedupe(src, "rule-2", 1*time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	count := 0
	for range d.Percepts(ctx) {
		count++
	}
	assert.Equal(t, 1, count)
}