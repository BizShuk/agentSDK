package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFakeAnthropic stands up a minimal /v1/messages server returning a
// canned Anthropic response (text + tool_use). The provider's Generate
// must parse this into the expected core.ModelResult — exercising the
// toAnthropicMessages / fromAnthropicResponse translation end-to-end
// without a real API key.
func newFakeAnthropic(t *testing.T, respBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGenerateParsesTextAndToolUse(t *testing.T) {
	// Anthropic response: one text block + one tool_use block.
	body := `{
	  "id": "msg_1",
	  "type": "message",
	  "role": "assistant",
	  "model": "claude-3-5-sonnet-latest",
	  "stop_reason": "tool_use",
	  "content": [
	    {"type": "text", "text": "I'll read the log first."},
	    {"type": "tool_use", "id": "call-1", "name": "read_log_tail", "input": {"n": 5}}
	  ],
	  "usage": {"input_tokens": 10, "output_tokens": 8}
	}`
	srv := newFakeAnthropic(t, body)

	p, err := anthropic.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "sk-test"},
	})
	require.NoError(t, err)

	mr, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role:  core.ROLE_USER,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "diagnose"}},
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, "tool_use", mr.StopReason)
	assert.Contains(t, mr.Text, "read the log")
	require.Len(t, mr.ToolCalls, 1)
	assert.Equal(t, "call-1", mr.ToolCalls[0].ID)
	assert.Equal(t, "read_log_tail", mr.ToolCalls[0].Name)
	assert.Equal(t, float64(5), mr.ToolCalls[0].Args["n"])
	assert.Equal(t, 18, mr.Usage.TotalTokens, "input+output")
}

func TestGeneratePreservesCacheAndNativeWebSearchUsage(t *testing.T) {
	body := `{
      "id":"msg_usage",
      "type":"message",
      "role":"assistant",
      "model":"claude-sonnet-5",
      "stop_reason":"end_turn",
      "content":[{"type":"text","text":"done"}],
      "usage":{
        "input_tokens":10,
        "output_tokens":2,
        "cache_creation_input_tokens":3,
        "cache_read_input_tokens":4,
        "server_tool_use":{"web_search_requests":2}
      }
    }`
	p, err := anthropic.New(provider.ResolvedConfig{
		Model:   "claude-sonnet-5",
		BaseURL: newFakeAnthropic(t, body).URL,
		Auth:    core.Auth{APIKey: "sk-test"},
	})
	require.NoError(t, err)

	result, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "search"}}}},
	})
	require.NoError(t, err)
	assert.Equal(t, core.TokenUsage{
		InputTokens:          17,
		OutputTokens:         2,
		InputCacheReadTokens: 4,
		WebSearchCount:       2,
		TotalTokens:          19,
	}, result.Usage)
}

func TestGenerateRoundTripsThinking(t *testing.T) {
	var gotThinking map[string]any
	body := `{
	  "id":"msg_1",
	  "type":"message",
	  "role":"assistant",
	  "model":"claude-opus-4-8",
	  "stop_reason":"tool_use",
	  "content":[
	    {"type":"thinking","thinking":"inspect first","signature":"sig-response"},
	    {"type":"tool_use","id":"call-1","name":"read","input":{}}
	  ],
	  "usage":{"input_tokens":2,"output_tokens":3}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content []map[string]any `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		for _, block := range request.Messages[0].Content {
			if block["type"] == "thinking" {
				gotThinking = block
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p, err := anthropic.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "sk-test"},
	})
	require.NoError(t, err)

	mr, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role: core.ROLE_ASSISTANT,
			Parts: []core.Part{{
				Kind:      core.PART_KIND_REASONING,
				Text:      "inspect previous turn",
				Reasoning: &core.ReasoningState{Signature: "sig-request"},
			}},
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, "thinking", gotThinking["type"])
	assert.Equal(t, "inspect previous turn", gotThinking["thinking"])
	assert.Equal(t, "sig-request", gotThinking["signature"])

	require.Len(t, mr.Parts, 2)
	reasoningPart := mr.Parts[0]
	assert.Equal(t, core.PART_KIND_REASONING, reasoningPart.Kind)
	assert.Equal(t, "inspect first", reasoningPart.Text)
	require.NotNil(t, reasoningPart.Reasoning)
	assert.Equal(t, "sig-response", reasoningPart.Reasoning.Signature)
	assert.Empty(t, mr.Text, "reasoning must not be folded into visible assistant text")
	require.Len(t, mr.ToolCalls, 1)
	assert.Equal(t, "call-1", mr.ToolCalls[0].ID)
}

