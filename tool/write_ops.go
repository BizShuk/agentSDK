package tool

// WriteOptions tunes the Write tool.
type WriteOptions struct {
	// DefaultMode is the file permission applied when creating a new file.
	// 0 means 0o644.
	DefaultMode int
}
