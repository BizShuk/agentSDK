package elevenlabs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

func newTestVoiceProvider(t *testing.T, handler http.HandlerFunc) *SpeechProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	speech, err := NewSpeech(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "test-key"},
	})
	require.NoError(t, err)
	return speech
}

func TestListVoicesMapsQueryAndResponse(t *testing.T) {
	speech := newTestVoiceProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, voicesPath, r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get(APIKeyHeader))
		query := r.URL.Query()
		assert.Equal(t, "rachel", query.Get("search"))
		assert.Equal(t, "premade", query.Get("category"))
		assert.Equal(t, "25", query.Get("page_size"))
		assert.Equal(t, "token-1", query.Get("next_page_token"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"voices": []map[string]any{
				{
					"voice_id":        "21m00Tcm4TlvDq8ikWAM",
					"name":            "Rachel",
					"category":        "premade",
					"labels":          map[string]string{"accent": "american"},
					"description":     "calm narration",
					"preview_url":     "https://cdn.example/rachel.mp3",
					"created_at_unix": 1700000000,
				},
			},
			"has_more":        true,
			"total_count":     42,
			"next_page_token": "token-2",
		})
	})

	result, err := speech.ListVoices(context.Background(), provider.VoiceListRequest{
		Search:    "rachel",
		Category:  "premade",
		PageSize:  25,
		PageToken: "token-1",
	})
	require.NoError(t, err)

	require.Len(t, result.Voices, 1)
	voice := result.Voices[0]
	assert.Equal(t, "21m00Tcm4TlvDq8ikWAM", voice.ID)
	assert.Equal(t, "Rachel", voice.Name)
	assert.Equal(t, "premade", voice.Category)
	assert.Equal(t, "calm narration", voice.Description)
	assert.Equal(t, map[string]string{"accent": "american"}, voice.Labels)
	assert.Equal(t, "https://cdn.example/rachel.mp3", voice.PreviewURL)
	assert.Equal(t, int64(1700000000), voice.CreatedAtUnix)

	assert.True(t, result.HasMore)
	assert.Equal(t, 42, result.TotalCount)
	assert.Equal(t, "token-2", result.NextPageToken)
}

func TestListVoicesRejectsOversizedPage(t *testing.T) {
	speech, err := NewSpeech(provider.ResolvedConfig{Auth: core.Auth{APIKey: "k"}})
	require.NoError(t, err)

	_, err = speech.ListVoices(context.Background(), provider.VoiceListRequest{PageSize: 101})
	require.ErrorContains(t, err, "page size")
}

func TestListVoicesAPIError(t *testing.T) {
	speech := newTestVoiceProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"detail": map[string]any{"status": "invalid_api_key", "message": "bad key"},
		})
	})

	_, err := speech.ListVoices(context.Background(), provider.VoiceListRequest{})
	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	assert.Equal(t, "invalid_api_key", apiErr.Code)
	assert.Equal(t, "list voices", apiErr.Operation)
}
