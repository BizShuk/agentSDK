package agent

import (
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/utils/configfile"
)

// Format is a config file's on-disk encoding.
type Format = configfile.Format

const (
	FORMAT_YAML = configfile.FORMAT_YAML
	FORMAT_JSON = configfile.FORMAT_JSON
)

// FormatOf reports the encoding a path implies. Anything that is not
// .json is treated as YAML, which makes an extensionless path (or a
// stdout "-") produce the friendlier format.
func FormatOf(path string) Format { return configfile.FormatOf(path) }

// LoadFile reads a config file and prepares it — expand, then validate.
//
// YAML reaches spec through JSON rather than through its own struct tags:
// configfile converts, and the `json` tags in spec govern both encodings
// by construction. Both paths share spec's validation, so a YAML file and
// a hand-built literal cannot diverge in what they accept.
func LoadFile(path string) (Config, error) {
	raw, err := configfile.ReadJSON(path)
	if err != nil {
		return Config{}, err
	}
	return spec.DecodeBytes(raw)
}

// Marshal encodes a config in the requested format. It does not expand or
// validate: a wizard writes what the user chose, and re-reading it must
// be a fixed point.
func Marshal(cfg Config, f Format) ([]byte, error) {
	jsonBytes, err := spec.EncodeBytes(cfg)
	if err != nil {
		return nil, err
	}
	return configfile.Marshal(jsonBytes, f)
}

// SaveFile writes a config. It refuses to clobber an existing file unless
// force is set — a wizard run that silently overwrote a hand-tuned config
// would be worse than one that failed.
func SaveFile(path string, cfg Config, force bool) error {
	jsonBytes, err := spec.EncodeBytes(cfg)
	if err != nil {
		return err
	}
	return configfile.Write(path, jsonBytes, force)
}
