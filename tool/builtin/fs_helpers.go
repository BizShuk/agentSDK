package builtin

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bizshuk/agentsdk/tool"
)

// resolvePath cleans a path and resolves symlinks, then re-checks against
// the sandbox. Returns the resolved absolute path, or an error if the
// path is rejected by the policy.
func resolvePath(policy tool.Sandbox, toolName, path, workingDir string) (string, error) {
	// Normalize: make absolute.
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDir, path)
	}
	path = filepath.Clean(path)

	// Step 1: Check policy against the literal target path BEFORE opening it.
	if policy != nil {
		if v := policy.Check(toolName, map[string]any{"path": path}); v != tool.VERDICT_ALLOW {
			return "", fmt.Errorf("sandbox denied tool %s: path %q is not allowed", toolName, path)
		}
	}

	// Step 2: Check target parent directory exists (or path itself exists)
	// to prevent creating intermediate directories that shouldn't exist.
	// If the file exists, resolve symlinks.
	resolved := path
	if st, err := os.Lstat(path); err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			evaled, err := filepath.EvalSymlinks(path)
			if err != nil {
				return "", fmt.Errorf("evaluating symlink %q: %w", path, err)
			}
			resolved = evaled
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		// Target doesn't exist yet (e.g. write creating new file).
		// Resolve symlinks on the existing parent directory.
		dir := filepath.Dir(path)
		if evaled, err := filepath.EvalSymlinks(dir); err == nil {
			resolved = filepath.Join(evaled, filepath.Base(path))
		}
	}

	// Step 3: Double-check policy against resolved realpath.
	if policy != nil && resolved != path {
		if v := policy.Check(toolName, map[string]any{"path": resolved}); v != tool.VERDICT_ALLOW {
			return "", fmt.Errorf("sandbox denied tool %s: symlink target %q is not allowed", toolName, resolved)
		}
	}

	return resolved, nil
}

// safeCwd resolves the base working directory to an absolute path.
func safeCwd(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving working dir %q: %w", dir, err)
	}
	return abs, nil
}

// resolveCwd resolves a relative CWD against workingDir and checks sandbox policy.
func resolveCwd(policy tool.Sandbox, toolName, cwd, workingDir string) (string, error) {
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(workingDir, cwd)
	}
	cwd = filepath.Clean(cwd)
	if policy != nil {
		if v := policy.Check(toolName, map[string]any{"path": cwd}); v != tool.VERDICT_ALLOW {
			return "", fmt.Errorf("sandbox denied tool %s: CWD %q is not allowed", toolName, cwd)
		}
	}
	fi, err := os.Stat(cwd)
	if err != nil {
		return "", fmt.Errorf("stat CWD %q: %w", cwd, err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("CWD %q is not a directory", cwd)
	}
	return cwd, nil
}

// atomicWrite writes data to path atomically using a temporary file in the same directory.
// Returns (bytes written, created, error).
func atomicWrite(resolvedPath string, data []byte, perm os.FileMode) (int64, bool, error) {
	if perm == 0 {
		perm = 0644
	}
	dir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, false, fmt.Errorf("creating directory %q: %w", dir, err)
	}

	_, statErr := os.Stat(resolvedPath)
	created := errors.Is(statErr, fs.ErrNotExist)

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return 0, false, fmt.Errorf("creating temp file in %q: %w", dir, err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	n, err := tmpFile.Write(data)
	if err != nil {
		tmpFile.Close()
		return 0, false, fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmpFile.Chmod(perm); err != nil {
		tmpFile.Close()
		return 0, false, fmt.Errorf("setting permissions on temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return 0, false, fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, resolvedPath); err != nil {
		return 0, false, fmt.Errorf("renaming temp file to %q: %w", resolvedPath, err)
	}
	return int64(n), created, nil
}

// sniffMime reads up to 512 bytes from r to detect its MIME type.
func sniffMime(r io.ReadSeeker) (string, error) {
	buf := make([]byte, 512)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}

// isTextMime returns true if the MIME type represents text/code.
func isTextMime(mime string) bool {
	return strings.HasPrefix(mime, "text/") ||
		mime == "application/json" ||
		mime == "application/xml" ||
		mime == "application/javascript" ||
		mime == "image/svg+xml"
}

// isBinaryContent checks if a byte slice contains binary data.
func isBinaryContent(buf []byte) bool {
	if len(buf) == 0 {
		return false
	}
	mime := http.DetectContentType(buf)
	if isTextMime(mime) {
		return false
	}
	limit := len(buf)
	if limit > 512 {
		limit = 512
	}
	for i := 0; i < limit; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return !isTextMime(mime)
}

// readLines reads a file up to maxLines, returning lines, total line count,
// and whether the file was truncated.
func readLines(r io.Reader, startLine, maxLines int) ([]string, int, bool, error) {
	if startLine < 1 {
		startLine = 1
	}

	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, false, err
	}

	if isBinaryContent(buf) {
		return nil, 0, false, fmt.Errorf("cannot read binary file as text")
	}

	allLines := strings.Split(string(buf), "\n")
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" && len(buf) > 0 && buf[len(buf)-1] == '\n' {
		allLines = allLines[:len(allLines)-1]
	}

	total := len(allLines)
	if startLine > total {
		return []string{}, total, false, nil
	}

	end := startLine - 1 + maxLines
	truncated := false
	if end > total {
		end = total
	} else if end < total {
		truncated = true
	}

	slice := allLines[startLine-1 : end]
	return slice, total, truncated, nil
}
