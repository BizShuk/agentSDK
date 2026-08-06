package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetch(t *testing.T) {
	t.Run("sends headers and returns body on 2xx", func(t *testing.T) {
		var gotAuth, gotAccept string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotAccept = r.Header.Get("Accept")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		raw, err := Fetch(context.Background(), srv.Client(), srv.URL, map[string]string{
			"Authorization": "Bearer secret",
			"Skip":          "", // empty values must not be set
		})
		require.NoError(t, err)
		assert.JSONEq(t, `{"ok":true}`, string(raw))
		assert.Equal(t, "Bearer secret", gotAuth)
		assert.Equal(t, "application/json", gotAccept)
	})

	t.Run("non-2xx becomes an error with a truncated excerpt", func(t *testing.T) {
		body := strings.Repeat("x", MAX_ERROR_BYTES+100)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()

		_, err := Fetch(context.Background(), srv.Client(), srv.URL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 401")
		assert.Contains(t, err.Error(), "…", "long body should be truncated with an ellipsis")
		assert.Less(t, len(err.Error()), len(body), "excerpt must be shorter than the full body")
	})

	t.Run("body is capped at MAX_BODY_BYTES", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Advertise JSON but stream more than the cap.
			for range (MAX_BODY_BYTES / 1024) + 8 {
				_, _ = w.Write(make([]byte, 1024))
			}
		}))
		defer srv.Close()

		raw, err := Fetch(context.Background(), srv.Client(), srv.URL, nil)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(raw), MAX_BODY_BYTES)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := Fetch(ctx, http.DefaultClient, "http://127.0.0.1:1/models", nil)
		require.Error(t, err)
	})
}

func TestDecodeIDList(t *testing.T) {
	t.Run("pulls ids and skips blanks", func(t *testing.T) {
		ids, err := DecodeIDList([]byte(`{"data":[{"id":"a"},{"id":""},{"id":"b"}]}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, ids)
	})

	t.Run("empty data is an error, not an empty slice", func(t *testing.T) {
		_, err := DecodeIDList([]byte(`{"data":[]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no model ids")
	})

	t.Run("malformed json is an error", func(t *testing.T) {
		_, err := DecodeIDList([]byte(`not json`))
		require.Error(t, err)
	})
}

func TestMerge(t *testing.T) {
	static := []provider.ModelSpec{
		{ID: "known", Family: "fam", Reasoning: true, ContextWindow: 1000, MaxTokens: 100},
		{ID: "dropped-upstream", Family: "gone"},
	}

	t.Run("live ids drive membership and order", func(t *testing.T) {
		out := Merge([]string{"new", "known"}, static)
		require.Len(t, out, 2)
		// Order follows the live list, not the static catalog.
		assert.Equal(t, "new", out[0].ID)
		assert.Equal(t, "known", out[1].ID)
		// A static entry absent from the live list is gone.
		for _, s := range out {
			assert.NotEqual(t, "dropped-upstream", s.ID)
		}
	})

	t.Run("known ids keep their bundled metadata", func(t *testing.T) {
		out := Merge([]string{"known"}, static)
		require.Len(t, out, 1)
		assert.Equal(t, "fam", out[0].Family)
		assert.True(t, out[0].Reasoning)
		assert.Equal(t, 1000, out[0].ContextWindow)
	})

	t.Run("unknown ids carry the id alone", func(t *testing.T) {
		out := Merge([]string{"mystery"}, static)
		require.Len(t, out, 1)
		assert.Equal(t, "mystery", out[0].ID)
		assert.Empty(t, out[0].Family)
		assert.Zero(t, out[0].ContextWindow)
	})
}
