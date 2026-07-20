// Package modelsapi holds the shared plumbing every provider adapter uses
// to enumerate models from its upstream's live catalog endpoint instead of
// a hand-maintained constant list.
//
// Two wire shapes cover every adapter in this repo:
//
//   - OpenAI / Anthropic style — GET {base}/models returning
//     {"data":[{"id":"..."}]}. Used by anthropic, minimax, grok, ollama
//     and any OpenAI-compatible endpoint.
//   - Google native — GET {base}/models returning {"models":[...]} with
//     per-model token limits. Decoded inside provider/google since only
//     that surface carries the extra fields.
//
// The live call is the source of truth for WHICH models exist; the
// bundled DefaultCatalog stays as the source of metadata (family,
// reasoning flag, context window) that the list endpoints do not report,
// and as the offline fallback. Merge stitches the two together.
package modelsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bizshuk/agentsdk/core"
)

// MAX_BODY_BYTES caps how much of a catalog response we read. Catalogs are
// a few KB; the cap stops a misconfigured base URL (an HTML error page, a
// captive portal) from being buffered without bound.
const MAX_BODY_BYTES = 4 << 20 // 4 MiB

// MAX_ERROR_BYTES caps how much of a non-2xx body is quoted back in the
// returned error. Enough to identify the failure, short enough that the
// message stays readable in a terminal.
const MAX_ERROR_BYTES = 512

// Fetch performs a GET against url with the supplied headers and returns
// the response body. Non-2xx responses become an error carrying the status
// code and a truncated body excerpt so callers can tell "bad key" from
// "wrong endpoint" without a debugger.
//
// The caller owns the client (and therefore the timeout); ctx cancellation
// is honored for the whole exchange.
func Fetch(ctx context.Context, client *http.Client, url string, headers map[string]string) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, MAX_BODY_BYTES))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("get %s: status %d: %s", url, resp.StatusCode, excerpt(raw))
	}
	return raw, nil
}

// excerpt truncates a response body for inclusion in an error message.
func excerpt(raw []byte) string {
	if len(raw) > MAX_ERROR_BYTES {
		return string(raw[:MAX_ERROR_BYTES]) + "…"
	}
	return string(raw)
}

// idListResponse is the shape OpenAI, Anthropic and every compatible
// gateway return from GET /models. Fields beyond `id` differ per vendor
// (display_name, created, owned_by) and are deliberately ignored — the
// bundled catalog is a better metadata source than any of them.
type idListResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// DecodeIDList pulls the model ids out of an OpenAI/Anthropic-style
// catalog response. An empty `data` array is an error rather than an empty
// slice: a catalog with no models means the endpoint answered but is not
// the catalog we asked for, and silently returning nothing would look
// identical to a provider that genuinely serves no models.
func DecodeIDList(raw []byte) ([]string, error) {
	var body idListResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("decode model list: %w", err)
	}
	ids := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("decode model list: response carried no model ids")
	}
	return ids, nil
}

// Merge overlays the live id list onto the bundled catalog's metadata.
//
// Live ids drive both membership and order — a model the upstream dropped
// disappears, a model it added shows up immediately. For ids the bundled
// catalog knows, the full ModelSpec is reused so family / reasoning /
// context-window survive; for ids it does not, the spec carries the id
// alone, which is honest about what the endpoint actually told us.
func Merge(ids []string, static []core.ModelSpec) []core.ModelSpec {
	known := make(map[string]core.ModelSpec, len(static))
	for _, s := range static {
		known[s.ID] = s
	}
	out := make([]core.ModelSpec, 0, len(ids))
	for _, id := range ids {
		if spec, ok := known[id]; ok {
			out = append(out, spec)
			continue
		}
		out = append(out, core.ModelSpec{ID: id})
	}
	return out
}
