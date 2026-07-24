package tool

// GrepOptions tunes the Grep tool.
type GrepOptions struct {
	// MaxResults caps the returned matches. 0 = 100.
	MaxResults int
}
