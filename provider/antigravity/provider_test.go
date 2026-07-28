package antigravity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/antigravity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewRequiresAPIKey — without an explicit key or env, New() fails fast.
func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv("ANTIGRAVITY_API_KEY", "")
	_, err := provider.New("antigravity", provider.Options{})
	assert.Error(t, err)
}

// TestNewWithExplicitAPIKey — resolved construction auth is accepted directly.
func TestNewWithExplicitAPIKey(t *testing.T) {
	p, err := antigravity.New(provider.ResolvedConfig{Auth: core.Auth{APIKey: "sk-direct"}})
	require.NoError(t, err)
	assert.NotNil(t, p)
}

// TestGenerateAgainstFakeServer — spin up an httptest server that mimics
// the Anthropic-Messages response shape and verify Generate() round-trips.
func TestGenerateAgainstFakeServer(t *testing.T) {
	var (
		gotAuth   string
		gotAPIKey string
		gotPath   string
		gotBody   []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_test",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-5",
			"stop_reason": "end_turn",
			"content": [{"type": "text", "text": "hello back"}],
			"usage": {"input_tokens": 7, "output_tokens": 3}
		}`))
	}))
	defer srv.Close()

	t.Setenv("ANTIGRAVITY_API_KEY", "sk-from-env")
	t.Setenv("ANTIGRAVITY_BASE_URL", srv.URL)

	p, err := provider.New("antigravity", provider.Options{})
	require.NoError(t, err)

	res, err := p.Generate(context.Background(), core.ModelRequest{
		MaxTokens: 128,
		Messages: []core.Message{
			{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hello"}}},
		},
	})
	require.NoError(t, err)
	// baseURL is the test server (no /v1), so the path is /messages.
	// The default gateway URL embeds /v1, so production sends /v1/messages.
	assert.Equal(t, "/messages", gotPath)
	assert.Equal(t, "hello back", res.Text)
	assert.Equal(t, "end_turn", res.StopReason)
	assert.Equal(t, 7, res.Usage.PromptTokens)
	assert.Equal(t, 3, res.Usage.CompletionTokens)
	assert.Equal(t, 10, res.Usage.TotalTokens)

	// Body sanity: model + max_tokens + a message made it across.
	var sent map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &sent))
	assert.Equal(t, "claude-sonnet-5", sent["model"])
	assert.Equal(t, float64(128), sent["max_tokens"])
	msgs, ok := sent["messages"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, msgs)

	// Default auth is the API-key path.
	assert.Empty(t, gotAuth, "API-key mode must not send an Authorization header")
	assert.Equal(t, "sk-from-env", gotAPIKey)
}

// TestBearerHeaderFromOAuth — resolved OAuth auth carries Authorization:
// Bearer <token>, not x-api-key.
func TestBearerHeaderFromOAuth(t *testing.T) {
	var gotAuth, gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_oauth",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-5",
			"stop_reason": "end_turn",
			"content": [{"type": "text", "text": "ok"}],
			"usage": {"input_tokens": 1, "output_tokens": 1}
		}`))
	}))
	defer srv.Close()

	p, err := antigravity.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{Bearer: "ya29.fake-bearer-token"},
	})
	require.NoError(t, err)

	_, err = p.Generate(context.Background(), core.ModelRequest{
		MaxTokens: 64,
		Messages: []core.Message{
			{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "ping"}}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer ya29.fake-bearer-token", gotAuth)
	assert.Empty(t, gotAPIKey, "OAuth mode must not send x-api-key")
}

func TestThinkingRoundTrip(t *testing.T) {
	var requestBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&requestBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5",
			"stop_reason":"end_turn",
			"content":[{"type":"thinking","thinking":"inspect","signature":"sig-out"},{"type":"text","text":"done"}],
			"usage":{"input_tokens":1,"output_tokens":2}
		}`))
	}))
	defer srv.Close()

	p, err := antigravity.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "k"},
	})
	require.NoError(t, err)
	result, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role: core.ROLE_ASSISTANT,
			Parts: []core.Part{{
				Kind:      core.PART_KIND_REASONING,
				Text:      "previous",
				Reasoning: &core.ReasoningState{Signature: "sig-in"},
			}},
		}},
	})
	require.NoError(t, err)

	messages := requestBody["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	assert.Equal(t, "thinking", content[0].(map[string]any)["type"])
	assert.Equal(t, "sig-in", content[0].(map[string]any)["signature"])
	require.Len(t, result.Parts, 2)
	assert.Equal(t, core.PART_KIND_REASONING, result.Parts[0].Kind)
	require.NotNil(t, result.Parts[0].Reasoning)
	assert.Equal(t, "sig-out", result.Parts[0].Reasoning.Signature)
	assert.Equal(t, "done", result.Text)
}

func TestStreamPreservesThinkingAndSignature(t *testing.T) {
	stream := strings.NewReader(strings.Join([]string{
		`data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"inspect"}}`,
		``,
		`data: {"type":"content_block_delta","delta":{"type":"signature_delta","signature":"sig"}}`,
		``,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n") + "\n")
	chunks, _ := antigravity.ParseStream(context.Background(), stream)
	var got []core.ModelChunk
	for chunk := range chunks {
		got = append(got, chunk)
	}
	require.Len(t, got, 3)
	assert.Equal(t, core.PART_KIND_REASONING, got[0].Kind)
	assert.Equal(t, "inspect", got[0].Text)
	require.NotNil(t, got[1].Reasoning)
	assert.Equal(t, "sig", got[1].Reasoning.Signature)
	assert.True(t, got[2].Done)
}

func TestGenerateRejectsResponsesReasoningMetadata(t *testing.T) {
	p, err := antigravity.New(provider.ResolvedConfig{Auth: core.Auth{APIKey: "k"}})
	require.NoError(t, err)

	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role: core.ROLE_ASSISTANT,
			Parts: []core.Part{{
				Kind:      core.PART_KIND_REASONING,
				Text:      "inspect",
				Reasoning: &core.ReasoningState{ID: "reasoning_1"},
			}},
		}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Responses continuation metadata")
}

// TestRequestBodyValidate — covers the four Validate() failure modes.
func TestRequestBodyValidate(t *testing.T) {
	cases := []struct {
		name string
		body antigravity.RequestBody
		want string
	}{
		{
			name: "empty model",
			body: antigravity.RequestBody{MaxTokens: 1, Messages: []antigravity.MessageParam{
				{Role: "user", Content: []antigravity.ContentParam{{Type: "text", Text: "hi"}}},
			}},
			want: "model is required",
		},
		{
			name: "zero max_tokens",
			body: antigravity.RequestBody{Model: "claude-sonnet-5", Messages: []antigravity.MessageParam{
				{Role: "user", Content: []antigravity.ContentParam{{Type: "text", Text: "hi"}}},
			}},
			want: "max_tokens must be positive",
		},
		{
			name: "no messages",
			body: antigravity.RequestBody{Model: "claude-sonnet-5", MaxTokens: 1},
			want: "at least one message",
		},
		{
			name: "bad role",
			body: antigravity.RequestBody{
				Model: "claude-sonnet-5", MaxTokens: 1,
				Messages: []antigravity.MessageParam{{Role: "system", Content: []antigravity.ContentParam{{Type: "text", Text: "x"}}}},
			},
			want: "role",
		},
		{
			name: "empty content",
			body: antigravity.RequestBody{
				Model: "claude-sonnet-5", MaxTokens: 1,
				Messages: []antigravity.MessageParam{{Role: "user"}},
			},
			want: "no content blocks",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.body.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
