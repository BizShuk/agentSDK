package codex_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/codex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	_, err := provider.New("codex", provider.Options{})
	assert.Error(t, err)
}

func TestNewAcceptsExplicitAPIKey(t *testing.T) {
	p, err := codex.New(provider.ResolvedConfig{Auth: core.Auth{APIKey: "sk-test"}})
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestNewAcceptsResolvedOAuth(t *testing.T) {
	p, err := codex.New(provider.ResolvedConfig{Auth: core.Auth{
		Bearer: "oauth-access",
		Headers: map[string]string{
			"ChatGPT-Account-ID": "acc-123",
		},
	}})
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestBearerHeaderFromOAuth(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer srv.Close()

	p, err := codex.New(provider.ResolvedConfig{
		Model:   "gpt-5-mini",
		BaseURL: srv.URL,
		Auth: core.Auth{
			Bearer: "oauth-abc",
			Headers: map[string]string{
				"ChatGPT-Account-ID": "acc-1",
			},
		},
	})
	require.NoError(t, err)

	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer oauth-abc", sawAuth)
}

func TestGenerateRejectsStreamReadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: " + strings.Repeat("x", 2*1024*1024)))
	}))
	defer srv.Close()

	p, err := codex.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "k"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role:  core.ROLE_USER,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}},
		}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream closed before terminal chunk")
}

func TestCodexHeaders(t *testing.T) {
	var gotOriginator, gotVersion, gotUA, gotAccountID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOriginator = r.Header.Get("originator")
		gotVersion = r.Header.Get("version")
		gotUA = r.Header.Get("User-Agent")
		gotAccountID = r.Header.Get("ChatGPT-Account-ID")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer srv.Close()

	p, err := codex.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth: core.Auth{
			Bearer: "tok",
			Headers: map[string]string{
				"ChatGPT-Account-ID": "acc-xyz",
			},
		},
	})
	require.NoError(t, err)

	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
	})
	require.NoError(t, err)
	assert.Equal(t, codex.CodexOriginator, gotOriginator)
	assert.Equal(t, codex.CodexVersion, gotVersion)
	assert.Contains(t, gotUA, codex.CodexOriginator+"/"+codex.CodexVersion)
	assert.Contains(t, gotUA, "; ")
	assert.Equal(t, "acc-xyz", gotAccountID)
}

func TestLiftInstructions(t *testing.T) {
	// Two system messages, one user. Verify the system text goes to
	// Instructions (joined with "\n\n") and the user message stays
	// in Input.
	var sawInstructions string
	var sawInput []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if v, ok := req["instructions"].(string); ok {
			sawInstructions = v
		}
		if v, ok := req["input"].([]any); ok {
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					sawInput = append(sawInput, m)
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer srv.Close()

	p, err := codex.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "k"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{
			{Role: core.ROLE_SYSTEM, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "be brief"}}},
			{Role: core.ROLE_SYSTEM, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "be kind"}}},
			{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "be brief\n\nbe kind", sawInstructions, "system messages must join with \\n\\n")
	require.Len(t, sawInput, 1, "only the user message must remain in input")
	assert.Equal(t, "user", sawInput[0]["role"])
}

func TestResponsesReasoningRoundTrip(t *testing.T) {
	var requestInput []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []map[string]any `json:"input"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requestInput = body.Input
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"type":"response.output_item.done","item":{"id":"reasoning_out","type":"reasoning","summary":[{"type":"summary_text","text":"inspect first"}],"encrypted_content":"encrypted-out"}}`,
			``,
			`data: {"type":"response.output_text.delta","delta":"done"}`,
			``,
			`data: {"type":"response.completed"}`,
			``,
		}, "\n") + "\n"))
	}))
	defer srv.Close()

	p, err := codex.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "k"},
	})
	require.NoError(t, err)
	mr, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role: core.ROLE_ASSISTANT,
			Parts: []core.Part{
				{
					Kind: core.PART_KIND_REASONING,
					Text: "previous reasoning",
					Reasoning: &core.ReasoningState{
						ID:               "reasoning_in",
						EncryptedContent: "encrypted-in",
					},
				},
				{Kind: core.PART_KIND_PLAIN_TEXT, Text: "previous answer"},
			},
		}},
	})
	require.NoError(t, err)

	require.Len(t, requestInput, 2)
	assert.Equal(t, "reasoning", requestInput[0]["type"])
	assert.Equal(t, "reasoning_in", requestInput[0]["id"])
	assert.Equal(t, "encrypted-in", requestInput[0]["encrypted_content"])
	summary := requestInput[0]["summary"].([]any)
	assert.Equal(t, "previous reasoning", summary[0].(map[string]any)["text"])
	assert.Equal(t, "message", requestInput[1]["type"])

	require.Len(t, mr.Parts, 2)
	reasoningPart := mr.Parts[0]
	assert.Equal(t, core.PART_KIND_REASONING, reasoningPart.Kind)
	assert.Equal(t, "inspect first", reasoningPart.Text)
	require.NotNil(t, reasoningPart.Reasoning)
	assert.Equal(t, "reasoning_out", reasoningPart.Reasoning.ID)
	assert.Equal(t, "encrypted-out", reasoningPart.Reasoning.EncryptedContent)
	assert.Equal(t, "done", mr.Text)
}

