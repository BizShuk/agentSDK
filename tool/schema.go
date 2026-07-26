package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/invopop/jsonschema"
)

// SchemaFor returns a *jsonschema.Schema reflecting the type T.
//
// Required-field inference follows json tags: a field without
// `omitempty` is required; with `omitempty` it becomes optional.
// String fields can opt out via `,omitempty` (or any other tag with
// the "omitempty" token).
func SchemaFor[T any]() *jsonschema.Schema {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		// T is interface{}; return the loosest schema.
		return &jsonschema.Schema{}
	}
	return jsonschema.ReflectFromType(t)
}

// SchemaJSON returns the JSON-encoded representation of SchemaFor[T]().
// The output is suitable for inclusion in core.ToolSchema.Parameters.
func SchemaJSON[T any]() (json.RawMessage, error) {
	s := SchemaFor[T]()
	return json.Marshal(s)
}

// SchemaForTool composes a full core.ToolSpec for a tool with the
// given Args type. The Name / Description / Risk fields come from the
// caller; Parameters is the reflected JSON schema for T.
func SchemaForTool[T any](name, desc string, risk core.RiskLevel) (core.ToolSpec, error) {
	raw, err := SchemaJSON[T]()
	if err != nil {
		return core.ToolSpec{}, fmt.Errorf("schema reflect %q: %w", name, err)
	}
	return core.ToolSpec{
		Name:        name,
		Description: desc,
		Risk:        risk,
		Parameters:  json.RawMessage(raw),
	}, nil
}

// SchemaError is returned by ValidateArgs when raw JSON does not match
// the reflected schema for T. Carries a list of field-level errors so
// callers can surface structured feedback.
type SchemaError struct {
	Tool   string
	Errors []string
}

// Error implements error.
func (e *SchemaError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Errors) == 0 {
		return e.Tool + ": schema validation failed"
	}
	return fmt.Sprintf("%s: schema validation failed: %v", e.Tool, e.Errors)
}

// ValidateArgs checks that raw conforms to SchemaFor[T](). The check
// is intentionally lightweight — required-field presence and basic
// type compatibility. It is NOT a full JSON Schema validator; for
// production-grade validation, plug in a library like santhosh-tekuri/jsonschema.
//
// The bool return is `valid`. When false, err is non-nil.
func ValidateArgs[T any](toolName string, raw json.RawMessage) (valid bool, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		// Allow zero-value args if T is the zero value type.
		var zero T
		if reflect.TypeOf(zero).NumField() == 0 {
			return true, nil
		}
		return false, &SchemaError{Tool: toolName, Errors: []string{"missing args object"}}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, &SchemaError{Tool: toolName, Errors: []string{"invalid JSON: " + err.Error()}}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return false, &SchemaError{Tool: toolName, Errors: []string{"args must be a JSON object"}}
	}
	s := SchemaFor[T]()
	missing := requiredFieldsMissing(s, m)
	if len(missing) > 0 {
		return false, &SchemaError{Tool: toolName, Errors: missing}
	}
	return true, nil
}

// requiredFieldsMissing compares the JSON Schema's required list
// against the keys present in m. jsonschema's reflector inlines simple
// types but emits `$ref` + `$defs` for named structs — we resolve the
// ref first so callers see the actual required list.
func requiredFieldsMissing(s *jsonschema.Schema, m map[string]any) []string {
	target := resolveRef(s)
	var missing []string
	for _, name := range target.Required {
		if _, ok := m[name]; !ok {
			missing = append(missing, "missing required field: "+name)
		}
	}
	return missing
}

// resolveRef walks `$ref` + `$defs` to land on the concrete schema.
// Returns the original schema if no ref is set.
func resolveRef(s *jsonschema.Schema) *jsonschema.Schema {
	if s == nil || s.Ref == "" {
		return s
	}
	ref := s.Ref
	// Strip leading "#/" then split into path segments.
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return s
	}
	name := strings.TrimPrefix(ref, prefix)
	if def, ok := s.Definitions[name]; ok && def != nil {
		return def
	}
	return s
}
