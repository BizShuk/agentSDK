package tool

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
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
	Matches []string `json:"matches"`
	Count   int      `json:"count"`
}

// GlobDesc is the tool description sent to the LLM.
const GlobDesc = "Find files matching a glob pattern. Supports ** for recursive directory matching (e.g. '**/*.go', 'src/**/*_test.go'). Returns sorted, relative paths. Capped at 100 results."

// Glob finds files matching a pattern. Supports ** for recursive matching.
// Results are sorted and capped at MaxMatches.
type Glob struct {
	policy     Sandbox
	rootDir    string
	maxMatches int
}

// NewGlob constructs a Glob tool. policy may be nil.
func NewGlob(opts GlobOptions, policy Sandbox, rootDir string) *Glob {
	if opts.MaxMatches <= 0 {
		opts.MaxMatches = defaultMaxMatches()
	}
	return &Glob{
		policy:     policy,
		rootDir:    rootDir,
		maxMatches: opts.MaxMatches,
	}
}

// ToolName returns the tool's registry name.
func (g *Glob) ToolName() string { return NAME_GLOB }

// ToolRisk returns the risk level.
func (g *Glob) ToolRisk() core.RiskLevel { return core.RISK_LEVEL_LOW }

// Handle is the pure business logic — no JSON, no schema.
func (g *Glob) Handle(_ context.Context, a GlobArgs) (GlobOutput, error) {
	wd, err := safeCwd(g.rootDir)
	if err != nil {
		return GlobOutput{}, err
	}
	if a.Cwd != "" {
		cwd, cwdErr := resolveCwd(g.policy, "glob", a.Cwd, wd)
		if cwdErr != nil {
			return GlobOutput{}, cwdErr
		}
		wd = cwd
	}

	if a.Pattern == "" {
		a.Pattern = "*"
	}

	// Check if pattern contains ** for recursive matching.
	if strings.Contains(a.Pattern, "**") {
		return g.globRecursive(wd, a.Pattern)
	}

	// Use stdlib Glob for simple patterns.
	absPattern := filepath.Join(wd, a.Pattern)
	matches, err := filepath.Glob(absPattern)
	if err != nil {
		return GlobOutput{}, err
	}
	// Make paths relative to wd.
	relMatches := make([]string, 0, len(matches))
	for _, m := range matches {
		if rel, relErr := filepath.Rel(wd, m); relErr == nil {
			relMatches = append(relMatches, rel)
		} else {
			relMatches = append(relMatches, m)
		}
	}
	return g.buildOutput(wd, relMatches), nil
}

// globRecursive handles ** patterns by walking the filesystem.
func (g *Glob) globRecursive(root, pattern string) (GlobOutput, error) {
	// Normalize pattern: make it relative to root.
	// Split pattern into prefix / ** / suffix.
	parts := splitDoublestar(pattern)
	prefix := parts[0]

	var matches []string

	walkRoot := root
	if prefix != "" && prefix != "." {
		walkRoot = filepath.Join(root, prefix)
	}

	err := filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if len(matches) >= g.maxMatches {
			return fs.SkipAll
		}

		// Compute relative path from root so we can match against pattern.
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}

		// Match against our doublestar pattern.
		if matchDoublestar(pattern, rel) {
			// Check sandbox.
			if g.policy != nil {
				v := g.policy.Check("glob", map[string]any{"path": path})
				if v != action.VERDICT_ALLOW {
					return nil // skip denied paths silently
				}
			}
			matches = append(matches, rel)
		}
		return nil
	})
	if err != nil {
		return GlobOutput{}, err
	}
	return g.buildOutput(root, matches), nil
}

// splitDoublestar splits a pattern like "a/**/b" into ["a/", "b"].
func splitDoublestar(pattern string) [2]string {
	before, after, found := strings.Cut(pattern, "**")
	if !found {
		return [2]string{pattern, ""}
	}
	// Trim trailing slash from prefix.
	before = strings.TrimSuffix(before, "/")
	// Trim leading slash from suffix.
	after = strings.TrimPrefix(after, "/")
	return [2]string{before, after}
}

// matchDoublestar checks if name matches a pattern that may contain **.
// This is a simplified implementation that handles the common cases:
// "**", "**/*.go", "prefix/**", "prefix/**/suffix".
func matchDoublestar(pattern, name string) bool {
	// Exact match on **.
	if pattern == "**" {
		return true
	}

	parts := splitDoublestar(pattern)
	prefix := parts[0]
	suffix := parts[1]

	// If prefix is non-empty and name doesn't start with prefix, no match.
	if prefix != "" {
		if !strings.HasPrefix(name, prefix) {
			return false
		}
		// Strip the prefix for suffix matching on the remainder.
		name = strings.TrimPrefix(name, prefix)
		if name != "" && name[0] == '/' {
			name = name[1:]
		}
	}

	// If suffix is empty, everything after ** matches.
	if suffix == "" {
		return true
	}

	// **/suffix → match: suffix appears after any number of path components.
	// We check if any suffix of name matches the suffix pattern.
	return matchSuffix(name, suffix)
}

// matchSuffix checks if path matches a pattern containing no **.
// Supports simple globs like "*.go" and "foo/*.go".
func matchSuffix(path, pattern string) bool {
	// If pattern contains /, we need to match full relative path.
	if strings.Contains(pattern, "/") {
		return matchPathComponents(path, pattern)
	}
	// Simple filename-only pattern (e.g., "*.go").
	// Check if the last component of path matches.
	_, base := filepath.Split(path)
	ok, _ := filepath.Match(pattern, base)
	return ok
}

// matchPathComponents matches a path against a pattern with /
// by aligning components from the end.
func matchPathComponents(path, pattern string) bool {
	pathParts := strings.Split(path, "/")
	patParts := strings.Split(pattern, "/")

	if len(patParts) > len(pathParts) {
		return false
	}

	// Align from the end.
	offset := len(pathParts) - len(patParts)
	for i, pp := range patParts {
		ok, _ := filepath.Match(pp, pathParts[offset+i])
		if !ok {
			return false
		}
	}
	return true
}

func (g *Glob) buildOutput(_ string, matches []string) GlobOutput {
	sort.Strings(matches)
	if len(matches) > g.maxMatches {
		matches = matches[:g.maxMatches]
	}
	return GlobOutput{Matches: matches, Count: len(matches)}
}
