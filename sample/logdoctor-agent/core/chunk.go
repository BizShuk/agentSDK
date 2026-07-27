package core

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
)

const (
	// MAX_CHUNK_BYTES is the aggregate raw-log limit for one analysis cycle.
	MAX_CHUNK_BYTES = 1 << 20
	// SOURCE_SLICE_BYTES is one source's quota in a round-robin pass.
	SOURCE_SLICE_BYTES = 64 << 10
	// ANCHOR_BYTES is the maximum prefix hashed to detect file replacement.
	ANCHOR_BYTES = 4 << 10
)

// Chunk is one bounded, uncommitted set of incremental log bytes.
type Chunk struct {
	Sources []ChunkSource
	Bytes   int
	Backlog bool

	next *checkpoint
}

// ChunkSource attributes a byte range to one app/file source.
type ChunkSource struct {
	Source      string
	StartOffset int64
	EndOffset   int64
	Content     []byte
}

// ChunkReader discovers direct log files and creates at-least-once chunks.
type ChunkReader struct {
	root           string
	checkpointPath string
}

// NewChunkReader returns a reader rooted at a .config directory.
func NewChunkReader(root, checkpointPath string) (*ChunkReader, error) {
	if root == "" {
		return nil, fmt.Errorf("log root must not be empty")
	}
	if checkpointPath == "" {
		return nil, fmt.Errorf("checkpoint path must not be empty")
	}
	return &ChunkReader{
		root:           filepath.Clean(root),
		checkpointPath: filepath.Clean(checkpointPath),
	}, nil
}

// Next reads the next uncommitted chunk without changing the checkpoint.
func (r *ChunkReader) Next(ctx context.Context) (Chunk, []error, error) {
	current, err := loadCheckpoint(ctx, r.checkpointPath)
	if err != nil {
		return Chunk{}, nil, fmt.Errorf("load log cursor: %w", err)
	}

	files, warnings, err := discoverLogFiles(ctx, r.root)
	if err != nil {
		return Chunk{}, warnings, err
	}
	if len(files) == 0 {
		return Chunk{next: &current}, warnings, nil
	}

	ordered := rotateLogFiles(files, current.NextSource)
	sourceIndexes := make(map[string]int, len(ordered))
	sources := make([]ChunkSource, 0, len(ordered))
	disabled := make(map[string]bool)
	lastSource := ""
	total := 0

	for total < MAX_CHUNK_BYTES {
		progressed := false
		for _, file := range ordered {
			if err := ctx.Err(); err != nil {
				return Chunk{}, warnings, err
			}
			if disabled[file.source] {
				continue
			}

			limit := int64(min(SOURCE_SLICE_BYTES, MAX_CHUNK_BYTES-total))
			cursor, exists := current.Files[file.source]
			data, nextCursor, _, readErr := readLogSlice(ctx, file, cursor, exists, limit)
			if readErr != nil {
				warnings = append(warnings, fmt.Errorf("read %s: %w", file.source, readErr))
				disabled[file.source] = true
				continue
			}
			current.Files[file.source] = nextCursor
			if len(data) == 0 {
				continue
			}

			index, exists := sourceIndexes[file.source]
			if !exists {
				index = len(sources)
				sourceIndexes[file.source] = index
				sources = append(sources, ChunkSource{
					Source:      file.source,
					StartOffset: nextCursor.Offset - int64(len(data)),
				})
			}
			sources[index].Content = append(sources[index].Content, data...)
			sources[index].EndOffset = nextCursor.Offset
			progressed = true
			lastSource = file.source
			total += len(data)
		}
		if !progressed || total == MAX_CHUNK_BYTES {
			break
		}
	}

	if lastSource != "" {
		current.NextSource = nextSourceAfter(files, lastSource)
	}
	backlog := false
	if total == MAX_CHUNK_BYTES {
		var backlogWarnings []error
		backlog, backlogWarnings, err = hasUnreadLogs(ctx, files, current, disabled)
		warnings = append(warnings, backlogWarnings...)
		if err != nil {
			return Chunk{}, warnings, err
		}
	}
	next := current
	return Chunk{
		Sources: sources,
		Bytes:   total,
		Backlog: backlog,
		next:    &next,
	}, warnings, nil
}

// Commit atomically persists the cursor carried by chunk.
func (r *ChunkReader) Commit(ctx context.Context, chunk Chunk) error {
	if chunk.next == nil {
		return fmt.Errorf("commit log chunk: chunk was not created by this reader")
	}
	if err := saveCheckpoint(ctx, r.checkpointPath, *chunk.next); err != nil {
		return fmt.Errorf("commit log chunk: %w", err)
	}
	return nil
}

func rotateLogFiles(files []logFile, nextSource string) []logFile {
	if len(files) < 2 || nextSource == "" {
		return files
	}
	start := sort.Search(len(files), func(i int) bool {
		return files[i].source >= nextSource
	})
	if start == len(files) {
		start = 0
	}
	ordered := make([]logFile, 0, len(files))
	ordered = append(ordered, files[start:]...)
	ordered = append(ordered, files[:start]...)
	return ordered
}

func nextSourceAfter(files []logFile, source string) string {
	if len(files) == 0 {
		return ""
	}
	for i := range files {
		if files[i].source == source {
			return files[(i+1)%len(files)].source
		}
	}
	return files[0].source
}

func hasUnreadLogs(
	ctx context.Context,
	files []logFile,
	value checkpoint,
	disabled map[string]bool,
) (bool, []error, error) {
	var warnings []error
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return false, warnings, err
		}
		if disabled[file.source] {
			continue
		}
		cursor, exists := value.Files[file.source]
		data, _, more, err := readLogSlice(ctx, file, cursor, exists, 1)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return false, warnings, ctxErr
			}
			warnings = append(warnings, fmt.Errorf("inspect backlog %s: %w", file.source, err))
			continue
		}
		if len(data) > 0 || more {
			return true, warnings, nil
		}
	}
	return false, warnings, nil
}
