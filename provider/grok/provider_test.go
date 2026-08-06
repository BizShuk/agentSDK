package grok_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/grok"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFakeGrok stands up a minimal HTTP server that pretends to be the
// xAI Grok chat-completions endpoint. The handler records the requested
// model and returns a canned OpenAI-compatible response.
func newFakeGrok(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	gotModel := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id": "test-id",
		  "choices": [
		    {"message": {"role": "assistant", "reasoning_content":"inspect", "content": "hello from grok"}, "finish_reason": "stop"}
		  ],
		  "usage": {"prompt_tokens": 7, "completion_tokens": 5, "total_tokens": 12}
		}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotModel
}

func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv("XAI_API_KEY", "")
	_, err := provider.New("grok", provider.Options{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "credential")
}

func TestNewFromEnv(t *testing.T) {
	t.Setenv("XAI_API_KEY", "xai-from-env")
	p, err := provider.New("grok", provider.Options{})
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestGenerateAgainstFakeServer(t *testing.T) {
	srv, gotModel := newFakeGrok(t)
	p, err := grok.New(provider.ResolvedConfig{
		Model:   "grok-4",
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "xai-test"},
	})
	require.NoError(t, err)

	req := core.ModelRequest{Messages: []core.Message{
		{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}},
	}}
	mr, err := p.Generate(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "hello from grok", mr.Text)
	require.Len(t, mr.Parts, 2)
	assert.Equal(t, core.PART_KIND_REASONING, mr.Parts[0].Kind)
	assert.Equal(t, "inspect", mr.Parts[0].Text)
	assert.Equal(t, "stop", mr.StopReason)
	assert.Equal(t, 12, mr.Usage.TotalTokens)
	assert.Equal(t, "grok-4", *gotModel)
}

func TestBearerHeaderFromAPIKey(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p, err := grok.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "xai-key"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer xai-key", sawAuth)
}

func TestBearerHeaderFromOAuth(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	// Both values may exist while a caller transitions credentials; Bearer wins.
	p, err := grok.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth: core.Auth{
			APIKey: "xai-should-be-ignored",
			Bearer: "oauth-access-token",
		},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer oauth-access-token", sawAuth)
	assert.NotContains(t, sawAuth, "xai-should-be-ignored",
		"apiKey should not leak into Authorization header when OAuth bearer is set")
}

func TestRequestBodyValidate(t *testing.T) {
	cases := []struct {
		name    string
		body    grok.RequestBody
		wantErr string
	}{
		{
			name:    "missing model",
			body:    grok.RequestBody{Messages: []grok.ChatMessage{{Role: "user", Content: "hi"}}},
			wantErr: "model is required",
		},
		{
			name:    "missing messages",
			body:    grok.RequestBody{Model: "grok-3"},
			wantErr: "at least one message",
		},
		{
			name: "bad role",
			body: grok.RequestBody{
				Model:    "grok-3",
				Messages: []grok.ChatMessage{{Role: "alien", Content: "hi"}},
			},
			wantErr: "role",
		},
		{
			name: "empty content",
			body: grok.RequestBody{
				Model:    "grok-3",
				Messages: []grok.ChatMessage{{Role: "user"}},
			},
			wantErr: "empty content",
		},
		{
			name: "happy path",
			body: grok.RequestBody{
				Model:    "grok-3",
				Messages: []grok.ChatMessage{{Role: "user", Content: "hi"}},
			},
			wantErr: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.body.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestReasoningRequestAndOpaqueMetadataBoundary(t *testing.T) {
	var reasoningContent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []grok.ChatMessage `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		reasoningContent = body.Messages[0].ReasoningContent
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	p, err := grok.New(provider.ResolvedConfig{BaseURL: srv.URL, Auth: core.Auth{APIKey: "k"}})
	require.NoError(t, err)

	_, err = p.Generate(context.Background(), core.ModelRequest{Messages: []core.Message{{
		Role:  core.ROLE_ASSISTANT,
		Parts: []core.Part{{Kind: core.PART_KIND_REASONING, Text: "previous"}},
	}}})
	require.NoError(t, err)
	assert.Equal(t, "previous", reasoningContent)

	_, err = p.Generate(context.Background(), core.ModelRequest{Messages: []core.Message{{
		Role: core.ROLE_ASSISTANT,
		Parts: []core.Part{{
			Kind:      core.PART_KIND_REASONING,
			Text:      "previous",
			Reasoning: &core.ReasoningState{EncryptedContent: "encrypted"},
		}},
	}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot preserve reasoning continuation metadata")
}

func TestProviderModelsContainsExpectedIDs(t *testing.T) {
	models := grok.DefaultCatalog()
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	for _, want := range []string{"grok-4.5", "grok-2-vision", grok.DefaultImageModel} {
		assert.Contains(t, ids, want, "catalog must include %s", want)
	}
}

func TestStreamAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"inspect\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello \"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	p, err := grok.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "xai-test"},
	})
	require.NoError(t, err)

	ch, err := p.Stream(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}}},
	})
	require.NoError(t, err)

	var assembled strings.Builder
	var reasoning strings.Builder
	for c := range ch {
		if c.Done {
			break
		}
		if c.Kind == core.PART_KIND_REASONING {
			reasoning.WriteString(c.Text)
		} else {
			assembled.WriteString(c.Text)
		}
	}
	assert.Equal(t, "hello world", assembled.String())
	assert.Equal(t, "inspect", reasoning.String())
}
