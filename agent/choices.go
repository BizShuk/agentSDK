// Package agent is the assembly layer between the declarative spec and
// the runtime Engine. It reads a spec.Config, builds every capability the
// config asked for, and hands back a wired *runtime.Engine.
//
// The split from spec is deliberate: spec is data that anything can read
// without dependencies, agent is the code that knows how to turn that
// data into objects — which reasoning styles to register, how the
// engine gets wired, how a Host is built.
//
// Wizard UI vocabulary (Choice, Label/Note composition) lives in
// cmd/agent/wizard; raw registry access (provider.Entries /
// provider.Catalog) lives in the provider package itself — agent does
// not pass those through.
package agent

import (
	"github.com/bizshuk/agentsdk/agent/spec"
)

// The spec types are aliased so an application importing only agent can
// still write agent.Config{...}. They are the same types — the
// definitions live in spec, which stays importable on its own by anything
// that reads or produces a config without building one.
type (
	Config     = spec.Config
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
