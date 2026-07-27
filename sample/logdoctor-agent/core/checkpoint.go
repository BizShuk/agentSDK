package core

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// CHECKPOINT_VERSION is the on-disk cursor schema version.
const CHECKPOINT_VERSION = 1

type checkpoint struct {
	Version    int                   `json:"version"`
	NextSource string                `json:"next_source,omitempty"`
	Files      map[string]fileCursor `json:"files"`
}

type fileCursor struct {
	Offset       int64  `json:"offset"`
	AnchorBytes  int64  `json:"anchor_bytes,omitempty"`
	AnchorSHA256 string `json:"anchor_sha256,omitempty"`
}

func newCheckpoint() checkpoint {
	return checkpoint{
		Version: CHECKPOINT_VERSION,
		Files:   make(map[string]fileCursor),
	}
}

func loadCheckpoint(ctx context.Context, filePath string) (checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return checkpoint{}, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return newCheckpoint(), nil
		}
		return checkpoint{}, fmt.Errorf("open checkpoint: %w", err)
	}
	defer func() {
		// Read-only close errors do not change a completed decode.
		_ = file.Close()
	}()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var value checkpoint
	if err := decoder.Decode(&value); err != nil {
		return checkpoint{}, fmt.Errorf("decode checkpoint: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return checkpoint{}, fmt.Errorf("decode checkpoint: multiple JSON values")
		}
		return checkpoint{}, fmt.Errorf("decode checkpoint trailing data: %w", err)
	}
	if err := validateCheckpoint(value); err != nil {
		return checkpoint{}, err
	}
	if value.Files == nil {
		value.Files = make(map[string]fileCursor)
	}
	return value, nil
}

func saveCheckpoint(ctx context.Context, filePath string, value checkpoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if filePath == "" {
		return fmt.Errorf("save checkpoint: path must not be empty")
	}
	if err := validateCheckpoint(value); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode checkpoint: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create checkpoint directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(filePath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create checkpoint temp file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		// Cleanup is best-effort; the primary write error is more useful.
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod checkpoint temp file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write checkpoint temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync checkpoint temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close checkpoint temp file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("replace checkpoint: %w", err)
	}

	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open checkpoint directory: %w", err)
	}
	if err := dirFile.Sync(); err != nil {
		// A failed sync is returned; close is best-effort on this error path.
		_ = dirFile.Close()
		return fmt.Errorf("sync checkpoint directory: %w", err)
	}
	if err := dirFile.Close(); err != nil {
		return fmt.Errorf("close checkpoint directory: %w", err)
	}
	return nil
}

func validateCheckpoint(value checkpoint) error {
	if value.Version != CHECKPOINT_VERSION {
		return fmt.Errorf(
			"checkpoint version %d is unsupported (want %d)",
			value.Version,
			CHECKPOINT_VERSION,
		)
	}
	if value.NextSource != "" && !validSource(value.NextSource) {
		return fmt.Errorf("checkpoint next_source %q is invalid", value.NextSource)
	}
	for source, cursor := range value.Files {
		if !validSource(source) {
			return fmt.Errorf("checkpoint source %q is invalid", source)
		}
		if cursor.Offset < 0 {
			return fmt.Errorf("checkpoint source %q has negative offset", source)
		}
		if cursor.AnchorBytes < 0 || cursor.AnchorBytes > ANCHOR_BYTES {
			return fmt.Errorf("checkpoint source %q has invalid anchor size", source)
		}
		if cursor.AnchorBytes == 0 {
			if cursor.AnchorSHA256 != "" {
				return fmt.Errorf("checkpoint source %q has hash without anchor", source)
			}
			continue
		}
		if len(cursor.AnchorSHA256) != 64 {
			return fmt.Errorf("checkpoint source %q has invalid anchor hash", source)
		}
		if _, err := hex.DecodeString(cursor.AnchorSHA256); err != nil {
			return fmt.Errorf("checkpoint source %q has invalid anchor hash: %w", source, err)
		}
	}
	return nil
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