func TestGenerateRejectsResponsesReasoningMetadata(t *testing.T) {
	p, err := anthropic.New(provider.ResolvedConfig{Auth: core.Auth{APIKey: "sk-test"}})
	require.NoError(t, err)

	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role: core.ROLE_ASSISTANT,
			Parts: []core.Part{{
				Kind: core.PART_KIND_REASONING,
				Text: "inspect",
				Reasoning: &core.ReasoningState{
					ID:               "reasoning_1",
					EncryptedContent: "encrypted",
				},
			}},
		}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Responses continuation metadata")
}

func TestGenerateAuthHeaders(t *testing.T) {
	cases := []struct {
		name              string
		auth              core.Auth
		wantAuthorization string
		wantAPIKey        string
		wantBeta          string
	}{
		{
			name:       "api key",
			auth:       core.Auth{APIKey: "sk-test"},
			wantAPIKey: "sk-test",
		},
		{
			name:              "oauth bearer",
			auth:              core.Auth{Bearer: "oauth-test"},
			wantAuthorization: "Bearer oauth-test",
			wantBeta:          anthropic.OAuthBetaValue,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuthorization, gotAPIKey, gotBeta string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuthorization = r.Header.Get("Authorization")
				gotAPIKey = r.Header.Get("x-api-key")
				gotBeta = r.Header.Get(anthropic.OAuthBetaHeader)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"x","stop_reason":"end_turn","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
			}))
			defer srv.Close()

			p, err := anthropic.New(provider.ResolvedConfig{BaseURL: srv.URL, Auth: tc.auth})
			require.NoError(t, err)
			_, err = p.Generate(context.Background(), core.ModelRequest{
				Messages: []core.Message{{
					Role:  core.ROLE_USER,
					Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}},
				}},
			})
			require.NoError(t, err)
			assert.Equal(t, tc.wantAuthorization, gotAuthorization)
			assert.Equal(t, tc.wantAPIKey, gotAPIKey)
			assert.Equal(t, tc.wantBeta, gotBeta)
		})
	}
}

// TestGeneratePropagatesHTTPError verifies a non-2xx surfaces as an error
// rather than a zero-value ModelResult.
func TestGeneratePropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, err := anthropic.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "sk-bad"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
	})
	require.Error(t, err)
}

func TestGenerateUsesConfiguredModel(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"claude-opus-4-8","stop_reason":"end_turn","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	p, err := anthropic.New(provider.ResolvedConfig{
		Model:   "claude-opus-4-8",
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "sk-x"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role:  core.ROLE_USER,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}},
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, "claude-opus-4-8", gotModel)
}

// TestToolSpecForwardedAsInputSchema verifies the tool's JSON schema
// parameters survive into the outbound request as the tool's input
// schema. We assert by inspecting the request body the fake receives.
func TestToolSpecForwardedAsInputSchema(t *testing.T) {
	var sawTools bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if tools, ok := req["tools"].([]any); ok && len(tools) > 0 {
			sawTools = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"x","stop_reason":"end_turn","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	p, err := anthropic.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "sk-test"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}}},
		Tools: []core.ToolSpec{{
			Name: "read_log_tail", Description: "read log",
			Parameters: json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`),
			Risk:       core.RISK_LEVEL_LOW,
		}},
	})
	require.NoError(t, err)
	assert.True(t, sawTools, "tools must be forwarded to the API as input schemas")
}

// Compile-time: ensure Provider satisfies the port.
var _ core.Provider = (*anthropic.Provider)(nil)
