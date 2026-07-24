package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bizshuk/agentsdk/agent/spec"
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

// LoadFile reads a config file and prepares it — expand, then validate.
//
// YAML is converted through JSON rather than given its own struct tags.
// Two tag sets would be two sources of truth for every key name, and they
// would drift the first time someone adds a field. This way the `json`
// tags govern both encodings by construction.
func LoadFile(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("agent: read %s: %w", path, err)
	}
	if FormatOf(path) == FORMAT_JSON {
		return spec.DecodeBytes(raw)
	}
	jsonBytes, err := yamlToJSON(raw)
	if err != nil {
		return Config{}, fmt.Errorf("agent: parse %s: %w", path, err)
	}
	return spec.DecodeBytes(jsonBytes)
}

// Marshal encodes a config in the requested format. It does not expand or
// validate: a wizard writes what the user chose, and re-reading it must
// be a fixed point.
func Marshal(cfg Config, f Format) ([]byte, error) {
	jsonBytes, err := spec.EncodeBytes(cfg)
	if err != nil {
		return nil, err
	}
	if f == FORMAT_JSON {
		return jsonBytes, nil
	}
	return jsonToYAML(jsonBytes)
}

// SaveFile writes a config. It refuses to clobber an existing file unless
// force is set — a wizard run that silently overwrote a hand-tuned config
// would be worse than one that failed.
func SaveFile(path string, cfg Config, force bool) error {
	absPath := path
	if abs, err := filepath.Abs(path); err == nil {
		absPath = abs
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("agent: %s already exists (pass force to overwrite)", absPath)
		}
	}
	out, err := Marshal(cfg, FormatOf(path))
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("agent: write %s: %w", absPath, err)
	}
	return nil
}

// yamlToJSON reads YAML into a generic value and re-encodes it as JSON, so
// the spec decoder sees the same key names either way — including its
// unknown-field check.
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
		return nil, fmt.Errorf("agent: encode yaml: %w", err)
	}
	return out, nil
}
