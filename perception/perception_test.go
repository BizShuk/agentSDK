package perception_test

import (
	"context"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/perception"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testSource struct {
	name string
	out  []core.Observation
}

func (s *testSource) Name() string { return s.name }
func (s *testSource) Observations(ctx context.Context) <-chan core.Observation {
	ch := make(chan core.Observation, len(s.out))
	for _, p := range s.out {
		ch <- p
	}
	close(ch)
	return ch
}

func TestFanInMergesAllSources(t *testing.T) {
	a := &testSource{name: "a", out: []core.Observation{
		{ID: "1", Source: "a", ObservedAt: time.Unix(0, 0)},
		{ID: "2", Source: "a", ObservedAt: time.Unix(0, 0)},
	}}
	b := &testSource{name: "b", out: []core.Observation{
		{ID: "3", Source: "b", ObservedAt: time.Unix(0, 0)},
	}}
	multi := &perception.FanIn{Sources: []perception.ObservationSource{a, b}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got := make(map[string]bool)
	for p := range multi.Observations(ctx) {
		got[p.ID] = true
	}
	assert.Equal(t, 3, len(got))
	assert.True(t, got["1"])
	assert.True(t, got["2"])
	assert.True(t, got["3"])
}

func TestToMessageDefault(t *testing.T) {
	n := &perception.ToMessage{}
	p := core.Observation{Payload: "hello", ObservedAt: time.Unix(0, 0)}
	m := n.Apply(p)
	require.Len(t, m.Parts, 1)
	assert.Equal(t, "hello", m.Parts[0].Text)
	assert.Equal(t, core.ROLE_USER, m.Role)
}

func TestToMessageCustom(t *testing.T) {
	n := &perception.ToMessage{
		Fn: func(p core.Observation) core.Message {
			// pretend payload is a log event — render to ROLE_SYSTEM for context.
			return core.Message{
				Role: core.ROLE_SYSTEM,
				Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "[log] " + p.Payload.(string)}},
				Ts:    p.ObservedAt,
			}
		},
	}
	m := n.Apply(core.Observation{Payload: "FATAL: oom"})
	assert.Equal(t, core.ROLE_SYSTEM, m.Role)
	assert.Equal(t, "[log] FATAL: oom", m.Parts[0].Text)
}
