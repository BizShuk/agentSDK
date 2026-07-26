package action_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type schemaArgs struct {
	Path string `json:"path"`           // required (no omitempty)
	Mode string `json:"mode,omitempty"` // optional
	N    int    `json:"n,omitempty"`    // optional
}

type onlyRequired struct {
	Name string `json:"name"`
}

type onlyOptional struct {
	Label string `json:"label,omitempty"`
}

func TestSchemaForContainsRequiredFields(t *testing.T) {
	s := action.SchemaFor[schemaArgs]()
	require.NotNil(t, s)
	// The reflector emits `$ref` + `$defs`; resolve to find the
	// concrete schema before asserting on Required.
	raw, err := json.Marshal(s)
	require.NoError(t, err)
	var asMap any
	require.NoError(t, json.Unmarshal(raw, &asMap))
	required := findRequired(asMap)
	require.NotEmpty(t, required, "expected required list in reflected schema")
	assert.Contains(t, required, "path", "non-omitempty field must be required")
	assert.NotContains(t, required, "mode")
	assert.NotContains(t, required, "n")
}

func TestSchemaJSONRoundTrip(t *testing.T) {
	raw, err := action.SchemaJSON[schemaArgs]()
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	// Top-level schema may be `$ref` + `$defs/schemaArgs`; resolve it.
	defs, _ := m["$defs"].(map[string]any)
	require.NotNil(t, defs, "expected $defs in schema output")
	inner, _ := defs["schemaArgs"].(map[string]any)
	require.NotNil(t, inner)
	assert.Equal(t, "object", inner["type"])
	props, _ := inner["properties"].(map[string]any)
	require.NotNil(t, props)
	assert.Contains(t, props, "path")
	assert.Contains(t, props, "mode")
}

func TestSchemaForToolProducesCompleteToolSchema(t *testing.T) {
	ts, err := action.SchemaForTool[schemaArgs]("read_file", "read a file", core.RISK_LEVEL_HIGH)
	require.NoError(t, err)
	assert.Equal(t, "read_file", ts.Name)
	assert.Equal(t, core.RISK_LEVEL_HIGH, ts.Risk)
	params, ok := ts.Parameters.(json.RawMessage)
	require.True(t, ok)
	var m map[string]any
	require.NoError(t, json.Unmarshal(params, &m))
	// Walk the schema to find the required list — the reflector may
	// place it under $defs/<TypeName>.required.
	defs, _ := m["$defs"].(map[string]any)
	required := findRequired(m)
	if defs != nil {
		required = append(required, findRequired(defs)...)
	}
	require.NotEmpty(t, required, "schema must declare required fields")
	hasPath := false
	for _, r := range required {
		if r == "path" {
			hasPath = true
		}
	}
	assert.True(t, hasPath)
}

func findRequired(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				out = append(out, s)
			}
		}
	}
	// Recurse into $defs (jsonschema reflector emits definitions).
	if defs, ok := m["$defs"].(map[string]any); ok {
		for _, child := range defs {
			out = append(out, findRequired(child)...)
		}
	}
	return out
}

func TestValidateArgsAcceptsCompletePayload(t *testing.T) {
	ok, err := action.ValidateArgs[schemaArgs]("read_file", json.RawMessage(`{"path":"/tmp/x"}`))
	assert.True(t, ok)
	assert.Nil(t, err)
}

func TestValidateArgsRejectsMissingRequired(t *testing.T) {
	ok, err := action.ValidateArgs[schemaArgs]("read_file", json.RawMessage(`{"mode":"fast"}`))
	assert.False(t, ok)
	require.NotNil(t, err)
	var se *action.SchemaError
	require.ErrorAs(t, err, &se)
	assert.Contains(t, err.Error(), "missing required field: path")
}

func TestValidateArgsRejectsInvalidJSON(t *testing.T) {
	ok, err := action.ValidateArgs[schemaArgs]("read_file", json.RawMessage(`not json`))
	assert.False(t, ok)
	require.NotNil(t, err)
}

func TestValidateArgsRejectsEmptyObjectForRequiredOnly(t *testing.T) {
	ok, err := action.ValidateArgs[onlyRequired]("rename", json.RawMessage(`{}`))
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "missing required field: name")
}

func TestValidateArgsAcceptsEmptyObjectForAllOptional(t *testing.T) {
	ok, err := action.ValidateArgs[onlyOptional]("labeler", json.RawMessage(`{}`))
	assert.True(t, ok)
	assert.Nil(t, err)
}

func TestRegisterFuncSchemaAutoReflected(t *testing.T) {
	r := action.NewRegistry()
	action.RegisterFunc(r, "read_file", "read a file", core.RISK_LEVEL_LOW,
		func(ctx context.Context, a schemaArgs) (struct{}, error) {
			return struct{}{}, nil
		})
	schemas := r.List()
	require.Len(t, schemas, 1)
	ts := schemas[0]
	assert.Equal(t, "read_file", ts.Name)
	assert.Equal(t, core.RISK_LEVEL_LOW, ts.Risk)
	params, ok := ts.Parameters.(json.RawMessage)
	require.True(t, ok)
	var asMap any
	require.NoError(t, json.Unmarshal(params, &asMap))
	required := findRequired(asMap)
	require.NotEmpty(t, required, "reflected schema must list required fields")
	hasPath := false
	for _, req := range required {
		if req == "path" {
			hasPath = true
		}
	}
	assert.True(t, hasPath, "reflected schema must list path as required")
}

// TestRegisterFuncCallValidatesBeforeFn verifies that an invalid payload
// is rejected before the wrapped function runs.
func TestRegisterFuncCallValidatesBeforeFn(t *testing.T) {
	called := false
	r := action.NewRegistry()
	action.RegisterFunc(r, "read_file", "read a file", core.RISK_LEVEL_LOW,
		func(ctx context.Context, a schemaArgs) (struct{}, error) {
			called = true
			return struct{}{}, nil
		})
	res := r.Call(context.Background(), core.ToolCall{ID: "c1", Name: "read_file", Args: map[string]any{"mode": "fast"}})
	assert.False(t, res.OK)
	assert.False(t, called, "fn must not be invoked when validation fails")
	assert.True(t, strings.Contains(res.Error, "missing required field"))
}