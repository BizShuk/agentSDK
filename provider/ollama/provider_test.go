package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/ollama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFakeOllama stands up a minimal HTTP server that pretends to be
// Ollama / an OpenAI-compatible endpoint. The handler inspects the
// /chat/completions request body and returns a small canned response.
func newFakeOllama(t *testing.T) (*httptest.Server, *string) {
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
		    {"message": {"role": "assistant", "content": "hello from ollama"}, "finish_reason": "stop"}
		  ],
		  "usage": {"prompt_tokens": 5, "completion_tokens": 4, "total_tokens": 9}
		}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotModel
}

func TestProviderRoundTripAgainstFakeOllama(t *testing.T) {
	srv, gotModel := newFakeOllama(t)
	p, err := ollama.New(provider.ResolvedConfig{BaseURL: srv.URL})
	require.NoError(t, err)

	req := core.ModelRequest{Messages: []core.Message{
		{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}},
	}}
	mr, err := p.Generate(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "hello from ollama", mr.Text)
	assert.Equal(t, "stop", mr.StopReason)
	assert.Equal(t, 9, mr.Usage.TotalTokens)
	assert.Equal(t, "qwen2.5vl:3b", *gotModel)
}

func TestProviderIncludesBearerHeader(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p, err := ollama.New(provider.ResolvedConfig{
		BaseURL: srv.URL,
		Auth:    core.Auth{APIKey: "sk-test"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-test", sawAuth)
}

func TestProviderPropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"rate-limited"}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p, err := ollama.New(provider.ResolvedConfig{BaseURL: srv.URL})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestProviderSkipsBearerForLocalHost(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p, err := ollama.New(provider.ResolvedConfig{BaseURL: srv.URL})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "x"}}}},
	})
	require.NoError(t, err)
	assert.Empty(t, sawAuth)
}

func TestProviderModelsCatalog(t *testing.T) {
	catalog := ollama.DefaultCatalog()
	require.NotEmpty(t, catalog)
	// We ship qwen2.5vl:3b as default model id in the catalog.
	ids := make([]string, 0, len(catalog))
	for _, m := range catalog {
		ids = append(ids, m.ID)
	}
	assert.Contains(t, ids, "qwen2.5vl:3b")
}
