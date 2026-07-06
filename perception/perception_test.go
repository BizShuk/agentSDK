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
	out  []core.Percept
}

func (s *testSource) Name() string                                  { return s.name }
func (s *testSource) Percepts(ctx context.Context) <-chan core.Percept {
	ch := make(chan core.Percept, len(s.out))
	for _, p := range s.out {
		ch <- p
	}
	close(ch)
	return ch
}

func TestMultiFanOut(t *testing.T) {
	a := &testSource{name: "a", out: []core.Percept{
		{ID: "1", Source: "a", ObservedAt: time.Unix(0, 0)},
		{ID: "2", Source: "a", ObservedAt: time.Unix(0, 0)},
	}}
	b := &testSource{name: "b", out: []core.Percept{
		{ID: "3", Source: "b", ObservedAt: time.Unix(0, 0)},
	}}
	multi := &perception.Multi{Sources: []perception.Source{a, b}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got := make(map[string]bool)
	for p := range multi.Percepts(ctx) {
		got[p.ID] = true
	}
	assert.Equal(t, 3, len(got))
	assert.True(t, got["1"])
	assert.True(t, got["2"])
	assert.True(t, got["3"])
}

func TestNormalizerDefault(t *testing.T) {
	n := &perception.Normalizer{}
	p := core.Percept{Payload: "hello", ObservedAt: time.Unix(0, 0)}
	m := n.Apply(p)
	require.Len(t, m.Chunks, 1)
	assert.Equal(t, "hello", m.Chunks[0].Text)
	assert.Equal(t, core.ROLE_USER, m.Role)
}

func TestNormalizerCustom(t *testing.T) {
	n := &perception.Normalizer{
		Fn: func(p core.Percept) core.Message {
			// pretend payload is a log event — render to ROLE_SYSTEM for context.
			return core.Message{
				Role: core.ROLE_SYSTEM,
				Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "[log] " + p.Payload.(string)}},
				Ts:    p.ObservedAt,
			}
		},
	}
	m := n.Apply(core.Percept{Payload: "FATAL: oom"})
	assert.Equal(t, core.ROLE_SYSTEM, m.Role)
	assert.Equal(t, "[log] FATAL: oom", m.Chunks[0].Text)
}