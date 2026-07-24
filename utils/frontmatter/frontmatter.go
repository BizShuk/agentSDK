// Package frontmatter parses the "---" / "+++" / ";;;" delimited header
// shared by SKILL.md, slash commands, and subagent definitions. Decoding
// is delegated to github.com/adrg/frontmatter, which auto-detects YAML,
// TOML, or JSON front matter and unmarshals the header.
//
// The returned map keys are lower-cased. Decoded values are coerced to
// strings so the existing flat string-lookup call sites in skill and
// subagent keep working:
//
//   - string values are returned as-is
//   - list / sequence values are joined with ","
//   - everything else is formatted via fmt.Sprintf("%v", v)
//
// When the input has no delimited header, Parse returns an empty map and
// the original content as body, matching the previous in-tree parser.
package frontmatter

import (
	"fmt"
	"strings"

	adrg "github.com/adrg/frontmatter"
)

// DELIMITER is the YAML delimiter that adrg/frontmatter recognises by
// default. Kept exported so callers / tests can reference the same
// literal without re-declaring it.
const DELIMITER = "---"

// Parse splits content into frontmatter fields and the remaining body.
// The returned map is keyed by lower-cased field name; values are
// stringified (see package doc). A decoding error from the underlying
// library is returned verbatim.
func Parse(content string) (map[string]string, string, error) {
	fields := map[string]string{}
	var raw map[string]any
	body, err := adrg.Parse(strings.NewReader(content), &raw)
	if err != nil {
		return nil, content, err
	}
	for k, v := range raw {
		fields[strings.ToLower(k)] = stringify(v)
	}
	return fields, string(body), nil
}

// stringify flattens a YAML/TOML/JSON-decoded value to a single string.
// Lists are joined with "," so callers that previously split on
// `[a, b]` or `a, b` keep the same observable shape.
func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		parts := make([]string, 0, len(x))
		for _, p := range x {
			parts = append(parts, stringify(p))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// List splits a frontmatter value into items: "[a, b]" or "a, b" → [a b].
// Kept for callers that already operate on a raw string; new code can
// rely on Parse's comma-joined form and skip List entirely.
func List(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}