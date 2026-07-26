package builtin

// WriteOption customizes a Write tool instance.
type WriteOption func(*Write)

// WithWriteDefaultMode sets the default file mode permission for new files.
func WithWriteDefaultMode(mode int) WriteOption {
	return func(w *Write) {
		if mode > 0 {
			w.mode = mode
		}
	}
}
