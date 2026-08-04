package elevenlabs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/elevenlabs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveCatalogBody is the vendor's bare-array shape: a JSON list, not the
// {"data":[...]} envelope the OpenAI-compatible adapters return.
const liveCatalogBody = `[
  {"model_id":"eleven_v3","name":"Eleven v3","can_do_text_to_speech":true},
  {"model_id":"eleven_multilingual_v2","name":"Multilingual v2"},
  {"model_id":"unknown_model","name":"Unknown Model"},
  {"model_id":""}
]`

func TestListModelsUsesLiveCatalog(t *testing.T) {
	var (
		capturedMethod string
		capturedPath   string
		capturedKey    string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedKey = r.Header.Get("xi-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(liveCatalogBody))
	}))
	t.Cleanup(server.Close)

	speech, err := elevenlabs.NewSpeech(speechConfig(server.URL))
	require.NoError(t, err)

	specs, err := speech.ListModels(context.Background())
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, capturedMethod)
	assert.Equal(t, "/v1/models", capturedPath)
	assert.Equal(t, "test-key", capturedKey)

	ids := make([]string, 0, len(specs))
	for _, spec := range specs {
		ids = append(ids, spec.ID)
	}
	assert.Equal(t, []string{
		// Live ids drive membership and order for synthesis models…
		"eleven_v3",
		"eleven_multilingual_v2",
		"unknown_model",
		// …and the audio models the endpoint cannot report are appended.
		"scribe_v2",
		"eleven_english_sts_v2",
		"eleven_multilingual_sts_v2",
	}, ids)

	assert.Equal(t, "eleven_v3", specs[0].Family,
		"an id the bundle knows keeps its bundled metadata")
	assert.Empty(t, specs[2].Family,
		"an id the bundle does not know carries the id alone")
	assert.Equal(t,
		[]core.Modality{core.MODALITY_AUDIO},
		specs[3].Input,
		"the appended transcription model keeps its audio modality")
}

func TestListModelsRejectsUnusableResponses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		expect string
	}{
		{
			name:   "empty array is an error, not an empty catalog",
			status: http.StatusOK,
			body:   `[]`,
			expect: "no model ids",
		},
		{
			name:   "malformed json is an error",
			status: http.StatusOK,
			body:   `not json`,
			expect: "decode",
		},
		{
			name:   "non-2xx surfaces the status",
			status: http.StatusUnauthorized,
			body:   `{"detail":{"status":"missing_key"}}`,
			expect: "status 401",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(testCase.status)
				_, _ = w.Write([]byte(testCase.body))
			}))
			t.Cleanup(server.Close)

			speech, err := elevenlabs.NewSpeech(speechConfig(server.URL))
			require.NoError(t, err)

			_, err = speech.ListModels(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.expect)
			assert.Contains(t, err.Error(), "elevenlabs list models")
		})
	}
}
