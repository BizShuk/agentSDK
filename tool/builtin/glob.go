package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
)

// NAME_GLOB is the registry name of the glob tool.
const NAME_GLOB = "glob"

// GlobArgs is the input shape the LLM sees for the glob tool.
type GlobArgs struct {
	Pattern string `json:"pattern"`
	Cwd     string `json:"cwd,omitempty"`
}

// GlobOutput is the output shape returned to the LLM.
type GlobOutput struct {
	Matches   []string `json:"matches"`
	Count     int      `json:"count"`
	Truncated bool     `json:"truncated"`
}

// GlobDesc is the tool description sent to the LLM.
const GlobDesc = "Fast file pattern matching. Returns matching file paths relative to cwd. Max 100 matches by default."

// Glob performs file pattern matching.
type Glob struct {
	policy     tool.Sandbox
	rootDir    string
	maxMatches int
}

// NewGlob constructs a Glob tool. policy may be nil.
func NewGlob(policy tool.Sandbox, rootDir string, opts ...GlobOption) *Glob {
	g := &Glob{
		policy:     policy,
		rootDir:    rootDir,
		maxMatches: defaultMaxMatches(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(g)
		}
	}
	return g
}

var _ tool.Tool = (*Glob)(nil)

// Name returns the tool's registry name.
func (g *Glob) Name() string { return NAME_GLOB }

// Spec returns the tool's metadata and reflected argument schema.
func (g *Glob) Spec() core.ToolSpec {
	return tool.MustSchemaForTool[GlobArgs](
		NAME_GLOB,
		GlobDesc,
		core.RISK_LEVEL_LOW,
	)
}

// Call converts raw JSON arguments and executes the glob operation.
func (g *Glob) Call(
	ctx context.Context,
	raw json.RawMessage,
) (core.ToolResult, error) {
	return tool.CallWithRawMessage(ctx, g.Name(), raw, g.execute)
}

func (g *Glob) execute(_ context.Context, a GlobArgs) (GlobOutput, error) {
	if a.Pattern == "" {
		return GlobOutput{}, fmt.Errorf("glob: pattern must not be empty")
	}

	wd, err := safeCwd(g.rootDir)
	if err != nil {
		return GlobOutput{}, fmt.Errorf("glob: working dir: %w", err)
	}

	searchDir := wd
	if a.Cwd != "" {
		resolvedCwd, cwdErr := resolveCwd(g.policy, "glob", a.Cwd, wd)
		if cwdErr != nil {
			return GlobOutput{}, fmt.Errorf("glob: invalid cwd: %w", cwdErr)
		}
		searchDir = resolvedCwd
	}

	var matches []string
	truncated := false

	err = filepath.WalkDir(searchDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(searchDir, path)
		if relErr != nil {
			return nil
		}

		matched, matchErr := filepath.Match(a.Pattern, d.Name())
		if matchErr != nil {
			return fmt.Errorf("invalid pattern %q: %w", a.Pattern, matchErr)
		}

		if matched {
			if g.policy != nil {
				if v := g.policy.Check("glob", map[string]any{"path": path}); v != tool.VERDICT_ALLOW {
					return nil // Skip files denied by sandbox
				}
			}

			if len(matches) >= g.maxMatches {
				truncated = true
				return filepath.SkipAll
			}
			matches = append(matches, rel)
		}
		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return GlobOutput{}, fmt.Errorf("glob walk: %w", err)
	}

	if matches == nil {
		matches = []string{}
	}

	return GlobOutput{
		Matches:   matches,
		Count:     len(matches),
		Truncated: truncated,
	}, nil
}
