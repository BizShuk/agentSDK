package builtin

// ReadOption customizes a Read tool instance.
type ReadOption func(*Read)

// WithReadMaxBytes sets the maximum bytes read per call.
func WithReadMaxBytes(maxBytes int64) ReadOption {
	return func(r *Read) {
		if maxBytes > 0 {
			r.maxBytes = maxBytes
		}
	}
}
