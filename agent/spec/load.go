package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Decode reads a JSON config and prepares it — expand, then validate.
//
// JSON only, and that is the dependency discipline showing: spec imports
// core and the standard library, nothing else. YAML support lives in
// utils/configfile, which converts and hands the result here. Both paths
// share this validation, so a YAML file and a hand-built literal cannot
// diverge in what they accept.
//
// Unknown fields are rejected. A silently ignored `tools:` block under a
// misspelled key is the exact failure mode a config layer exists to
// prevent.
func Decode(r io.Reader) (Config, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var raw Config
	if err := dec.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("spec: decode: %w", err)
	}
	out, err := raw.Prepare()
	if err != nil {
		return Config{}, err
	}
	return out, nil
}

// DecodeBytes is Decode over a byte slice.
func DecodeBytes(b []byte) (Config, error) { return Decode(bytes.NewReader(b)) }

// Encode writes the config as indented JSON. It does not expand or
// validate: a wizard writes what the user chose, and round-tripping an
// expanded config back through Decode must be a fixed point.
func Encode(w io.Writer, c Config) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return fmt.Errorf("spec: encode: %w", err)
	}
	return nil
}

// EncodeBytes is Encode into a byte slice.
func EncodeBytes(c Config) ([]byte, error) {
	var buf bytes.Buffer
	if err := Encode(&buf, c); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
