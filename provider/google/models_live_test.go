package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nativeCatalogJSON is a trimmed Google native /models response carrying one
// chat model, one embedding model, and one Imagen model — enough to prove
// the generateContent filter and the metadata merge.
const nativeCatalogJSON = `{
  "models": [
    {
      "name": "models/gemini-3-flash-preview",
      "displayName": "Gemini 3 Flash Preview",
      "inputTokenLimit": 1048576,
      "outputTokenLimit": 65536,
      "supportedGenerationMethods": ["generateContent", "countTokens"]
    },
    {
      "name": "models/gemini-embedding-001",
      "displayName": "Gemini Embedding 001",
      "inputTokenLimit": 2048,
      "outputTokenLimit": 1,
      "supportedGenerationMethods": ["embedContent"]
    },
    {
      "name": "models/gemini-9-ultra-preview",
      "displayName": "Future model not in bundled catalog",
      "inputTokenLimit": 2000000,
      "outputTokenLimit": 128000,
      "supportedGenerationMethods": ["generateContent"]
    }
  ]
}`

func TestListModels(t *testing.T) {
	var gotPath, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-goog-api-key")
		_, _ = w.Write([]byte(nativeCatalogJSON))
	}))
	defer srv.Close()

	// Point the OpenAI-compat base at the fake server's /openai path so we
	// also exercise nativeBaseURL's suffix trimming.
	prov, err := New(WithAPIKey("k"), WithBaseURL(srv.URL+"/openai"))
	require.NoError(t, err)

	specs, err := prov.ListModels(context.Background())
	require.NoError(t, err)

	// The native (not /openai) path is queried.
	assert.Equal(t, "/models", gotPath)
	assert.Equal(t, "k", gotKey)

	// The embedding model is filtered out; the two generateContent models remain.
	ids := make([]string, len(specs))
	for i, s := range specs {
		ids[i] = s.ID
	}
	assert.Equal(t, []string{"gemini-3-flash-preview", "gemini-9-ultra-preview"}, ids)

	// Known id keeps bundled metadata but takes token limits from the API.
	known := specs[0]
	assert.Equal(t, "gemini-flash", known.Family)
	assert.Equal(t, 1048576, known.ContextWindow)
	assert.Equal(t, 65536, known.MaxTokens)

	// Unknown id infers a coarse family and defaults to text input.
	unknown := specs[1]
	assert.Equal(t, "gemini", unknown.Family)
	assert.Equal(t, 2000000, unknown.ContextWindow)
	assert.Equal(t, []core.Modality{core.MODALITY_TEXT}, unknown.Input)
}

func TestListModelsEmptyIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"models/only-embed","supportedGenerationMethods":["embedContent"]}]}`))
	}))
	defer srv.Close()

	prov, err := New(WithAPIKey("k"), WithBaseURL(srv.URL))
	require.NoError(t, err)

	_, err = prov.ListModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no generateContent-capable models")
}
