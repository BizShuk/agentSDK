// Package contextfile loads hierarchical project instruction files — the
// AGENTS.md / CLAUDE.md convention every surveyed agent client shares: a
// user-level file, then the repo root, then every directory down to the
// working directory, with "@relative/path" import expansion.
//
// The loader performs read-only filesystem access and returns plain text;
// the composition root injects the result into the initial system prompt.
package contextfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DEFAULT_MAX_BYTES    = 256 * 1024
	DEFAULT_IMPORT_DEPTH = 5
)

// DefaultNames is the per-directory filename priority: the first existing
// name wins for that directory.
var DefaultNames = []string{"AGENTS.md", "CLAUDE.md"}

// File is one loaded instruction file (imports already expanded).
type File struct {
	Path    string
	Content string
}

// Loader configures discovery. The zero value works: DefaultNames, no
// user layer, DEFAULT_MAX_BYTES cap.
type Loader struct {
	Names          []string // filename priority per directory; nil → DefaultNames
	UserDir        string   // optional top layer, e.g. ~/.config/<app>
	MaxBytes       int      // total content cap; <= 0 → DEFAULT_MAX_BYTES
	MaxImportDepth int      // @import recursion cap; <= 0 → DEFAULT_IMPORT_DEPTH
}

// Load walks UserDir → repo root → … → cwd, loads the first matching name
// in each directory, expands imports, and returns the merged instruction
// text plus the file list. Missing files are not errors.
func (l Loader) Load(cwd string) (string, []File, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", nil, fmt.Errorf("contextfile: abs %q: %w", cwd, err)
	}
	names := l.Names
	if len(names) == 0 {
		names = DefaultNames
	}
	maxBytes := l.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DEFAULT_MAX_BYTES
	}
	depth := l.MaxImportDepth
	if depth <= 0 {
		depth = DEFAULT_IMPORT_DEPTH
	}

	var dirs []string
	if l.UserDir != "" {
		dirs = append(dirs, l.UserDir)
	}
	dirs = append(dirs, chainFromRoot(abs)...)

	var files []File
	var sb strings.Builder
	seen := map[string]bool{}
	for _, dir := range dirs {
		path, ok := firstExisting(dir, names)
		if !ok || seen[path] {
			continue
		}
		seen[path] = true
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := expandImports(path, string(raw), map[string]bool{path: true}, depth)
		files = append(files, File{Path: path, Content: content})
		entry := fmt.Sprintf("## Context: %s\n\n%s", path, strings.TrimSpace(content))
		if sb.Len()+len(entry) > maxBytes {
			fmt.Fprintf(&sb, "## Context: %s\n\n(omitted: context budget %d bytes exceeded)\n\n", path, maxBytes)
			continue
		}
		sb.WriteString(entry)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), files, nil
}

// chainFromRoot returns repoRoot..cwd top-down. Without a .git marker the
// chain is just cwd.
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

func firstExisting(dir string, names []string) (string, bool) {
	for _, name := range names {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

// expandImports replaces lines of the form "@relative/path" with the file's
// content (recursively, cycle-safe). Anything that does not resolve stays
// verbatim.
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
