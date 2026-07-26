package builtin

// GlobOption customizes a Glob tool instance.
type GlobOption func(*Glob)

// WithGlobMaxMatches sets the maximum matching files returned.
func WithGlobMaxMatches(max int) GlobOption {
	return func(g *Glob) {
		if max > 0 {
			g.maxMatches = max
		}
	}
}
