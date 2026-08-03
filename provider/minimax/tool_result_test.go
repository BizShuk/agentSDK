package minimax_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/minimax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tool_result block carries either a string or an array of content blocks —
// never a bare object. Marshalling the tool output straight into the block
// looked right for a tool returning a string, but any tool returning a struct
// or map produced an object and minimax rejected the entire request with
// "invalid params, invalid tool_result content (2013)", so such an agent could
// not complete a single round against the live API.
func TestToolResultContentIsAlwaysAString(t *testing.T) {
	structured := map[string]any{
		"hits":  []any{map[string]any{"heading": "保固", "score": 0.74}},
		"scope": true,
	}

	tests := []struct {
		name   string
		output any
		want   string
	}{
		{
			name:   "結構化輸出序列化為字串",
			output: structured,
			want:   `{"hits":[{"heading":"保固","score":0.74}],"scope":true}`,
		},
		{
			name:   "字串輸出原樣送出，不會被再包一層引號",
			output: "found: 12 months",
			want:   "found: 12 months",
		},
		{
			name:   "陣列輸出也序列化為字串",
			output: []any{"a", "b"},
			want:   `["a","b"]`,
		},
		{
			name:   "nil 輸出不會讓區塊消失",
			output: nil,
			want:   "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(body, &captured))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(cannedAnthropicResponse))
			}))
			defer server.Close()

			p, err := minimax.New(provider.ResolvedConfig{
				Model:   "MiniMax-M2",
				BaseURL: server.URL,
				Auth:    core.Auth{APIKey: "test-key"},
			})
			require.NoError(t, err)

			_, err = p.Generate(context.Background(), core.ModelRequest{
				Messages: []core.Message{
					{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "查保固"}}},
					{Role: core.ROLE_ASSISTANT, Parts: []core.Part{{
						Kind:    core.PART_KIND_TOOL_USE,
						ToolUse: &core.ToolCall{ID: "toolu_1", Name: "search_scope"},
					}}},
					{Role: core.ROLE_USER, Parts: []core.Part{{
						Kind: core.PART_KIND_TOOL_RESULT,
						ToolResult: &core.ToolResult{
							CallID: "toolu_1",
							Name:   "search_scope",
							OK:     true,
							Output: tt.output,
						},
					}}},
				},
			})
			require.NoError(t, err)

			block := findToolResultBlock(t, captured)
			content, isString := block["content"].(string)
			assert.True(t, isString,
				"content 必須是字串；物件會被 minimax 以 2013 拒絕，實際型別 %T", block["content"])
			assert.Equal(t, tt.want, content)
			assert.Equal(t, "toolu_1", block["tool_use_id"], "配對必須保留")
		})
	}
}

// findToolResultBlock 從送出的 request body 撈出唯一的 tool_result 區塊。
func findToolResultBlock(t *testing.T, body map[string]any) map[string]any {
	t.Helper()

	messages, ok := body["messages"].([]any)
	require.True(t, ok, "request 必須有 messages")

	for _, message := range messages {
		blocks, ok := message.(map[string]any)["content"].([]any)
		if !ok {
			continue
		}
		for _, block := range blocks {
			b, ok := block.(map[string]any)
			if ok && b["type"] == "tool_result" {
				return b
			}
		}
	}
	require.FailNow(t, "request 裡找不到 tool_result 區塊")
	return nil
}
