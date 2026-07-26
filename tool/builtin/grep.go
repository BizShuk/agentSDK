package builtin

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
)

// NAME_GREP is the registry name of the grep tool.
const NAME_GREP = "grep"

// GrepArgs is the input shape the LLM sees for the grep tool.
type GrepArgs struct {
	Query string `json:"query"`
	Cwd   string `json:"cwd,omitempty"`
}

// GrepMatch represents a single matching line in a file.
type GrepMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// GrepOutput is the output shape returned to the LLM.
type GrepOutput struct {
	Matches   []GrepMatch `json:"matches"`
	Count     int         `json:"count"`
	Truncated bool        `json:"truncated"`
}

// GrepDesc is the tool description sent to the LLM.
const GrepDesc = "Fast content search across files in a directory using regex or substring matching. Returns matching lines with file paths and line numbers. Max 100 results."

// Grep performs content search across files.
type Grep struct {
	policy     tool.Sandbox
	rootDir    string
	maxResults int
}

// NewGrep constructs a Grep tool. policy may be nil.
func NewGrep(policy tool.Sandbox, rootDir string, opts ...GrepOption) *Grep {
	gr := &Grep{
		policy:     policy,
		rootDir:    rootDir,
		maxResults: defaultMaxMatches(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(gr)
		}
	}
	return gr
}

var _ tool.Tool[GrepArgs, GrepOutput] = (*Grep)(nil)

// Name returns the tool's registry name.
func (g *Grep) Name() string { return NAME_GREP }

// Risk returns the risk level.
func (g *Grep) Risk() core.RiskLevel { return core.RISK_LEVEL_LOW }

// Desc returns the tool description.
func (g *Grep) Desc() string { return GrepDesc }

// Handle is the pure business logic — no JSON, no schema.
func (g *Grep) Handle(_ context.Context, a GrepArgs) (GrepOutput, error) {
	if a.Query == "" {
		return GrepOutput{}, fmt.Errorf("grep: query must not be empty")
	}

	wd, err := safeCwd(g.rootDir)
	if err != nil {
		return GrepOutput{}, fmt.Errorf("grep: working dir: %w", err)
	}

	searchDir := wd
	if a.Cwd != "" {
		resolvedCwd, cwdErr := resolveCwd(g.policy, "grep", a.Cwd, wd)
		if cwdErr != nil {
			return GrepOutput{}, fmt.Errorf("grep: invalid cwd: %w", cwdErr)
		}
		searchDir = resolvedCwd
	}

	// Try regex compile; fallback to literal substring search.
	re, reErr := regexp.Compile(a.Query)

	var matches []GrepMatch
	truncated := false

	err = filepath.WalkDir(searchDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}

		if g.policy != nil {
			if v := g.policy.Check("grep", map[string]any{"path": path}); v != tool.VERDICT_ALLOW {
				return nil // Skip files denied by sandbox
			}
		}

		f, err := os.Open(path)
		if err != nil {
			return nil // Skip unreadable files
		}
		defer f.Close()

		// Sniff MIME: skip binary files.
		mime, err := sniffMime(f)
		if err != nil || !isTextMime(mime) {
			return nil
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil
		}

		rel, relErr := filepath.Rel(searchDir, path)
		if relErr != nil {
			rel = path
		}

		scanner := bufio.NewScanner(f)
		lineNo := 0

		for scanner.Scan() {
			lineNo++
			lineText := scanner.Text()

			matched := false
			if reErr == nil {
				matched = re.MatchString(lineText)
			} else {
				matched = strings.Contains(lineText, a.Query)
			}

			if matched {
				matches = append(matches, GrepMatch{
					Path:    rel,
					Line:    lineNo,
					Content: lineText,
				})

				if len(matches) >= g.maxResults {
					truncated = true
					return filepath.SkipAll
				}
			}
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return GrepOutput{}, fmt.Errorf("grep walk: %w", err)
	}

	if matches == nil {
		matches = []GrepMatch{}
	}

	return GrepOutput{
		Matches:   matches,
		Count:     len(matches),
		Truncated: truncated,
	}, nil
}
