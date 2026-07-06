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

// FakeProvider is a deterministic, scripted ModelProvider.
//
// Tests queue up an ordered list of ModelResults; Generate / Stream
// consume them in order. When the queue is empty, Generate returns
// ErrQueueEmpty. This lets tests reproduce arbitrary transcript shapes
// (end_turn / tool_use / etc.) without any network access.
type FakeProvider struct {
	mu      sync.Mutex
	queue   []core.ModelResult
	calls   int
	streams int
	// OnGenerate is an optional side-effect hook fired before returning
	// the head of the queue. Useful for capturing the request that
	// triggered a particular scripted response.
	OnGenerate func(req core.ModelRequest)
}

// NewFakeProvider constructs an empty queue.
func NewFakeProvider() *FakeProvider { return &FakeProvider{} }

// Enqueue appends scripted results.
func (f *FakeProvider) Enqueue(rs ...core.ModelResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, rs...)
}

// EnqueueToolCall is sugar for a single tool_use response.
func (f *FakeProvider) EnqueueToolCall(id, name string, args map[string]any) {
	f.Enqueue(core.ModelResult{
		StopReason: "tool_use",
		ToolCalls:  []core.ToolCall{{ID: id, Name: name, Args: args}},
	})
}

// EnqueueEndTurn is sugar for a final assistant message.
func (f *FakeProvider) EnqueueEndTurn(text string) {
	f.Enqueue(core.ModelResult{
		StopReason: "end_turn",
		Text:       text,
	})
}

// Name implements core.ModelProvider.
func (f *FakeProvider) Name() string { return "fake" }

// Generate pops the next scripted result. Errors if the queue is empty.
func (f *FakeProvider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	f.mu.Lock()
	if f.OnGenerate != nil {
		f.OnGenerate(req)
	}
	if len(f.queue) == 0 {
		f.mu.Unlock()
		return core.ModelResult{}, ErrQueueEmpty
	}
	r := f.queue[0]
	f.queue = f.queue[1:]
	f.calls++
	f.mu.Unlock()
	return r, nil
}

// Stream emits the head of the queue as a single chunked result.
func (f *FakeProvider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	f.mu.Lock()
	if len(f.queue) == 0 {
		f.mu.Unlock()
		return nil, ErrQueueEmpty
	}
	r := f.queue[0]
	f.queue = f.queue[1:]
	f.calls++
	f.streams++
	f.mu.Unlock()
	ch := make(chan core.ModelChunk, 1)
	defer close(ch)
	ch <- core.ModelChunk{Kind: core.CHUNK_KIND_TEXT, Text: r.Text, Done: true}
	return ch, nil
}

// CountTokens returns 1 per message — good enough for tests that don't
// assert on token usage.
func (f *FakeProvider) CountTokens(ctx context.Context, msgs []core.Message) (int, error) {
	return len(msgs), nil
}

// CallCount returns how many times Generate was called.
func (f *FakeProvider) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// ErrQueueEmpty is returned when Generate is called with no scripted results left.
var ErrQueueEmpty = errors.New("fake provider queue empty")

// Ensure error type can be compared via errors.Is in tests.
func init() {
	if !errors.Is(ErrQueueEmpty, ErrQueueEmpty) {
		panic(fmt.Sprintf("testutil: %v", ErrQueueEmpty))
	}
}