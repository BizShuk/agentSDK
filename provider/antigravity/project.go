package antigravity

// Cloud Code addresses every request to a project. The IDE learns its
// project by calling loadCodeAssist once at startup and reusing the
// answer; this file does the same.
//
// Discovery is best-effort by design. A brand-new account has no project
// provisioned yet and the endpoint answers without one, so falling back
// to the sentinel the reference clients use keeps the first call working
// instead of failing on a field the user cannot supply.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
)

// LOAD_CODE_ASSIST_MODE is the mode value the IDE sends. It selects the
// "resolve my entitlement" behaviour rather than an onboarding flow.
const LOAD_CODE_ASSIST_MODE = 1

// clientMetadata is the IDE identity loadCodeAssist keys entitlement on.
// The gateway takes the enum values as numbers.
type clientMetadata struct {
	IDEType    int `json:"ideType"`
	Platform   int `json:"platform"`
	PluginType int `json:"pluginType"`
}

// loadCodeAssistRequest is the discovery call body.
type loadCodeAssistRequest struct {
	Metadata clientMetadata `json:"metadata"`
	Mode     int            `json:"mode"`
}

// loadCodeAssistResponse carries the project id. The response also
// reports subscription tiers, which this adapter has no use for —
// entitlement shows up as the model list, and that comes from
// fetchAvailableModels.
//
// cloudaicompanionProject arrives either as a bare string or as an object
// with an id, depending on provisioning state, so it is decoded loosely.
type loadCodeAssistResponse struct {
	Project json.RawMessage `json:"cloudaicompanionProject"`
}

// ProjectID resolves the Cloud Code project for this Provider.
//
// Precedence: an id pinned by the caller, then the cached discovery
// result, then a live loadCodeAssist call, then the sentinel. Discovery
// runs at most once per Provider even when it falls through to the
// sentinel — a second attempt would fail the same way and cost a
// round-trip on every request.
func (p *Provider) ProjectID(ctx context.Context, override core.Auth) (string, error) {
	if p.pinned != "" {
		return p.pinned, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.discovered != "" {
		return p.discovered, nil
	}

	id, err := p.loadCodeAssist(ctx, override)
	if err != nil || id == "" {
		// The error is deliberately swallowed: an account without a
		// provisioned project is the normal first-run state, not a
		// failure the caller can act on.
		id = DefaultProjectID
	}
	p.discovered = id
	return id, nil
}

// loadCodeAssist performs the discovery round-trip.
func (p *Provider) loadCodeAssist(ctx context.Context, override core.Auth) (string, error) {
	raw, err := p.post(ctx, PATH_LOAD_CODE_ASSIST, loadCodeAssistRequest{
		Metadata: clientMetadata{
			IDEType:    IDE_TYPE_ANTIGRAVITY,
			Platform:   platformEnum(),
			PluginType: PLUGIN_TYPE_GEMINI,
		},
		Mode: LOAD_CODE_ASSIST_MODE,
	}, override, "")
	if err != nil {
		return "", err
	}
	var body loadCodeAssistResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", fmt.Errorf("antigravity: decode loadCodeAssist: %w", err)
	}
	return decodeProject(body.Project), nil
}

// decodeProject accepts both shapes the field takes.
func decodeProject(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asObject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return asObject.ID
	}
	return ""
}
