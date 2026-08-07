package funasr

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/provider"
)

func TestListModelsMergesLiveWithCatalog(t *testing.T) {
	transcriber := newTestTranscriber(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		_, _ = w.Write([]byte(`{"object":"list","data":[
			{"id":"sensevoice","object":"model","owned_by":"funasr","ready":false},
			{"id":"qwen3-asr-0.6b","object":"model","owned_by":"funasr","ready":true},
			{"id":"custom-lab-model","object":"model","owned_by":"funasr","ready":false}
		]}`))
	})

	specs, err := transcriber.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, specs, 3, "live list decides membership")

	byID := map[string]provider.ModelSpec{}
	for _, spec := range specs {
		byID[spec.ID] = spec
	}
	// Catalog-known ids keep bundled metadata.
	assert.Equal(t, "sensevoice", byID["sensevoice"].Family)
	assert.Equal(t, "qwen3-asr", byID["qwen3-asr-0.6b"].Family)
	// Unknown deployment-local ids still carry the transcribe capability.
	custom := byID["custom-lab-model"]
	assert.Equal(t, []provider.Capability{provider.CAPABILITY_TRANSCRIBE}, custom.Capabilities)
	assert.Equal(t, []provider.Modality{provider.MODALITY_AUDIO}, custom.InputModalities)
	assert.Equal(t, []provider.Modality{provider.MODALITY_TEXT}, custom.OutputModalities)
}

func TestRegistryTranscriberPreservesModelLister(t *testing.T) {
	transcriber, err := provider.NewTranscriber("funasr", provider.Options{
		LookupEnv: func(string) string { return "" },
	})
	require.NoError(t, err)
	_, ok := transcriber.(provider.ModelLister)
	assert.True(t, ok, "accounting/decorator wrap must keep ModelLister discoverable")
}

func TestListModelsErrorsOnEmptyList(t *testing.T) {
	transcriber := newTestTranscriber(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	})
	_, err := transcriber.ListModels(context.Background())
	assert.ErrorContains(t, err, "no model ids")
}
