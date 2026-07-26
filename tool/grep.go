package tool

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
)

// NAME_GREP is the registry name of the grep tool.
const NAME_GREP = "grep"

// GrepArgs is the input shape the LLM sees for the grep tool.
type GrepArgs struct {
	Pattern         string `json:"pattern"`
	Path            string `json:"path,omitempty"`
	Glob            string `json:"glob,omitempty"`
	CaseInsensitive bool   `json:"case_insensitive,omitempty"`
	MaxResults      int    `json:"max_results,omitempty"`
	LineNumbers     bool   `json:"line_numbers,omitempty"`
}

// GrepOutput is the output shape returned to the LLM.
type GrepOutput struct {
	Matches   []GrepMatch `json:"matches"`
	Count     int         `json:"count"`
	Truncated bool        `json:"truncated,omitempty"`
}

// GrepMatch is a single matched line from grep.
type GrepMatch struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// GrepDesc is the tool description sent to the LLM.
const GrepDesc = "Search files for lines matching a regex pattern (RE2 syntax). Use 'path' to scope to a directory or file, and 'glob' to filter filenames (e.g. '*.go'). Skips binary files. Results are capped at 100 by default. Case-insensitive mode available."

// Grep searches files for a regex pattern. Uses Go's regexp (RE2) for
// linear-time matching. Skips binary files. Results capped at MaxResults.
type Grep struct {
	policy     Sandbox
	rootDir    string
	maxResults int
}

// NewGrep constructs a Grep tool. policy may be nil.
func NewGrep(opts GrepOptions, policy Sandbox, rootDir string) *Grep {
	if opts.MaxResults <= 0 {
		opts.MaxResults = defaultMaxMatches()
	}
	return &Grep{
		policy:     policy,
		rootDir:    rootDir,
		maxResults: opts.MaxResults,
	}
}

// ToolName returns the tool's registry name.
func (gr *Grep) ToolName() string { return NAME_GREP }

// ToolRisk returns the risk level.
func (gr *Grep) ToolRisk() core.RiskLevel { return core.RISK_LEVEL_LOW }

// Handle is the pure business logic — no JSON, no schema.
func (gr *Grep) Handle(_ context.Context, a GrepArgs) (GrepOutput, error) {
	wd, err := safeCwd(gr.rootDir)
	if err != nil {
		return GrepOutput{}, err
	}

	// Resolve search root.
	searchRoot := wd
	if a.Path != "" {
		if !filepath.IsAbs(a.Path) {
			a.Path = filepath.Join(wd, a.Path)
		}
		// Sandbox check on the search path.
		if gr.policy != nil {
			if v := gr.policy.Check("grep", map[string]any{"path": filepath.Clean(a.Path)}); v != action.VERDICT_ALLOW {
				return GrepOutput{}, fmt.Errorf("grep: path %q not allowed by sandbox", a.Path)
			}
		}
		searchRoot = filepath.Clean(a.Path)
	}

	// Compile regex.
	pattern := a.Pattern
	if a.CaseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return GrepOutput{}, fmt.Errorf("grep: invalid regex pattern: %w", err)
	}

	// Build file name filter from glob.
	var fileFilter func(name string) bool
	if a.Glob != "" {
		fileFilter = func(name string) bool {
			base := filepath.Base(name)
			ok, _ := filepath.Match(a.Glob, base)
			return ok
		}
	}

	max := a.MaxResults
	if max <= 0 {
		max = gr.maxResults
	}

	// Check if searchRoot is a file or directory.
	fi, statErr := os.Stat(searchRoot)
	if statErr != nil {
		return GrepOutput{}, fmt.Errorf("grep: stat %q: %w", searchRoot, statErr)
	}

	var matches []GrepMatch
	truncated := false

	if !fi.IsDir() {
		// Single file.
		matches, err = gr.grepFile(re, searchRoot, wd, fileFilter, max, a.LineNumbers)
		if err != nil {
			return GrepOutput{}, err
		}
	} else {
		// Walk directory.
		walkErr := filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				base := filepath.Base(path)
				if strings.HasPrefix(base, ".") && path != searchRoot {
					// Skip hidden directories (e.g., .git).
					return fs.SkipDir
				}
				return nil
			}
			if len(matches) >= max {
				truncated = true
				return fs.SkipAll
			}
			// Apply file name filter.
			if fileFilter != nil && !fileFilter(path) {
				return nil
			}
			// Sandbox check.
			if gr.policy != nil {
				if v := gr.policy.Check("grep", map[string]any{"path": path}); v != action.VERDICT_ALLOW {
					return nil
				}
			}
			fileMatches, grepErr := gr.grepFile(re, path, wd, nil, max-len(matches), a.LineNumbers)
			if grepErr != nil {
				return nil // skip unreadable files
			}
			matches = append(matches, fileMatches...)
			return nil
		})
		if walkErr != nil {
			return GrepOutput{}, err
		}
	}

	return GrepOutput{Matches: matches, Count: len(matches), Truncated: truncated}, nil
}

// grepFile searches a single file for regex matches.
func (gr *Grep) grepFile(re *regexp.Regexp, path, rootDir string, fileFilter func(string) bool, max int, lineNumbers bool) ([]GrepMatch, error) {
	// Apply file filter if present and not already filtered by caller.
	if fileFilter != nil {
		if !fileFilter(path) {
			return nil, nil
		}
	}

	// Skip binary files by checking MIME type.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mime, err := sniffMime(f)
	if err != nil {
		return nil, nil
	}
	if !isTextMime(mime) && !strings.HasPrefix(mime, "application/octet-stream") {
		// Known binary type — skip.
		return nil, nil
	}
	// application/octet-stream could still be text; we'll try.
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	// Compute relative path for output.
	rel := path
	if rootDir != "" {
		if r, relErr := filepath.Rel(rootDir, path); relErr == nil {
			rel = r
		}
	}

	var matches []GrepMatch
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if re.Match(scanner.Bytes()) {
			text := scanner.Text()
			if !isTextContent(text) {
				continue // binary-looking content, skip
			}
			line := 0
			if lineNumbers {
				line = lineNum
			}
			matches = append(matches, GrepMatch{File: rel, Line: line, Text: text})
			if len(matches) >= max {
				break
			}
		}
	}
	_ = scanner.Err()
	return matches, nil
}

// isTextContent is a quick check: if the line contains a null byte, treat
// as binary.
func isTextContent(s string) bool {
	return !strings.ContainsRune(s, 0)
}
