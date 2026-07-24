package tool

// ReadOptions tunes the Read tool.
type ReadOptions struct {
	// MaxBytes caps the bytes read per call. 0 = 1 MiB.
	MaxBytes int64
}
