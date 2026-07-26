package builtin

// GrepOption customizes a Grep tool instance.
type GrepOption func(*Grep)

// WithGrepMaxResults sets the maximum matching lines returned.
func WithGrepMaxResults(max int) GrepOption {
	return func(gr *Grep) {
		if max > 0 {
			gr.maxResults = max
		}
	}
}
