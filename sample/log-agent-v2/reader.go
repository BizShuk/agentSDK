package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// APP_NAME is also excluded from discovery to prevent feedback loops.
	APP_NAME = "log-agent-v2"

	// MAX_BATCH_BYTES is the aggregate raw-log limit for one analysis run.
	MAX_BATCH_BYTES = 1 << 20

	// CURSOR_VERSION is the on-disk cursor schema version.
	CURSOR_VERSION = 1
)

// Batch is one bounded, uncommitted set of incremental log bytes.
type Batch struct {
	Parts   []LogPart
	Bytes   int
	Backlog bool

	next *cursor
}

// LogPart attributes one byte range to an application log file.
type LogPart struct {
	Source      string
	StartOffset int64
	EndOffset   int64
	Content     []byte
}

// Reader discovers direct log files and produces at-least-once batches.
type Reader struct {
	root       string
	cursorPath string
}

// NewReader returns a reader rooted at the conventional config directory.
func NewReader(root, cursorPath string) (*Reader, error) {
	if root == "" {
		return nil, fmt.Errorf("log root must not be empty")
	}
	if cursorPath == "" {
		return nil, fmt.Errorf("cursor path must not be empty")
	}
	return &Reader{
		root:       filepath.Clean(root),
		cursorPath: filepath.Clean(cursorPath),
	}, nil
}

// Next reads a batch without changing the on-disk cursor.
func (r *Reader) Next(ctx context.Context) (Batch, []error, error) {
	current, err := loadCursor(ctx, r.cursorPath)
	if err != nil {
		return Batch{}, nil, fmt.Errorf("load log cursor: %w", err)
	}

	files, warnings, err := discoverLogFiles(ctx, r.root)
	if err != nil {
		return Batch{}, warnings, err
	}

	next := cloneCursor(current)
	parts, total, backlog, readWarnings, err := readBatch(ctx, files, next)
	warnings = append(warnings, readWarnings...)
	if err != nil {
		return Batch{}, warnings, err
	}
	return Batch{
		Parts:   parts,
		Bytes:   total,
		Backlog: backlog,
		next:    &next,
	}, warnings, nil
}

// Commit atomically persists the cursor carried by batch.
func (r *Reader) Commit(ctx context.Context, batch Batch) error {
	if batch.next == nil {
		return fmt.Errorf("commit log batch: batch was not created by Reader.Next")
	}
	if err := saveCursor(ctx, r.cursorPath, *batch.next); err != nil {
		return fmt.Errorf("commit log batch: %w", err)
	}
	return nil
}

type cursor struct {
	Version int              `json:"version"`
	Files   map[string]int64 `json:"files"`
}

func newCursor() cursor {
	return cursor{
		Version: CURSOR_VERSION,
		Files:   make(map[string]int64),
	}
}

func cloneCursor(value cursor) cursor {
	out := newCursor()
	for source, offset := range value.Files {
		out.Files[source] = offset
	}
	return out
}

func loadCursor(ctx context.Context, filePath string) (cursor, error) {
	if err := ctx.Err(); err != nil {
		return cursor{}, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return newCursor(), nil
		}
		return cursor{}, fmt.Errorf("open cursor: %w", err)
	}
	defer func() {
		// A close error cannot change a completed read-only decode.
		_ = file.Close()
	}()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var value cursor
	if err := decoder.Decode(&value); err != nil {
		return cursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return cursor{}, fmt.Errorf("decode cursor: multiple JSON values")
		}
		return cursor{}, fmt.Errorf("decode cursor trailing data: %w", err)
	}
	if err := validateCursor(value); err != nil {
		return cursor{}, err
	}
	if value.Files == nil {
		value.Files = make(map[string]int64)
	}
	return value, nil
}

func saveCursor(ctx context.Context, filePath string, value cursor) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateCursor(value); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cursor: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create cursor directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(filePath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create cursor temp file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		// Cleanup is best-effort; the primary write error is more useful.
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod cursor temp file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write cursor temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync cursor temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close cursor temp file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("replace cursor: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return err
	}
	return nil
}

func syncDirectory(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open cursor directory: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync cursor directory: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close cursor directory: %w", err)
	}
	return nil
}

func validateCursor(value cursor) error {
	if value.Version != CURSOR_VERSION {
		return fmt.Errorf(
			"cursor version %d is unsupported (want %d)",
			value.Version,
			CURSOR_VERSION,
		)
	}
	for source, offset := range value.Files {
		if !validSource(source) {
			return fmt.Errorf("cursor source %q is invalid", source)
		}
		if offset < 0 {
			return fmt.Errorf("cursor source %q has negative offset", source)
		}
	}
	return nil
}

