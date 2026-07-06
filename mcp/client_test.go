package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/mcp"
	mcppkg "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInMemoryServer spins up an in-process MCP server with the given
// tools, returning the client side of the InMemoryTransport pair.
func newInMemoryServer(t *testing.T, tools map[string]func(args map[string]any) (string, error)) *mcp.Client {
	t.Helper()
	server := mcppkg.NewServer(&mcppkg.Implementation{Name: "test", Version: "0.1"}, nil)
	for name, fn := range tools {
		name := name
		fn := fn
		mcppkg.AddTool(server, &mcppkg.Tool{Name: name, Description: "test " + name},
			func(_ context.Context, _ *mcppkg.CallToolRequest, args map[string]any) (*mcppkg.CallToolResult, any, error) {
				out, err := fn(args)
				if err != nil {
					return &mcppkg.CallToolResult{IsError: true, Content: []mcppkg.Content{
						&mcppkg.TextContent{Text: err.Error()},
					}}, nil, nil
				}
				return &mcppkg.CallToolResult{Content: []mcppkg.Content{
					&mcppkg.TextContent{Text: out},
				}}, nil, nil
			})
	}
	serverTransport, clientTransport := mcppkg.NewInMemoryTransports()

	// Server side
	go func() {
		_, _ = server.Connect(context.Background(), serverTransport, nil)
	}()
	session, err := mcppkg.NewClient(&mcppkg.Implementation{Name: "client", Version: "0.1"}, nil).Connect(
		context.Background(), clientTransport, nil,
	)
	require.NoError(t, err)
	return mcp.NewClient(session)
}

func TestDiscoverReturnsDeclaredTools(t *testing.T) {
	client := newInMemoryServer(t, map[string]func(args map[string]any) (string, error){
		"echo": func(args map[string]any) (string, error) {
			s, _ := args["text"].(string)
			return s, nil
		},
		"add": func(args map[string]any) (string, error) {
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			sum := float64ToInt(a) + float64ToInt(b)
			return intToStr(sum), nil
		},
	})
	schemas, err := client.Discover(context.Background())
	require.NoError(t, err)
	names := make([]string, 0, len(schemas))
	for _, s := range schemas {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "echo")
	assert.Contains(t, names, "add")
}

func TestCallForwardsToServer(t *testing.T) {
	client := newInMemoryServer(t, map[string]func(args map[string]any) (string, error){
		"echo": func(args map[string]any) (string, error) {
			s, _ := args["text"].(string)
			return s, nil
		},
	})
	res, err := client.Call(context.Background(), "echo", json.RawMessage(`{"text":"hello"}`))
	require.NoError(t, err)
	assert.True(t, res.OK)
	// Output is encoded JSON — content is a list of {type,text}.
	outStr, ok := res.Output.(json.RawMessage)
	require.True(t, ok)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(outStr, &out))
	require.NotEmpty(t, out)
	assert.Equal(t, "hello", out[0]["text"])
}

func TestCallReportsToolError(t *testing.T) {
	client := newInMemoryServer(t, map[string]func(args map[string]any) (string, error){
		"fail": func(_ map[string]any) (string, error) {
			return "", assertError("kaboom")
		},
	})
	res, err := client.Call(context.Background(), "fail", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "kaboom")
}

func TestInferRisk(t *testing.T) {
	// Risk classification is internal; exercised through Discover — when
	// we expose a get_* / read_* tool we get LOW, exec_* is HIGH.
	// Here we just sanity-check the public surface of Discover.
	client := newInMemoryServer(t, map[string]func(args map[string]any) (string, error){
		"read_data": func(_ map[string]any) (string, error) { return "x", nil },
		"delete_data": func(_ map[string]any) (string, error) { return "ok", nil },
	})
	schemas, err := client.Discover(context.Background())
	require.NoError(t, err)
	byName := make(map[string]core.ToolSchema, len(schemas))
	for _, s := range schemas {
		byName[s.Name] = s
	}
	assert.Equal(t, core.RISK_LEVEL_LOW, byName["read_data"].Risk)
	assert.Equal(t, core.RISK_LEVEL_HIGH, byName["delete_data"].Risk)
}

// helpers
func intToStr(n int) string        { return jsonString(itoaForTest(n)) }
func jsonString(s string) string   { return s }
func float64ToInt(f float64) int    { return int(f) }
func assertError(s string) error    { return &clientError{s} }

func itoaForTest(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type clientError struct{ msg string }

func (e *clientError) Error() string { return e.msg }