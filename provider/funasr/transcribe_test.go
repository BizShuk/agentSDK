package funasr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/provider"
)

func newTestTranscriber(t *testing.T, handler http.HandlerFunc) *TranscribeProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	transcriber, err := NewTranscriber(provider.ResolvedConfig{BaseURL: server.URL})
	require.NoError(t, err)
	return transcriber
}

func TestTranscribeEncodesMultipartAndFoldsSegments(t *testing.T) {
	transcriber := newTestTranscriber(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/audio/transcriptions", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		assert.Equal(t, "paraformer-zh", r.FormValue("model"))
		assert.Equal(t, "verbose_json", r.FormValue("response_format"))
		assert.Equal(t, "zh", r.FormValue("language"))
		assert.Empty(t, r.Header.Get("Authorization"), "keyless by default")

		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		defer file.Close()
		assert.Equal(t, "audio.wav", header.Filename)

		speaker := 1
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"text":     "你好 世界",
			"language": "zh",
			"duration": 0.123, // server processing time — must NOT become usage
			"segments": []map[string]any{
				{"text": "你好", "start": 0.0, "end": 0.5, "speaker": nil},
				{"text": "世界", "start": 0.5, "end": 1.25, "speaker": speaker},
			},
		}))
	})

	result, err := transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Model:    "paraformer-zh",
		Language: "zh",
		Audio:    provider.AudioSource{Bytes: []byte("RIFF"), Format: "wav"},
	})
	require.NoError(t, err)
	assert.Equal(t, "你好 世界", result.Text)
	assert.Equal(t, "zh", result.Language)
	require.Len(t, result.Words, 2)
	assert.Equal(t, provider.TranscribedWord{Text: "你好", StartMs: 0, EndMs: 500}, result.Words[0])
	assert.Equal(t,
		provider.TranscribedWord{Text: "世界", StartMs: 500, EndMs: 1250, Speaker: "1"},
		result.Words[1],
	)
	assert.Zero(t, result.Usage.AudioDurationMilliseconds)
}

func TestTranscribeDefaultsModelAndDropsAutoLanguage(t *testing.T) {
	transcriber := newTestTranscriber(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		assert.Equal(t, DefaultTranscribeModel, r.FormValue("model"))
		assert.Empty(t, r.FormValue("language"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"text": "hello", "language": "auto",
		}))
	})

	result, err := transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{Bytes: []byte("RIFF"), Format: "wav"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Text)
	assert.Empty(t, result.Language, "server 'auto' placeholder folds to empty")
	assert.Empty(t, result.Words)
}

func TestTranscribeRejectsUnrepresentableFields(t *testing.T) {
	transcriber, err := NewTranscriber(provider.ResolvedConfig{})
	require.NoError(t, err)

	_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{URL: "https://example.com/a.wav"},
	})
	assert.ErrorContains(t, err, "audio URL input is not supported")

	_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio:   provider.AudioSource{Bytes: []byte("RIFF")},
		Diarize: true,
	})
	assert.ErrorContains(t, err, "diarize is not a request-time option")
}

func TestTranscribeDecodesFastAPIError(t *testing.T) {
	transcriber := newTestTranscriber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail": "Model 'nope' not found. Available: ['sensevoice']"}`))
	})

	_, err := transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Model: "nope",
		Audio: provider.AudioSource{Bytes: []byte("RIFF"), Format: "wav"},
	})
	var apiErr *provider.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, "funasr", apiErr.Provider)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Contains(t, apiErr.Message, "Model 'nope' not found")
}

func TestRegistryBuildsTranscriberWithoutCredential(t *testing.T) {
	transcriber, err := provider.NewTranscriber("funasr", provider.Options{
		LookupEnv: func(string) string { return "" },
	})
	require.NoError(t, err)
	assert.NotNil(t, transcriber)
}

func TestDefaultCatalogIsTranscribeOnly(t *testing.T) {
	for _, spec := range DefaultCatalog() {
		assert.Equal(t, []provider.Capability{provider.CAPABILITY_TRANSCRIBE},
			spec.Capabilities, spec.ID)
		assert.True(t, spec.Supports(provider.CAPABILITY_TRANSCRIBE), spec.ID)
	}
}
