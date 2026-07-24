package tool

import "github.com/bizshuk/agentsdk/action"

// Options configure the built-in tool set. Zero value means "safe defaults"
// — Read/Glob/Grep unrestricted, Write/Edit/Bash will error if Policy is nil.
type Options struct {
	// Policy is the sandbox consulted by every tool before accessing the
	// filesystem or executing a command. Required for Write/Edit/Bash;
	// optional for Read/Glob/Grep (nil means no path check).
	Policy action.Sandbox

	// WorkingDir is the base directory for tools that accept a --cwd or
	// relative path argument. Defaults to "." (process CWD).
	WorkingDir string

	Read  ReadOptions
	Write WriteOptions
	Edit  EditOptions
	Bash  BashOptions
	Glob  GlobOptions
	Grep  GrepOptions
}
