package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func readLogSlice(
	ctx context.Context,
	source logFile,
	cursor fileCursor,
	exists bool,
	limit int64,
) (data []byte, next fileCursor, more bool, err error) {
	if err := ctx.Err(); err != nil {
		return nil, fileCursor{}, false, err
	}

	file, err := os.Open(source.path)
	if err != nil {
		return nil, fileCursor{}, false, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close log file: %w", closeErr)
		}
	}()

	info, err := file.Stat()
	if err != nil {
		return nil, fileCursor{}, false, fmt.Errorf("stat opened file: %w", err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(source.info, info) {
		return nil, fileCursor{}, false, fmt.Errorf("file changed during discovery")
	}

	next, err = prepareCursor(file, info.Size(), cursor, exists)
	if err != nil {
		return nil, fileCursor{}, false, err
	}
	if info.Size() <= next.Offset || limit == 0 {
		return nil, next, false, nil
	}

	readBytes := min(limit, info.Size()-next.Offset)
	data = make([]byte, readBytes)
	n, readErr := file.ReadAt(data, next.Offset)
	if readErr != nil && readErr != io.EOF {
		return nil, fileCursor{}, false, fmt.Errorf("read at offset %d: %w", next.Offset, readErr)
	}
	data = data[:n]
	next.Offset += int64(n)

	latest, statErr := file.Stat()
	if statErr != nil {
		return nil, fileCursor{}, false, fmt.Errorf("stat after read: %w", statErr)
	}
	return data, next, latest.Size() > next.Offset, nil
}

func prepareCursor(
	file *os.File,
	size int64,
	current fileCursor,
	exists bool,
) (fileCursor, error) {
	if !exists {
		anchor, err := fileAnchor(file, size)
		if err != nil {
			return fileCursor{}, err
		}
		anchor.Offset = max(0, size-SOURCE_SLICE_BYTES)
		return anchor, nil
	}

	replaced := size < current.Offset
	if !replaced && current.AnchorBytes > 0 {
		if size < current.AnchorBytes {
			replaced = true
		} else {
			hash, err := hashPrefix(file, current.AnchorBytes)
			if err != nil {
				return fileCursor{}, err
			}
			replaced = hash != current.AnchorSHA256
		}
	}
	if replaced {
		return fileAnchor(file, size)
	}
	if current.AnchorBytes == 0 && size > 0 {
		anchor, err := fileAnchor(file, size)
		if err != nil {
			return fileCursor{}, err
		}
		current.AnchorBytes = anchor.AnchorBytes
		current.AnchorSHA256 = anchor.AnchorSHA256
	}
	return current, nil
}

func fileAnchor(file *os.File, size int64) (fileCursor, error) {
	bytes := min(int64(ANCHOR_BYTES), size)
	if bytes == 0 {
		return fileCursor{}, nil
	}
	hash, err := hashPrefix(file, bytes)
	if err != nil {
		return fileCursor{}, err
	}
	return fileCursor{
		AnchorBytes:  bytes,
		AnchorSHA256: hash,
	}, nil
}

func hashPrefix(file *os.File, bytes int64) (string, error) {
	data := make([]byte, bytes)
	n, err := file.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read file anchor: %w", err)
	}
	sum := sha256.Sum256(data[:n])
	return hex.EncodeToString(sum[:]), nil
}
