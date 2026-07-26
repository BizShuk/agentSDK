package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/tool"
	sdktool "github.com/bizshuk/agentsdk/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSource is a controllable Source for tool tests.
type stubSource struct {
	out []sdkcore.Observation
	ch  chan sdkcore.Observation
}

func newStubSource(out ...sdkcore.Observation) *stubSource {
	return &stubSource{out: out, ch: make(chan sdkcore.Observation, len(out))}
}

func (s *stubSource) Observations(ctx context.Context) <-chan sdkcore.Observation {
	go func() {
		defer close(s.ch)
		for _, p := range s.out {
			select {
			case <-ctx.Done():
				return
			case s.ch <- p:
			}
		}
	}()
	return s.ch
}

func TestReadLogTailExtractsFirstN(t *testing.T) {
	src := newStubSource(sdkcore.Observation{
		ObservedAt: time.Unix(0, 0),
		Payload:    "a\nb\nc\nd\ne",
	})
	reg := sdktool.NewRegistry()
	tool.NewReadLogTail(src).Register(reg)

	res := reg.Call(context.Background(), core.ToolCall{
		ID:   "c1",
		Name: "read_log_tail",
		Args: map[string]any{"n": 3},
	})
	assert.True(t, res.OK)

	outBytes, ok := res.Output.(json.RawMessage)
	require.True(t, ok)
	var out tool.ReadLogTailOutput
	require.NoError(t, json.Unmarshal(outBytes, &out))
	assert.Equal(t, []string{"a", "b", "c"}, out.Lines)
	assert.True(t, out.Truncated)
}

func TestReadLogTailDefaultsTo20(t *testing.T) {
	src := newStubSource(sdkcore.Observation{
		ObservedAt: time.Unix(0, 0),
		Payload:    "a\nb",
	})
	reg := sdktool.NewRegistry()
	tool.NewReadLogTail(src).Register(reg)

	res := reg.Call(context.Background(), core.ToolCall{
		ID:   "c1",
		Name: "read_log_tail",
		Args: map[string]any{},
	})
	assert.True(t, res.OK)
}

func TestReadLogTailBadArgs(t *testing.T) {
	src := newStubSource(sdkcore.Observation{Payload: "x"})
	reg := sdktool.NewRegistry()
	tool.NewReadLogTail(src).Register(reg)

	res := reg.Call(context.Background(), core.ToolCall{
		ID:   "c1",
		Name: "read_log_tail",
		Args: map[string]any{"n": "not a number"},
	})
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "invalid args")
}

func TestNotifyWritesLine(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	f, err := os.Create(out)
	require.NoError(t, err)
	defer f.Close()

	reg := sdktool.NewRegistry()
	tool.NewNotify(f).Register(reg)

	res := reg.Call(context.Background(), core.ToolCall{
		ID:   "c1",
		Name: "notify",
		Args: map[string]any{"level": "warn", "message": "disk full"},
	})
	assert.True(t, res.OK)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[notify][warn]")
	assert.Contains(t, string(data), "disk full")
}

func TestNotifyDefaultsToInfo(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	f, err := os.Create(out)
	require.NoError(t, err)
	defer f.Close()

	reg := sdktool.NewRegistry()
	tool.NewNotify(f).Register(reg)

	res := reg.Call(context.Background(), core.ToolCall{
		ID:   "c1",
		Name: "notify",
		Args: map[string]any{"message": "hi"},
	})
	assert.True(t, res.OK)

	data, _ := os.ReadFile(out)
	assert.Contains(t, string(data), "[notify][info]")
}

func TestNotifyErrorPropagates(t *testing.T) {
	fw := &failsWriter{}
	reg := sdktool.NewRegistry()
	tool.NewNotify(fw).Register(reg)

	res := reg.Call(context.Background(), core.ToolCall{
		ID:   "c1",
		Name: "notify",
		Args: map[string]any{"level": "warn", "message": "x"},
	})
	assert.True(t, res.OK)
}

type failsWriter struct{}

func (failsWriter) Write(_ []byte) (int, error) { return 0, errors.New("disk full") }

// smoke: listener + ReadLogTail together.
func TestListenerAndReadLogTailIntegrated(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.log")
	require.NoError(t, os.WriteFile(p, []byte("L1\nL2\nL3"), 0o600))

	l, err := domain.NewLogFileListener(p)
	require.NoError(t, err)

	reg := sdktool.NewRegistry()
	tool.NewReadLogTail(l).Register(reg)

	res := reg.Call(context.Background(), core.ToolCall{
		ID:   "c1",
		Name: "read_log_tail",
		Args: map[string]any{"n": 2},
	})
	assert.True(t, res.OK)
}
