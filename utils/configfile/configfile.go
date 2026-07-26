// Package configfile reads and writes config files whose on-disk encoding
// is chosen by extension, presenting every file to the caller as JSON.
//
// The JSON pivot is the point, not an accident of implementation. A caller
// decodes with encoding/json, so its `json` struct tags govern both
// encodings by construction. A second `yaml` tag set would be a second
// source of truth for every key name and would drift the first time
// someone adds a field — and the two encoders do not even agree on what
// "empty" means, so the drift would be silent in both directions.
//
// The package holds no domain knowledge: it moves bytes between formats
// and the filesystem, and the caller owns decoding and validation.
package configfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format is a config file's on-disk encoding.
type Format string

const (
	FORMAT_YAML Format = "yaml"
	FORMAT_JSON Format = "json"
)

// FormatOf reports the encoding a path implies. Anything that is not
// .json is treated as YAML, which makes an extensionless path (or a
// stdout "-") produce the friendlier format.
func FormatOf(path string) Format {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return FORMAT_JSON
	}
	return FORMAT_YAML
}

// ReadJSON reads a config file and returns its content as JSON, whatever
// the on-disk encoding was.
//
// The YAML branch round-trips through a generic value so the caller's
// decoder sees the same key names either way — including its
// unknown-field check.
func ReadJSON(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("configfile: read %s: %w", path, err)
	}
	if FormatOf(path) == FORMAT_JSON {
		return raw, nil
	}
	out, err := yamlToJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("configfile: parse %s: %w", path, err)
	}
	return out, nil
}

// Marshal re-encodes JSON in the requested format.
func Marshal(jsonBytes []byte, f Format) ([]byte, error) {
	if f == FORMAT_JSON {
		return jsonBytes, nil
	}
	return jsonToYAML(jsonBytes)
}

// Write encodes JSON in the format the path implies and writes it. It
// refuses to clobber an existing file unless force is set — a generator
// run that silently overwrote a hand-tuned config would be worse than one
// that failed.
func Write(path string, jsonBytes []byte, force bool) error {
	absPath := path
	if abs, err := filepath.Abs(path); err == nil {
		absPath = abs
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("configfile: %s already exists (pass force to overwrite)", absPath)
		}
	}
	out, err := Marshal(jsonBytes, FormatOf(path))
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("configfile: write %s: %w", absPath, err)
	}
	return nil
}

// yamlToJSON reads YAML into a generic value and re-encodes it as JSON.
// An empty document becomes an empty object rather than JSON null, so a
// caller decoding into a struct gets zero values instead of an error.
func yamlToJSON(in []byte) ([]byte, error) {
	var v any
	if err := yaml.Unmarshal(in, &v); err != nil {
		return nil, err
	}
	if v == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(v)
}

func jsonToYAML(in []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, err
	}
	out, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("configfile: encode yaml: %w", err)
	}
	return out, nil
}
