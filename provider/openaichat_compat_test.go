package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/google"
	"github.com/bizshuk/agentsdk/provider/ollama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type openAIChatAdapterCase struct {
	name string
	new  func(t *testing.T, baseURL string) provider.Adapter
}

func openAIChatAdapterCases() []openAIChatAdapterCase {
	return []openAIChatAdapterCase{
		{
			name: "google",
			new: func(t *testing.T, baseURL string) provider.Adapter {
				t.Helper()
				adapter, err := google.New(provider.ResolvedConfig{
					Model:   "shared-model",
					BaseURL: baseURL,
					Auth:    core.Auth{APIKey: "test-key"},
				})
				require.NoError(t, err)
				return adapter
			},
		},
		{
			name: "ollama",
			new: func(t *testing.T, baseURL string) provider.Adapter {
				t.Helper()
				adapter, err := ollama.New(provider.ResolvedConfig{
					Model:   "shared-model",
					BaseURL: baseURL,
					Auth:    core.Auth{APIKey: "test-key"},
				})
				require.NoError(t, err)
				return adapter
			},
		},
	}
}

func openAIChatRequest() core.ModelRequest {
	return core.ModelRequest{
		Messages: []core.Message{
			{
				Role: core.ROLE_SYSTEM,
				Parts: []core.Part{{
					Kind: core.PART_KIND_PLAIN_TEXT,
					Text: "stay terse",
				}},
			},
			{
				Role: core.ROLE_USER,
				Parts: []core.Part{{
					Kind: core.PART_KIND_PLAIN_TEXT,
					Text: "inspect",
				}},
			},
			{
				Role: core.ROLE_ASSISTANT,
				Parts: []core.Part{
					{Kind: core.PART_KIND_PLAIN_TEXT, Text: "calling"},
					{
						Kind: core.PART_KIND_TOOL_USE,
						ToolUse: &core.ToolCall{
							ID:   "call-1",
							Name: "sum",
							Args: map[string]any{
								"a": 1,
								"b": 2,
							},
						},
					},
				},
			},
			{
				Role: core.ROLE_TOOL,
				Parts: []core.Part{{
					Kind: core.PART_KIND_TOOL_RESULT,
					ToolResult: &core.ToolResult{
						CallID: "call-1",
						Name:   "sum",
						OK:     true,
						Output: map[string]any{"total": 3},
					},
				}},
			},
		},
		Tools: []core.ToolSpec{{
			Name:        "sum",
			Description: "add integers",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"]}`),
		}},
		MaxTokens: 321,
	}
}

func readOpenAIChatGolden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/openaichat/" + name)
	require.NoError(t, err)
	if strings.HasSuffix(name, ".sse") {
		if !bytes.HasSuffix(raw, []byte("\n\n")) {
			raw = append(raw, '\n')
		}
		return raw
	}
	return bytes.TrimSuffix(raw, []byte("\n"))
}

func TestOpenAIChatGenerateGolden(t *testing.T) {
	response := readOpenAIChatGolden(t, "response.json")
	wantRequest := readOpenAIChatGolden(t, "generate_request.json")
	wantResult := core.ModelResult{
		Text:       "weather",
		StopReason: "tool_calls",
		ToolCalls: []core.ToolCall{{
			ID:   "call-2",
			Name: "weather",
			Args: map[string]any{"city": "Taipei"},
		}},
		Usage: core.TokenUsage{
			PromptTokens:     8,
			CompletionTokens: 3,
			TotalTokens:      11,
		},
	}

	for _, tc := range openAIChatAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte
			var gotPath string
			var gotAuth string
			var gotAccept string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				gotAccept = r.Header.Get("Accept")
				var err error
				gotBody, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(response)
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			got, err := tc.new(t, srv.URL).Generate(context.Background(), openAIChatRequest())
			require.NoError(t, err)
			assert.Equal(t, wantRequest, gotBody)
			assert.Equal(t, "/chat/completions", gotPath)
			assert.Equal(t, "Bearer test-key", gotAuth)
			assert.Empty(t, gotAccept)
			assert.Equal(t, wantResult, got)
		})
	}
}

