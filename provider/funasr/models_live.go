package funasr

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/utils"
)

// Compile-time: the transcribe surface carries the catalog endpoint.
var _ provider.ModelLister = (*TranscribeProvider)(nil)

// modelsPath answers the standard OpenAI {"object":"list","data":[...]}
// envelope, which utils.DecodeIDList understands directly.
const modelsPath = "/v1/models"

// ListModels implements provider.ModelLister against GET /v1/models.
//
// The live list is the server's models.json registry — deployment truth —
// so it decides membership; the bundled catalog supplies metadata for the
// ids it knows. The server only ever registers ASR models, so an id the
// bundle has never heard of still gets the transcribe capability rather
// than an empty spec.
func (p *TranscribeProvider) ListModels(ctx context.Context) ([]provider.ModelSpec, error) {
	headers := map[string]string{}
	if token := p.auth.Token(); token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	raw, err := utils.Fetch(ctx, p.client, p.baseURL+modelsPath, headers)
	if err != nil {
		return nil, fmt.Errorf("funasr list models: %w", err)
	}
	ids, err := utils.DecodeIDList(raw)
	if err != nil {
		return nil, fmt.Errorf("funasr list models: %w", err)
	}
	specs := utils.Merge(ids, DefaultCatalog())
	for i, spec := range specs {
		if len(spec.Capabilities) == 0 {
			specs[i].Capabilities = []provider.Capability{provider.CAPABILITY_TRANSCRIBE}
			specs[i].InputModalities = []provider.Modality{provider.MODALITY_AUDIO}
			specs[i].OutputModalities = []provider.Modality{provider.MODALITY_TEXT}
		}
	}
	return specs, nil
}
