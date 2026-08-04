package minimax

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

func voiceCatalogHandler(t *testing.T, capturedType *string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, voiceListPath, r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		var body voiceListRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		*capturedType = body.VoiceType
		_ = json.NewEncoder(w).Encode(map[string]any{
			"system_voice": []map[string]any{
				{"voice_id": "male-qn-qingse", "voice_name": "Radiant Girl",
					"description": []string{"English"}, "created_time": "2025-03-01"},
				{"voice_id": "presenter_male", "voice_name": "Expressive Narrator",
					"description": []string{"English", "Narration"}},
			},
			"voice_cloning": []map[string]any{
				{"voice_id": "clone-1", "created_time": "2025-06-15"},
			},
			"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
		})
	}
}

func TestListVoicesFoldsCategories(t *testing.T) {
	var capturedType string
	speech := newTestVoiceProvider(t, voiceCatalogHandler(t, &capturedType))

	result, err := speech.ListVoices(context.Background(), provider.VoiceListRequest{})
	require.NoError(t, err)

	assert.Equal(t, "all", capturedType, "empty category queries every class")
	require.Len(t, result.Voices, 3)
	assert.Equal(t, 3, result.TotalCount)
	assert.False(t, result.HasMore)

	first := result.Voices[0]
	assert.Equal(t, "male-qn-qingse", first.ID)
	assert.Equal(t, "Radiant Girl", first.Name)
	assert.Equal(t, "system", first.Category)
	assert.Equal(t, "English", first.Description)
	assert.NotZero(t, first.CreatedAtUnix)

	clone := result.Voices[2]
	assert.Equal(t, "clone-1", clone.ID)
	assert.Equal(t, "voice_cloning", clone.Category)
	assert.Empty(t, clone.Name)
}

func TestListVoicesSearchAndPageSize(t *testing.T) {
	var capturedType string
	speech := newTestVoiceProvider(t, voiceCatalogHandler(t, &capturedType))

	result, err := speech.ListVoices(context.Background(), provider.VoiceListRequest{
		Category: "system",
		Search:   "narrat",
	})
	require.NoError(t, err)
	assert.Equal(t, "system", capturedType)
	require.Len(t, result.Voices, 1)
	assert.Equal(t, "presenter_male", result.Voices[0].ID)

	paged, err := speech.ListVoices(context.Background(), provider.VoiceListRequest{PageSize: 2})
	require.NoError(t, err)
	assert.Len(t, paged.Voices, 2)
	assert.True(t, paged.HasMore)
	assert.Equal(t, 3, paged.TotalCount)
	assert.Empty(t, paged.NextPageToken)
}

func TestListVoicesRejectsUnsupportedInputs(t *testing.T) {
	speech, err := NewSpeech(provider.ResolvedConfig{Auth: core.Auth{APIKey: "k"}})
	require.NoError(t, err)

	_, err = speech.ListVoices(context.Background(), provider.VoiceListRequest{PageToken: "next"})
	require.ErrorContains(t, err, "pagination")

	_, err = speech.ListVoices(context.Background(), provider.VoiceListRequest{Category: "premade"})
	require.ErrorContains(t, err, "voice category")
}

func TestListVoicesAPIError(t *testing.T) {
	speech := newTestVoiceProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"base_resp": map[string]any{"status_code": 2013, "status_msg": "invalid params"},
		})
	})

	_, err := speech.ListVoices(context.Background(), provider.VoiceListRequest{})
	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "2013", apiErr.Code)
	assert.Equal(t, "list voices", apiErr.Operation)
}
