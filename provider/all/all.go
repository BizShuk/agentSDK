// Package all links every built-in provider adapter into the binary.
//
// Adapters self-register from their package init(); this meta-package
// blank-imports them so any binary that wants the full set can do
//
//	import _ "github.com/bizshuk/agentsdk/provider/all"
//
// Slim binaries (a sample that only ships ollama, an embedded use case
// that only ships anthropic) should import the specific adapter
// packages instead and let Go's linker drop the rest.
package all

import (
	// Built-in adapters. Add new providers here.
	_ "github.com/bizshuk/agentsdk/provider/anthropic"
	_ "github.com/bizshuk/agentsdk/provider/antigravity"
	_ "github.com/bizshuk/agentsdk/provider/codex"
	_ "github.com/bizshuk/agentsdk/provider/elevenlabs"
	_ "github.com/bizshuk/agentsdk/provider/funasr"
	_ "github.com/bizshuk/agentsdk/provider/google"
	_ "github.com/bizshuk/agentsdk/provider/grok"
	_ "github.com/bizshuk/agentsdk/provider/minimax"
	_ "github.com/bizshuk/agentsdk/provider/ollama"
)
