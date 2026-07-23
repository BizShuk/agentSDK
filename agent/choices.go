// Package agent is the assembly layer between the declarative spec and
// the runtime Engine. It reads a spec.Config, builds every capability the
// config asked for, and hands back a wired *runtime.Engine.
//
// The split from spec is deliberate: spec is data that anything can read
// without dependencies, agent is the code that knows how to turn that
// data into objects — which providers are compiled in, what order the
// middleware chain goes in, which port each harness package plugs into.
package agent

import (
	"strings"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/provider/registry"
)

// The spec types are aliased so an application importing only agent can
// still write agent.Config{...}. They are the same types — the
// definitions live in spec, which stays importable on its own by anything
// that reads or produces a config without building one.
type (
	Config = spec.Config
	Choice = spec.Choice

	Model      = spec.Model
	Reasoning  = spec.Reasoning
	Limits     = spec.Limits
	Middleware = spec.Middleware
	Memory     = spec.Memory
	Tools      = spec.Tools
	Safety     = spec.Safety
	Prompt     = spec.Prompt
	Skills     = spec.Skills
	Subagents  = spec.Subagents
	Sessions   = spec.Sessions
	Output     = spec.Output
	Telemetry  = spec.Telemetry
)

// ProviderChoices lists the provider adapters compiled into this binary.
//
// It lives here rather than in spec because it is the one choice list
// that needs compile-time knowledge: spec can enumerate reasoning styles
// from core's constants and tiers from its own vocabulary, but "which
// adapters are linked in" is only answerable by the package that links
// them.
func ProviderChoices() []Choice {
	entries := registry.Entries()
	out := make([]Choice, 0, len(entries))
	for _, e := range entries {
		out = append(out, Choice{
			Value:   e.Name,
			Label:   e.Label,
			Note:    providerNote(e),
			Default: e.Name == registry.DEFAULT,
		})
	}
	return out
}

// providerNote combines the entry's own note with the credential it reads,
// so a wizard can tell the user what to export before they pick.
func providerNote(e registry.Entry) string {
	var parts []string
	if e.Note != "" {
		parts = append(parts, e.Note)
	}
	if len(e.APIKeyEnv) > 0 {
		parts = append(parts, "reads "+strings.Join(e.APIKeyEnv, " or "))
	}
	return strings.Join(parts, "; ")
}
