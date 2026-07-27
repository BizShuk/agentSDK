// Package testutil holds testing-only helpers shared across the agentsdk
// module and samples. Nothing here may be imported by production code.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// ScriptedProvider is a deterministic, scripted Provider.
//
// Tests queue up an ordered list of ModelResults; Generate / Stream
// consume them in order. When the queue is empty, Generate returns
// ErrQueueEmpty. This lets tests reproduce arbitrary transcript shapes
// (end_turn / tool_use / etc.) without any network access.
type ScriptedProvider struct {
	mu      sync.Mutex
	queue   []core.ModelResult
	calls   int
	streams int
	lastReq core.ModelRequest
	// OnRequest is an optional side-effect hook fired before returning
	// the head of the queue. Useful for capturing the request that
	// triggered a particular scripted response.
	OnRequest func(req core.ModelRequest)
}

// NewScriptedProvider constructs an empty queue.
func NewScriptedProvider() *ScriptedProvider { return &ScriptedProvider{} }

// Enqueue appends scripted results.
func (s *ScriptedProvider) Enqueue(rs ...core.ModelResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, rs...)
}

// EnqueueToolCall is sugar for a single tool_use response.
func (s *ScriptedProvider) EnqueueToolCall(id, name string, args map[string]any) {
	s.Enqueue(core.ModelResult{
		StopReason: "tool_use",
		ToolCalls:  []core.ToolCall{{ID: id, Name: name, Args: args}},
	})
}

// EnqueueToolCalls queues one response carrying a whole batch — the
// shape every frontier model emits when it wants parallel operations,
// and the one the engine has to settle 1:1 with tool_result messages.
func (s *ScriptedProvider) EnqueueToolCalls(calls ...core.ToolCall) {
	s.Enqueue(core.ModelResult{
		StopReason: "tool_use",
		ToolCalls:  calls,
	})
}

// EnqueueEndTurn is sugar for a final assistant message.
func (s *ScriptedProvider) EnqueueEndTurn(text string) {
	s.Enqueue(core.ModelResult{
		StopReason: "end_turn",
		Text:       text,
	})
}

// Generate pops the next scripted result. Errors if the queue is empty.
func (s *ScriptedProvider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	s.mu.Lock()
	s.lastReq = req
	if s.OnRequest != nil {
		s.OnRequest(req)
	}
	if len(s.queue) == 0 {
		s.mu.Unlock()
		return core.ModelResult{}, ErrQueueEmpty
	}
	r := s.queue[0]
	s.queue = s.queue[1:]
	s.calls++
	s.mu.Unlock()
	return r, nil
}

// Stream emits the head of the queue as a single chunked result.
func (s *ScriptedProvider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	s.mu.Lock()
	s.lastReq = req
	if len(s.queue) == 0 {
		s.mu.Unlock()
		return nil, ErrQueueEmpty
	}
	r := s.queue[0]
	s.queue = s.queue[1:]
	s.calls++
	s.streams++
	s.mu.Unlock()
	ch := make(chan core.ModelChunk, 1)
	defer close(ch)
	ch <- core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Text: r.Text, Done: true}
	return ch, nil
}

// LastRequest returns the most recent request the provider received —
// the seam for asserting what a composition layer actually sent (system
// message ordering, tool specs, model id).
func (s *ScriptedProvider) LastRequest() core.ModelRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastReq
}

// RequestCount returns how many times Generate was called.
func (s *ScriptedProvider) RequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// ErrQueueEmpty is returned when Generate is called with no scripted results left.
var ErrQueueEmpty = errors.New("scripted provider queue empty")

// Ensure error type can be compared via errors.Is in tests.
func init() {
	if !errors.Is(ErrQueueEmpty, ErrQueueEmpty) {
		panic(fmt.Sprintf("testutil: %v", ErrQueueEmpty))
	}
}
