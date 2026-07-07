package core

// RiskLevel is set at tool definition time and consulted by ApprovalPolicy.
type RiskLevel string

const (
	RISK_LEVEL_LOW  RiskLevel = "low"
	RISK_LEVEL_HIGH RiskLevel = "high"
)

// ToolSpec is what the LLM sees — the JSON shape of one tool's arguments.
// Parameters is a JSON Schema string or object depending on the provider; we
// keep it as RawMessage so any provider adapter can re-encode to its own dialect.
type ToolSpec struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Parameters  any       `json:"parameters"` // JSON Schema object
	Risk        RiskLevel `json:"risk"`
}

// JSONSchema describes what a tool's Args struct accepts. It is generated
// by action/schema.go from struct tags (jsonschema tag + json tag).
// This is a go-side annotation — the runtime-facing type is ToolSpec.
type JSONSchema struct {
	Type                 string             `json:"type"`
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties any                `json:"additionalProperties,omitempty"`
}
