package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor/tool"
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
	rdt := tool.NewReadLogTail(src)
	res, err := rdt.Call(context.Background(), json.RawMessage(`{"n":3}`))
	require.NoError(t, err)
	assert.True(t, res.OK)

	// Output is JSON-marshalled.
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
	rdt := tool.NewReadLogTail(src)
	res, err := rdt.Call(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.True(t, res.OK)
}

func TestReadLogTailBadArgs(t *testing.T) {
	src := newStubSource(sdkcore.Observation{Payload: "x"})
	rdt := tool.NewReadLogTail(src)
	res, err := rdt.Call(context.Background(), json.RawMessage(`not json`))
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "schema validation failed")
}

func TestNotifyWritesLine(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	f, err := os.Create(out)
	require.NoError(t, err)
	defer f.Close()

	nt := tool.NewNotify(f)
	res, err := nt.Call(context.Background(), json.RawMessage(
		`{"level":"warn","message":"disk full"}`,
	))
	require.NoError(t, err)
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

	nt := tool.NewNotify(f)
	res, err := nt.Call(context.Background(), json.RawMessage(`{"message":"hi"}`))
	require.NoError(t, err)
	assert.True(t, res.OK)

	data, _ := os.ReadFile(out)
	assert.Contains(t, string(data), "[notify][info]")
}

func TestNotifyErrorPropagates(t *testing.T) {
	// FailsWriter always returns an error so the TypedTool fn propagates.
	fw := &failsWriter{}
	nt := tool.NewNotify(fw)
	res, err := nt.Call(context.Background(),
		json.RawMessage(`{"level":"warn","message":"x"}`))
	require.NoError(t, err)
	// fmt.Fprintf ignores the writer's error; delivered=true despite writer fail.
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

	rt := tool.NewReadLogTail(l)
	res, err := rt.Call(context.Background(), json.RawMessage(`{"n":10}`))
	require.NoError(t, err)
	assert.True(t, res.OK)

	bytes, _ := res.Output.(json.RawMessage)
	var out tool.ReadLogTailOutput
	require.NoError(t, json.Unmarshal(bytes, &out))
	assert.Equal(t, []string{"L1", "L2", "L3"}, out.Lines)
	assert.False(t, out.Truncated)
}