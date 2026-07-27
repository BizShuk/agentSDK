package builtin

import (
	"path/filepath"

	"github.com/bizshuk/agentsdk/tool"
)

// testPolicy returns a Policy that allows paths under dirs.
func testPolicy(dirs ...string) *tool.Policy {
	p := tool.DefaultPolicy()
	for _, d := range dirs {
		p.AllowedPathPrefixes = append(p.AllowedPathPrefixes, d)
		if resolved, err := filepath.EvalSymlinks(d); err == nil && resolved != d {
			p.AllowedPathPrefixes = append(p.AllowedPathPrefixes, resolved)
		}
	}
	return p
}