func TestResponsesRejectsUnrepresentableAnthropicSignature(t *testing.T) {
	p, err := codex.New(provider.ResolvedConfig{Auth: core.Auth{APIKey: "k"}})
	require.NoError(t, err)

	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role: core.ROLE_ASSISTANT,
			Parts: []core.Part{{
				Kind:      core.PART_KIND_REASONING,
				Text:      "thinking",
				Reasoning: &core.ReasoningState{Signature: "anthropic-signature"},
			}},
		}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Anthropic signature")
}

func TestMaxOutputTokensStripped(t *testing.T) {
	// Even when the caller sets MaxTokens, the wire body MUST NOT
	// carry max_output_tokens (Codex rejects it).
	var sawBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sawBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer srv.Close()

	p, err := codex.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "k"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages:  []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
		MaxTokens: 4096,
	})
	require.NoError(t, err)
	_, hasMaxOutputTokens := sawBody["max_output_tokens"]
	assert.False(t, hasMaxOutputTokens, "Codex rejects max_output_tokens — must be stripped")
	// Stream/Store are always-on contract checks.
	assert.Equal(t, true, sawBody["stream"])
	assert.Equal(t, false, sawBody["store"])
	assert.Equal(t, []any{"reasoning.encrypted_content"}, sawBody["include"])
}

func TestLiteModelForcesParallelFalse(t *testing.T) {
	cases := []struct {
		model     string
		wantField bool
	}{
		{"gpt-5.6", true},
		{"gpt-5.6-sol", true},
		{"gpt-5", false},
		{"gpt-5-mini", false},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			var sawBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&sawBody)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
			}))
			defer srv.Close()

			p, err := codex.New(provider.ResolvedConfig{
				Model:   tc.model,
				BaseURL: srv.URL,
				Auth:    core.Auth{APIKey: "k"},
			})
			require.NoError(t, err)
			_, err = p.Generate(context.Background(), core.ModelRequest{
				Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
			})
			require.NoError(t, err)

			if tc.wantField {
				v, ok := sawBody["parallel_tool_calls"]
				assert.True(t, ok, "lite model must set parallel_tool_calls")
				assert.Equal(t, false, v)
			} else {
				_, ok := sawBody["parallel_tool_calls"]
				assert.False(t, ok, "non-lite model must omit parallel_tool_calls")
			}
		})
	}
}

func TestIsLiteModel(t *testing.T) {
	cases := map[string]bool{
		"gpt-5":       false,
		"gpt-5-mini":  false,
		"gpt-5.6":     true,
		"gpt-5.6-sol": true,
		"":            false,
		"some-other":  false,
	}
	for model, want := range cases {
		t.Run(model, func(t *testing.T) {
			assert.Equal(t, want, codex.IsLiteModel(model))
		})
	}
}

func TestRequestBodyValidate(t *testing.T) {
	// Missing model fails.
	err := codex.RequestBody{}.Validate()
	assert.Error(t, err)

	// Empty instructions + empty input fails.
	err = codex.RequestBody{Model: "gpt-5"}.Validate()
	assert.Error(t, err)

	// Instructions alone is OK.
	err = codex.RequestBody{Model: "gpt-5", Instructions: "hi"}.Validate()
	assert.NoError(t, err)

	// Input alone is OK.
	err = codex.RequestBody{Model: "gpt-5", Input: []codex.InputItem{{Type: "message", Role: "user"}}}.Validate()
	assert.NoError(t, err)
}

func TestCodexUserAgentFormat(t *testing.T) {
	ua := codex.CodexUserAgent()
	prefix := codex.CodexOriginator + "/" + codex.CodexVersion
	assert.True(t, strings.HasPrefix(ua, prefix))
	assert.Contains(t, ua, "; ")
	// Platform / arch separators must be the literal "(" and ")".
	assert.Regexp(t, `^`+regexp.QuoteMeta(prefix)+` \([a-z]+; [a-z0-9_]+\)$`, ua)
}

func TestGeneratePropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, err := codex.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "k-bad"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestDefaultCatalogReturnsExpectedFamily(t *testing.T) {
	models := codex.DefaultCatalog()
	require.NotEmpty(t, models)
	// Sanity: the catalog MUST ship the gpt-5.6 family variants
	// (gpt-5.6-sol is the lite one — the wire contract pins
	// parallel_tool_calls=false on it, see IsLiteModel).
	ids := map[string]bool{}
	for _, m := range models {
		ids[m.ID] = true
	}
	assert.True(t, ids["gpt-5.6-sol"], "gpt-5.6-sol must be in catalog")
	assert.True(t, ids["gpt-5.6-terra"], "gpt-5.6-terra must be in catalog")
	assert.True(t, ids["gpt-5.6-luna"], "gpt-5.6-luna must be in catalog")
	assert.True(t, ids["gpt-5.5"])
	assert.True(t, ids[codex.DefaultLiveModel])
}

// Compile-time: codex.Provider satisfies core.Provider.
var _ core.Provider = (*codex.Provider)(nil)