func TestOpenAIChatStreamGolden(t *testing.T) {
	stream := readOpenAIChatGolden(t, "stream.sse")
	wantRequest := readOpenAIChatGolden(t, "stream_request.json")
	wantChunks := []core.ModelChunk{
		{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hel"},
		{Kind: core.PART_KIND_PLAIN_TEXT, Text: "lo"},
		{
			Kind: core.PART_KIND_TOOL_USE,
			ToolUse: &core.ToolCall{
				ID:   "call-2",
				Name: "weather",
				Args: map[string]any{"city": "Taipei"},
			},
		},
		{Done: true},
		{Done: true},
	}

	for _, tc := range openAIChatAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte
			var gotAccept string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAccept = r.Header.Get("Accept")
				var err error
				gotBody, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				w.Header().Set("Content-Type", "text/event-stream")
				_, err = w.Write(stream)
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			chunks, err := tc.new(t, srv.URL).Stream(context.Background(), openAIChatRequest())
			require.NoError(t, err)

			var got []core.ModelChunk
			for chunk := range chunks {
				got = append(got, chunk)
			}
			assert.Equal(t, wantRequest, gotBody)
			assert.Equal(t, "text/event-stream", gotAccept)
			assert.Equal(t, wantChunks, got)
		})
	}
}

func TestOpenAIChatErrorSemantics(t *testing.T) {
	for _, tc := range openAIChatAdapterCases() {
		t.Run(tc.name+"/validation", func(t *testing.T) {
			_, err := tc.new(t, "http://127.0.0.1:1").Generate(context.Background(), core.ModelRequest{})
			require.EqualError(t, err, tc.name+": at least one message is required")
		})

		t.Run(tc.name+"/status", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
				_, err := w.Write([]byte(`{"error":"rate-limited"}`))
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			_, err := tc.new(t, srv.URL).Generate(context.Background(), openAIChatRequest())
			require.EqualError(t, err, tc.name+`: status 429: {"error":"rate-limited"}`)
		})

		t.Run(tc.name+"/decode", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write([]byte("not-json"))
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			_, err := tc.new(t, srv.URL).Generate(context.Background(), openAIChatRequest())
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.name+": decode:")
		})
	}
}

func TestOpenAIChatInvalidRawSchemaKeepsLegacyEmptyPayload(t *testing.T) {
	response := readOpenAIChatGolden(t, "response.json")
	req := openAIChatRequest()
	req.Tools[0].Parameters = json.RawMessage(`{invalid`)

	for _, tc := range openAIChatAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				gotBody, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				_, err = w.Write(response)
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			_, err := tc.new(t, srv.URL).Generate(context.Background(), req)
			require.NoError(t, err)
			assert.Empty(t, gotBody)
		})
	}
}

func TestOpenAIChatScannerErrorClosesWithoutDone(t *testing.T) {
	payload := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")

	for _, tc := range openAIChatAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Content-Length", strconv.Itoa(len(payload)+10))
				_, err := w.Write(payload)
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			chunks, err := tc.new(t, srv.URL).Stream(context.Background(), openAIChatRequest())
			require.NoError(t, err)

			var got []core.ModelChunk
			for chunk := range chunks {
				got = append(got, chunk)
			}
			assert.Equal(t, []core.ModelChunk{{
				Kind: core.PART_KIND_PLAIN_TEXT,
				Text: "partial",
			}}, got)
		})
	}
}

func TestOpenAIChatContextCancellationClosesStream(t *testing.T) {
	for _, tc := range openAIChatAdapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, err := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n"))
				assert.NoError(t, err)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithCancel(context.Background())
			chunks, err := tc.new(t, srv.URL).Stream(ctx, openAIChatRequest())
			require.NoError(t, err)

			select {
			case chunk := <-chunks:
				require.Equal(t, core.ModelChunk{
					Kind: core.PART_KIND_PLAIN_TEXT,
					Text: "first",
				}, chunk)
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for first stream chunk")
			}

			cancel()
			select {
			case _, ok := <-chunks:
				assert.False(t, ok, "stream must close without a terminal chunk after cancellation")
			case <-time.After(2 * time.Second):
				t.Fatal("stream did not close after context cancellation")
			}
		})
	}
}
