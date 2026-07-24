package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Context-file loading. The behaviour is fixed: a single function with
// only cwd and userDir as inputs, and a hardcoded file-name priority,
// byte budget, and @import depth. No Loader struct, no knobs — apps
// vary only by what files exist on disk, not by how the loader walks.

const (
	contextFilesMaxBytes    = 256 * 1024
	contextFilesImportDepth = 5
)

// contextFileNames is the per-directory filename priority: the first
// existing name wins for that directory.
var contextFileNames = []string{"AGENTS.md", "CLAUDE.md"}

// ContextFile is one loaded instruction file with imports already
// expanded. Returned alongside the merged text so callers can inspect
// provenance (e.g. for diagnostics) without re-walking the hierarchy.
type ContextFile struct {
	Path    string
	Content string
}

// LoadContextFiles walks userDir → repo root → … → cwd, loads the first
// matching AGENTS.md / CLAUDE.md from each directory, expands "@relative/
// path" imports, and returns the merged instruction text plus the file
// list. Missing files are not errors.
//
// The loader performs read-only filesystem access. The result is meant
// to feed the system prompt's "context files" section (ORDER_FILES).
func LoadContextFiles(cwd, userDir string) (string, []ContextFile, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", nil, fmt.Errorf("context files: abs %q: %w", cwd, err)
	}

	var dirs []string
	if userDir != "" {
		dirs = append(dirs, userDir)
	}
	dirs = append(dirs, chainFromRoot(abs)...)

	var files []ContextFile
	var sb strings.Builder
	seen := map[string]bool{}
	for _, dir := range dirs {
		path, ok := firstExisting(dir)
		if !ok || seen[path] {
			continue
		}
		seen[path] = true
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := expandImports(path, string(raw), map[string]bool{path: true}, contextFilesImportDepth)
		files = append(files, ContextFile{Path: path, Content: content})
		entry := fmt.Sprintf("## Context: %s\n\n%s", path, strings.TrimSpace(content))
		if sb.Len()+len(entry) > contextFilesMaxBytes {
			fmt.Fprintf(&sb, "## Context: %s\n\n(omitted: context budget %d bytes exceeded)\n\n", path, contextFilesMaxBytes)
			continue
		}
		sb.WriteString(entry)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), files, nil
}

// chainFromRoot returns repoRoot..cwd top-down. Without a .git marker
// the chain is just cwd.
func chainFromRoot(cwd string) []string {
	root := repoRoot(cwd)
	if root == "" {
		return []string{cwd}
	}
	var chain []string
	dir := cwd
	for {
		chain = append([]string{dir}, chain...)
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return chain
}

// repoRoot walks up from cwd looking for a .git entry.
func repoRoot(cwd string) string {
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func firstExisting(dir string) (string, bool) {
	for _, name := range contextFileNames {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

// expandImports replaces lines of the form "@relative/path" with the
// file's content (recursively, cycle-safe). Anything that does not
// resolve stays verbatim. depth decreases by one per level and stops
// the recursion when it hits zero.
func expandImports(path, content string, visited map[string]bool, depth int) string {
	if depth <= 0 {
		return content
	}
	dir := filepath.Dir(path)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "@") || strings.ContainsAny(trimmed, " \t") {
			continue
		}
		target := filepath.Join(dir, strings.TrimPrefix(trimmed, "@"))
		if visited[target] {
			lines[i] = fmt.Sprintf("<!-- import cycle skipped: %s -->", trimmed)
			continue
		}
		raw, err := os.ReadFile(target)
		if err != nil {
			continue // unresolvable import stays verbatim
		}
		visited[target] = true
		lines[i] = expandImports(target, string(raw), visited, depth-1)
	}
	return strings.Join(lines, "\n")
}
