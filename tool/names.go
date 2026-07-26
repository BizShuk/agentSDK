package tool

// Built-in tool names. Each name is the single source of truth: the
// switch in tool/tool.go that registers them, the config allowlist
// (spec.Tools.Builtin) that selects which to enable, and the runtime
// switch in agent/build.go that maps a configured name to its
// constructor. Renaming any of these is a breaking change for the
// agent scaffold and for users running the provider CLI's flag surface.
const (
	NAME_READ  = "read"
	NAME_WRITE = "write"
	NAME_EDIT  = "edit"
	NAME_BASH  = "bash"
	NAME_GLOB  = "glob"
	NAME_GREP  = "grep"
)

// BuiltinNames lists every built-in tool. The slice doubles as the
// default allowlist when Config.Tools.Builtin is unset, and as the
// fallback registerDefaults loop over each name.
func BuiltinNames() []string {
	return []string{NAME_READ, NAME_WRITE, NAME_EDIT, NAME_BASH, NAME_GLOB, NAME_GREP}
}
