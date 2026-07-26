package builtin

import (
	"github.com/bizshuk/agentsdk/tool"
)

// Options configure the built-in tool set. Zero value means "safe defaults"
// — Read/Glob/Grep unrestricted, Write/Edit/Bash will error if Policy is nil.
type Options struct {
	// Policy is the sandbox consulted by every tool before accessing the
	// filesystem or executing a command. Required for Write/Edit/Bash;
	// optional for Read/Glob/Grep (nil means no path check).
	Policy tool.Sandbox

	// WorkingDir is the base directory for tools that accept a --cwd or
	// relative path argument. Defaults to "." (process CWD).
	WorkingDir string

	ReadOpts  []ReadOption
	WriteOpts []WriteOption
	EditOpts  []EditOption
	BashOpts  []BashOption
	GlobOpts  []GlobOption
	GrepOpts  []GrepOption
}
