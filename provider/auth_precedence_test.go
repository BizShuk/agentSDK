package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExplicitConstructionKeyOutranksModelDecorator(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, err := w.Write([]byte(`{
			"choices":[{
				"message":{"role":"assistant","content":"ok"},
				"finish_reason":"stop"
			}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	adapter, err := provider.New("google", provider.Options{
		Model:   "chat-model",
		BaseURL: srv.URL,
		APIKey:  "explicit-key",
		Decorator: func(context.Context) (core.Auth, error) {
			return core.Auth{Bearer: "ambient-token"}, nil
		},
	})
	require.NoError(t, err)

	_, err = adapter.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role: core.ROLE_USER,
			Parts: []core.Part{{
				Kind: core.PART_KIND_PLAIN_TEXT,
				Text: "ping",
			}},
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer explicit-key", gotAuth)
}
