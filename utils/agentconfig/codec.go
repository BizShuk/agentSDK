package agentconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/bizshuk/agentsdk/agent/spec"
)

// Decode reads a JSON config and prepares it — expand, then validate.
//
// The codec lives here rather than in spec, and that is the dependency
// discipline showing: spec is data plus its own semantics (Expand,
// Validate, Prepare), so its imports are core and a handful of pure
// stdlib packages — encoding/json is no longer among them. Turning bytes
// into a Config is I/O, and I/O is this package's job. The `json` struct
// tags stay on spec.Config because they describe the data, not a codec.
//
// JSON only. YAML reaches this function through configfile, which
// converts and hands the result here, so the `json` tags in spec govern
// both encodings by construction. Both paths share spec's validation, so
// a YAML file and a hand-built literal cannot diverge in what they accept.
//
// Unknown fields are rejected. A silently ignored `tools:` block under a
// misspelled key is the exact failure mode a config layer exists to
// prevent.
func Decode(r io.Reader) (spec.Config, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var raw spec.Config
	if err := dec.Decode(&raw); err != nil {
		return spec.Config{}, fmt.Errorf("agentconfig: decode: %w", err)
	}
	out, err := raw.Prepare()
	if err != nil {
		return spec.Config{}, err
	}
	return out, nil
}

// DecodeBytes is Decode over a byte slice.
func DecodeBytes(b []byte) (spec.Config, error) { return Decode(bytes.NewReader(b)) }

// Encode writes the config as indented JSON. It does not expand or
// validate: a wizard writes what the user chose, and round-tripping an
// expanded config back through Decode must be a fixed point.
func Encode(w io.Writer, c spec.Config) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return fmt.Errorf("agentconfig: encode: %w", err)
	}
	return nil
}

// EncodeBytes is Encode into a byte slice.
func EncodeBytes(c spec.Config) ([]byte, error) {
	var buf bytes.Buffer
	if err := Encode(&buf, c); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