type logFile struct {
	source string
	path   string
	info   os.FileInfo
}

func discoverLogFiles(ctx context.Context, root string) ([]logFile, []error, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("scan log root: %w", err)
	}

	var files []logFile
	var warnings []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, warnings, err
		}
		if entry.Name() == APP_NAME {
			continue
		}
		appFiles, appWarnings, err := discoverAppLogs(ctx, entry.Name(), root)
		files = append(files, appFiles...)
		warnings = append(warnings, appWarnings...)
		if err != nil {
			return nil, warnings, err
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].source < files[j].source
	})
	return files, warnings, nil
}

func discoverAppLogs(
	ctx context.Context,
	appName string,
	root string,
) ([]logFile, []error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	appPath := filepath.Join(root, appName)
	appInfo, err := os.Lstat(appPath)
	if err != nil {
		return nil, []error{fmt.Errorf("inspect app %s: %w", appName, err)}, nil
	}
	if appInfo.Mode()&os.ModeSymlink != 0 || !appInfo.IsDir() {
		return nil, nil, nil
	}

	logsPath := filepath.Join(appPath, "logs")
	logsInfo, err := os.Lstat(logsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, []error{fmt.Errorf("inspect %s/logs: %w", appName, err)}, nil
	}
	if logsInfo.Mode()&os.ModeSymlink != 0 || !logsInfo.IsDir() {
		return nil, nil, nil
	}

	entries, err := os.ReadDir(logsPath)
	if err != nil {
		return nil, []error{fmt.Errorf("scan %s/logs: %w", appName, err)}, nil
	}

	var files []logFile
	var warnings []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, warnings, err
		}
		logPath := filepath.Join(logsPath, entry.Name())
		info, err := os.Lstat(logPath)
		if err != nil {
			warnings = append(warnings, fmt.Errorf(
				"inspect %s/logs/%s: %w",
				appName,
				entry.Name(),
				err,
			))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, logFile{
			source: filepath.ToSlash(filepath.Join(appName, entry.Name())),
			path:   logPath,
			info:   info,
		})
	}
	return files, warnings, nil
}

func readBatch(
	ctx context.Context,
	files []logFile,
	next cursor,
) ([]LogPart, int, bool, []error, error) {
	var parts []LogPart
	var warnings []error
	total := 0
	backlog := false

	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, 0, false, warnings, err
		}
		remaining := MAX_BATCH_BYTES - total
		if remaining == 0 {
			backlog = true
			break
		}
		quota := max(1, remaining/(len(files)-i))

		data, start, end, more, err := readLogSlice(
			ctx,
			file,
			next.Files[file.source],
			int64(quota),
		)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, 0, false, warnings, ctxErr
			}
			warnings = append(warnings, fmt.Errorf("read %s: %w", file.source, err))
			continue
		}
		next.Files[file.source] = end
		backlog = backlog || more
		if len(data) == 0 {
			continue
		}
		parts = append(parts, LogPart{
			Source:      file.source,
			StartOffset: start,
			EndOffset:   end,
			Content:     data,
		})
		total += len(data)
	}
	return parts, total, backlog, warnings, nil
}

func readLogSlice(
	ctx context.Context,
	file logFile,
	offset int64,
	limit int64,
) ([]byte, int64, int64, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, 0, false, err
	}

	opened, err := os.Open(file.path)
	if err != nil {
		return nil, 0, 0, false, fmt.Errorf("open: %w", err)
	}
	defer func() {
		// Read errors are reported directly; a read-only close cannot
		// invalidate bytes already returned to the caller.
		_ = opened.Close()
	}()

	info, err := opened.Stat()
	if err != nil {
		return nil, 0, 0, false, fmt.Errorf("stat opened file: %w", err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(file.info, info) {
		return nil, 0, 0, false, fmt.Errorf("file changed during discovery")
	}

	start := offset
	if info.Size() < start {
		start = 0
	}
	if _, err := opened.Seek(start, io.SeekStart); err != nil {
		return nil, 0, 0, false, fmt.Errorf("seek to %d: %w", start, err)
	}

	data, err := io.ReadAll(io.LimitReader(opened, limit))
	if err != nil {
		return nil, 0, 0, false, fmt.Errorf("read: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, 0, false, err
	}
	end := start + int64(len(data))
	return data, start, end, end < info.Size(), nil
}

func validSource(source string) bool {
	if source == "" || strings.HasPrefix(source, "/") || path.Clean(source) != source {
		return false
	}
	parts := strings.Split(source, "/")
	return len(parts) == 2 &&
		parts[0] != "" && parts[0] != "." && parts[0] != ".." &&
		parts[1] != "" && parts[1] != "." && parts[1] != ".."
}
