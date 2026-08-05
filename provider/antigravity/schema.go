package antigravity

// Tool schemas cross a dialect boundary here. agentsdk tools describe
// their arguments in JSON Schema; Cloud Code validates them against
// Google's protobuf Schema message, which is a small subset with
// UPPERCASE type names. Sending plain JSON Schema through is not a
// tolerated superset — the gateway rejects the whole request with
// "Proto field is not repeating, cannot start list" or "Extra inputs are
// not permitted", so every tool call in the run fails.

import "encoding/json"

// UNSUPPORTED_SCHEMA_KEYS are the JSON Schema keywords Google's Schema
// message has no field for. They are dropped rather than translated: a
// dropped constraint loosens validation, whereas a rejected request
// removes the tool entirely.
var UNSUPPORTED_SCHEMA_KEYS = map[string]bool{
	"additionalProperties":  true,
	"$schema":               true,
	"$id":                   true,
	"$comment":              true,
	"$ref":                  true,
	"$defs":                 true,
	"definitions":           true,
	"const":                 true,
	"contentMediaType":      true,
	"contentEncoding":       true,
	"if":                    true,
	"then":                  true,
	"else":                  true,
	"not":                   true,
	"patternProperties":     true,
	"unevaluatedProperties": true,
	"unevaluatedItems":      true,
	"dependentRequired":     true,
	"dependentSchemas":      true,
	"propertyNames":         true,
	"minContains":           true,
	"maxContains":           true,
}

// KEPT_SCHEMA_KEYS are the leaf keywords carried through untouched.
var KEPT_SCHEMA_KEYS = map[string]bool{
	"description": true,
	"enum":        true,
	"format":      true,
	"default":     true,
	"examples":    true,
}

// CleanSchema converts one JSON Schema value into Google's dialect.
//
// The transformations, in the order they matter:
//   - type strings are uppercased ("object" → "OBJECT")
//   - anyOf/oneOf collapse to the first object branch, else the first
//     branch — Google has no union type, and dropping the field entirely
//     would lose the argument
//   - unsupported keywords are removed recursively
//   - `required` is filtered to names that actually appear in properties,
//     since Google rejects a required name with no declaration
//   - an ARRAY without `items` gets a STRING item, and an object with no
//     properties gets a placeholder, because Google rejects both empties
func CleanSchema(value any) any {
	switch v := value.(type) {
	case bool:
		// JSON Schema's `true` means "anything"; `false` means "nothing".
		if v {
			return map[string]any{"type": "STRING"}
		}
		return map[string]any{"type": "NULL"}
	case map[string]any:
		return cleanObject(v)
	default:
		return value
	}
}

func cleanObject(schema map[string]any) any {
	if branch, ok := pickUnionBranch(schema); ok {
		return CleanSchema(branch)
	}

	declared := map[string]bool{}
	if props, ok := schema["properties"].(map[string]any); ok {
		for name := range props {
			declared[name] = true
		}
	}

	out := map[string]any{}
	for key, value := range schema {
		if UNSUPPORTED_SCHEMA_KEYS[key] {
			continue
		}
		switch key {
		case "type":
			if s, ok := value.(string); ok {
				out[key] = upper(s)
			}
		case "properties":
			props, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if len(props) == 0 {
				out[key] = placeholderProperties()
				continue
			}
			cleaned := make(map[string]any, len(props))
			for name, sub := range props {
				cleaned[name] = CleanSchema(sub)
			}
			out[key] = cleaned
		case "items":
			out[key] = CleanSchema(value)
		case "required":
			list, ok := value.([]any)
			if !ok {
				continue
			}
			kept := make([]any, 0, len(list))
			for _, name := range list {
				if s, ok := name.(string); ok && declared[s] {
					kept = append(kept, s)
				}
			}
			if len(kept) > 0 {
				out[key] = kept
			}
		default:
			if KEPT_SCHEMA_KEYS[key] {
				out[key] = value
			}
		}
	}

	if out["type"] == "ARRAY" {
		if _, ok := out["items"]; !ok {
			out["items"] = map[string]any{"type": "STRING"}
		}
	}
	if _, ok := out["type"]; !ok {
		if _, hasProps := out["properties"]; hasProps {
			out["type"] = "OBJECT"
		}
	}
	if props, ok := out["properties"].(map[string]any); ok {
		if _, placeheld := props["_placeholder"]; placeheld {
			out["required"] = []any{"_placeholder"}
		}
	}
	return out
}

// pickUnionBranch resolves anyOf/oneOf to a single branch, preferring an
// object branch since that is what a tool argument almost always is.
func pickUnionBranch(schema map[string]any) (any, bool) {
	for _, key := range []string{"anyOf", "oneOf"} {
		options, ok := schema[key].([]any)
		if !ok || len(options) == 0 {
			continue
		}
		for _, opt := range options {
			if m, ok := opt.(map[string]any); ok {
				if t, _ := m["type"].(string); upper(t) == "OBJECT" {
					return opt, true
				}
			}
		}
		return options[0], true
	}
	return nil, false
}

// placeholderProperties satisfies Google's "properties must be non-empty"
// rule for tools that genuinely take no arguments.
func placeholderProperties() map[string]any {
	return map[string]any{
		"_placeholder": map[string]any{
			"type":        "BOOLEAN",
			"description": "Technical placeholder to ensure a non-empty schema",
		},
	}
}

// encodeSchema turns a provider-neutral core.ToolSpec.Parameters value
// into the cleaned Google-dialect JSON the declaration carries. A value
// that cannot round-trip through JSON yields no schema at all, which the
// gateway reads as "no arguments" — better than a body it rejects.
func encodeSchema(parameters any) json.RawMessage {
	if parameters == nil {
		return nil
	}
	var raw []byte
	switch v := parameters.(type) {
	case json.RawMessage:
		raw = v
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		raw = encoded
	}
	if len(raw) == 0 {
		return nil
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil
	}
	cleaned, err := json.Marshal(CleanSchema(generic))
	if err != nil {
		return nil
	}
	return cleaned
}

func upper(s string) string {
	out := []byte(s)
	for i := range out {
		if out[i] >= 'a' && out[i] <= 'z' {
			out[i] -= 'a' - 'A'
		}
	}
	return string(out)
}
