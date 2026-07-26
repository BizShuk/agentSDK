package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/utils/frontmatter"
)

const (
	// TOOL_NAME is the registry name of the delegation tool.
	TOOL_NAME = "task"

	// DEFAULT_MAX_DEPTH allows one level of delegation and refuses
	// recursive spawn — the safe default every surveyed client uses.
	DEFAULT_MAX_DEPTH = 1
)

// SubAgent is one subagent definition (markdown frontmatter + body prompt).
type SubAgent struct {
	Name        string
	Description string
	Provider    string
	Model       string
	Tools       []string
	Prompt      string // markdown body = system prompt of the subagent
}

// ParseDef parses one definition; fallbackName is used when frontmatter
// has no name (typically the file base name).
func ParseDef(fallbackName, content string) SubAgent {
	fields, body, _ := frontmatter.Parse(content)
	name := fields["name"]
	if name == "" {
		name = fallbackName
	}
	return SubAgent{
		Name:        name,
		Description: fields["description"],
		Provider:    fields["provider"],
		Model:       fields["model"],
		Tools:       frontmatter.List(fields["tools"]),
		Prompt:      strings.TrimSpace(body),
	}
}

// DiscoverDefs loads every <dir>/*.md definition, sorted by name.
//
// Deprecated: use Registry.DiscoverSubagents and Registry.Subagents so
// multiple discovery roots share the same override semantics as skills and
// commands.
func DiscoverDefs(dir string) ([]SubAgent, error) {
	r := NewRegistry()
	if err := r.DiscoverSubagents(dir); err != nil {
		return nil, err
	}
	defs := r.Subagents()
	if len(defs) == 0 {
		return nil, nil
	}
	return defs, nil
}

// RunFunc executes one delegated task and returns the subagent's final
// text. Injected from the composition root.
type RunFunc func(ctx context.Context, def SubAgent, prompt string) (string, error)

// Spawner implements core.Tool ("task").
type Spawner struct {
	RunFn    RunFunc
	MaxDepth int // <= 0 → DEFAULT_MAX_DEPTH

	defs map[string]SubAgent
}

// NewSpawner builds a Spawner over the given definitions.
func NewSpawner(run RunFunc, defs ...SubAgent) *Spawner {
	m := make(map[string]SubAgent, len(defs))
	for _, d := range defs {
		m[d.Name] = d
	}
	return &Spawner{RunFn: run, defs: m}
}

// Defs lists definitions sorted by name.
func (s *Spawner) Defs() []SubAgent {
	out := make([]SubAgent, 0, len(s.defs))
	for _, d := range s.defs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Name implements core.Tool.
func (s *Spawner) Name() string { return TOOL_NAME }

// Spec implements core.Tool — returns the full ToolSpec including Parameters.
func (s *Spawner) Spec() core.ToolSpec {
	var sb strings.Builder
	sb.WriteString("Delegate a self-contained task to a subagent. Available agents:\n")
	for _, d := range s.Defs() {
		fmt.Fprintf(&sb, "- %s: %s\n", d.Name, d.Description)
	}
	return core.ToolSpec{
		Name:        TOOL_NAME,
		Description: strings.TrimSpace(sb.String()),
		Parameters: &core.JSONSchema{
			Type: "object",
			Properties: map[string]*core.JSONSchema{
				"agent":  {Type: "string"},
				"prompt": {Type: "string"},
			},
			Required: []string{"agent", "prompt"},
		},
		Risk: core.RISK_LEVEL_LOW,
	}
}

// taskArgs is the wire shape of one delegation call.
type taskArgs struct {
	Agent  string `json:"agent"`
	Prompt string `json:"prompt"`
}

// Call implements core.Tool. Expected failures are encoded into ToolResult so
// the model always sees why delegation was rejected.
func (s *Spawner) Call(ctx context.Context, raw json.RawMessage) (core.ToolResult, error) {
	fail := func(msg string) (core.ToolResult, error) {
		return core.ToolResult{Name: TOOL_NAME, OK: false, Error: msg}, nil
	}
	var args taskArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return fail(fmt.Sprintf("bad task args: %v", err))
	}
	if args.Agent == "" || args.Prompt == "" {
		return fail("task requires both agent and prompt")
	}
	def, ok := s.defs[args.Agent]
	if !ok {
		return fail(fmt.Sprintf("unknown agent: %s", args.Agent))
	}
	maxDepth := s.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DEFAULT_MAX_DEPTH
	}
	depth := Depth(ctx)
	if depth >= maxDepth {
		return fail(fmt.Sprintf("subagent depth limit reached (%d)", maxDepth))
	}
	if s.RunFn == nil {
		return fail("no subagent runner wired")
	}
	out, err := s.RunFn(WithDepth(ctx, depth+1), def, args.Prompt)
	if err != nil {
		return fail(fmt.Sprintf("subagent %s: %v", def.Name, err))
	}
	return core.ToolResult{Name: TOOL_NAME, OK: true, Output: out}, nil
}

type depthKey struct{}

// Depth reports the current delegation depth (0 = top-level agent).
func Depth(ctx context.Context) int {
	if v, ok := ctx.Value(depthKey{}).(int); ok {
		return v
	}
	return 0
}

// WithDepth returns a context carrying the given delegation depth.
func WithDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, depthKey{}, depth)
}
