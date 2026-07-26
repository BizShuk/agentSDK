package credential

import (
	"fmt"
	"sort"

	"github.com/bizshuk/agentsdk/core"
	authprovider "github.com/bizshuk/auth/provider"
)

// This file holds the one translation this repository owes the auth
// module: our registry name plus a credential kind, into auth's route id.
//
// The two vocabularies genuinely differ, and not only in spelling:
//
//	agentsdk names the ADAPTER          auth names the VENDOR ACCOUNT
//	  codex   (Codex Responses wire)      openai
//	  grok    (xAI Grok)                  xai
//
// and agentsdk keeps credential kind as a SEPARATE AXIS
// (spec.Model.Provider + spec.Model.CredentialKind) where auth flattens it
// into the id (`anthropic` vs `anthropic_oauth`). The axis is the better
// model for configuration — it lets a config say "anthropic, but force the
// API key" without inventing a second provider name — so the flattening
// stops here. Nothing above this package should ever see "anthropic_oauth".

// route names one (adapter, credential kind) pair's auth id.
type route struct {
	name string // agentsdk registry name
	kind string // core.CREDENTIAL_KIND_*
}

// routes is the whole mapping. A pair that is absent is not supported by
// the auth module, which is a different condition from "the adapter has no
// such credential kind" — hence the explicit error rather than a fallback.
var routes = map[route]string{
	{"anthropic", core.CREDENTIAL_KIND_APIKEY}:  authprovider.ANTHROPIC,
	{"anthropic", core.CREDENTIAL_KIND_OAUTH}:   authprovider.ANTHROPIC_OAUTH,
	{"codex", core.CREDENTIAL_KIND_APIKEY}:      authprovider.OPENAI,
	{"codex", core.CREDENTIAL_KIND_OAUTH}:       authprovider.OPENAI_OAUTH,
	{"grok", core.CREDENTIAL_KIND_APIKEY}:       authprovider.XAI,
	{"grok", core.CREDENTIAL_KIND_OAUTH}:        authprovider.XAI_OAUTH,
	{"antigravity", core.CREDENTIAL_KIND_OAUTH}: authprovider.ANTIGRAVITY,
	{"google", core.CREDENTIAL_KIND_APIKEY}:     authprovider.GOOGLE,
}

// RouteID resolves an agentsdk provider name and credential kind to the
// auth module's route id.
//
// An empty kind means CREDENTIAL_KIND_AUTO, which has no single answer
// here: "let precedence decide" is a question for the resolver, not for a
// static table. Callers that want auto should use Resolve, which consults
// the store and the environment, rather than asking for a route.
func RouteID(name, kind string) (string, error) {
	if kind == core.CREDENTIAL_KIND_AUTO {
		return "", fmt.Errorf("credential: provider %q: a specific credential kind is required to name an auth route (got auto)", name)
	}
	id, ok := routes[route{name, kind}]
	if !ok {
		return "", fmt.Errorf("credential: provider %q has no %s route in the auth module (supported: %s)",
			name, kind, joinKinds(name))
	}
	return id, nil
}

// Kinds lists the credential kinds the auth module can supply for one
// provider name, sorted. Empty when the provider is unknown to auth — an
// adapter can still work from environment variables, it just has no
// stored-credential path.
func Kinds(name string) []string {
	var out []string
	for r := range routes {
		if r.name == name {
			out = append(out, r.kind)
		}
	}
	sort.Strings(out)
	return out
}

// Names lists every agentsdk provider name that has at least one auth
// route, sorted.
func Names() []string {
	seen := map[string]bool{}
	for r := range routes {
		seen[r.name] = true
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func joinKinds(name string) string {
	ks := Kinds(name)
	if len(ks) == 0 {
		return "none"
	}
	s := ks[0]
	for _, k := range ks[1:] {
		s += ", " + k
	}
	return s
}
