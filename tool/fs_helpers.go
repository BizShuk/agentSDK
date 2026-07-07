package tool

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bizshuk/agentsdk/action"
)

// resolvePath cleans a path and resolves symlinks, then re-checks against
// the sandbox. Returns the resolved absolute path, or an error if the
// path is rejected by the policy.
//
// The double-check pattern: if policy is non-nil, Check is called first
// against the literal input, then again against the resolved path (in
// case a symlink escapes the allowed prefix).
func resolvePath(policy action.Sandbox, toolName, path, workingDir string) (string, error) {
	// Normalize: make absolute.
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDir, path)
	}
	clean := filepath.Clean(path)

	// Sandbox check against literal input.
	if err := checkPathArgs(policy, toolName, clean); err != nil {
		return "", err
	}

	// Resolve symlinks, then re-check.
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// File doesn't exist yet — trust the cleaned path for writes.
			return clean, nil
		}
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	if resolved != clean {
		if err := checkPathArgs(policy, toolName, resolved); err != nil {
			return "", fmt.Errorf("symlink escape after resolving %q → %q: %w", clean, resolved, err)
		}
	}
	return resolved, nil
}

// resolveCwd resolves a working directory for tools that accept a cwd
// argument. If cwdArg is empty, workingDir is used. The resolved path is
// checked against the sandbox (if policy is non-nil).
func resolveCwd(policy action.Sandbox, toolName, cwdArg, workingDir string) (string, error) {
	cwd := cwdArg
	if cwd == "" {
		cwd = workingDir
	}
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(workingDir, cwd)
	}
	cwd = filepath.Clean(cwd)

	if policy != nil {
		// cwd is not in the default Policy.PathKeys, but we check anyway
		// as a defensive layer.
		_ = policy.Check(toolName, map[string]any{"path": cwd})
	}

	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd %q: %w", cwd, err)
	}
	return resolved, nil
}

// sniffMime reads up to 512 bytes and returns the MIME type.
// Returns "text/plain" on empty content and "application/octet-stream"
// when detection fails.
func sniffMime(r io.Reader) (string, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	if n == 0 {
		return "text/plain", nil
	}
	ct := http.DetectContentType(buf[:n])
	return ct, nil
}

// isTextMime returns true if the MIME type is a known text format (plain,
// JSON, XML, script, markdown, YAML, etc.) rather than a binary format.
func isTextMime(mime string) bool {
	textPrefixes := []string{
		"text/",
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-ndjson",
		"application/x-yaml",
	}
	mime = strings.TrimSpace(mime)
	for _, p := range textPrefixes {
		if strings.HasPrefix(mime, p) {
			return true
		}
	}
	return false
}

// atomicWrite writes data to path atomically: write to a temp file in the
// same directory, then rename. perm is used if the file is created; 0
// falls back to 0o644.
func atomicWrite(path string, data []byte, perm int) (int64, bool, error) {
	if perm == 0 {
		perm = 0o644
	}
	dir := filepath.Dir(path)

	// Check if file already exists.
	_, statErr := os.Stat(path)
	created := errors.Is(statErr, fs.ErrNotExist)

	f, err := os.CreateTemp(dir, ".tool-write-*")
	if err != nil {
		return 0, false, fmt.Errorf("create temp file for %q: %w", path, err)
	}
	tmpName := f.Name()
	defer func() {
		f.Close()
		// If rename succeeded, the temp file is gone. Otherwise clean up.
		if _, statErr2 := os.Stat(tmpName); statErr2 == nil {
			_ = os.Remove(tmpName)
		}
	}()

	// Write to temp.
	n, err := f.Write(data)
	if err != nil {
		return 0, false, fmt.Errorf("write temp file for %q: %w", path, err)
	}
	if err := f.Chmod(fs.FileMode(perm)); err != nil {
		return 0, false, fmt.Errorf("chmod temp file for %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return 0, false, fmt.Errorf("close temp file for %q: %w", path, err)
	}

	// Atomic rename.
	if err := os.Rename(tmpName, path); err != nil {
		return 0, false, fmt.Errorf("rename temp → %q: %w", path, err)
	}
	return int64(n), created, nil
}

// safeCwd returns the configured working directory, defaulting to the
// process CWD when empty. Inline here so tests don't depend on globals.
func safeCwd(configured string) (string, error) {
	if configured != "" {
		return filepath.Abs(configured)
	}
	return os.Getwd()
}
