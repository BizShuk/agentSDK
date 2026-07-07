package tool

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
)

// ReadArgs is the input shape the LLM sees for the read tool.
type ReadArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// ReadOutput is the output shape returned to the LLM.
type ReadOutput struct {
	Content   string `json:"content"`
	Encoding  string `json:"encoding,omitempty"` // "text", "base64"
	Truncated bool   `json:"truncated,omitempty"`
	Size      int64  `json:"size"`
	Mime      string `json:"mime,omitempty"`
}

// Read reads a file's content. Text files are returned as-is; binary
// files are base64-encoded with a MIME type hint. Pagination via Offset
// and Limit supports reading large files in chunks.
type Read struct {
	Inner    *action.TypedTool[ReadArgs, ReadOutput]
	policy   Sandbox
	rootDir  string
	maxBytes int64
}

// NewRead constructs a Read tool. Use opts.MaxBytes to cap the read
// (0 → 1 MiB). policy may be nil for unrestricted reads within rootDir.
func NewRead(opts ReadOptions, policy Sandbox, rootDir string) *Read {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultMaxBytes()
	}
	rd := &Read{
		policy:   policy,
		rootDir:  rootDir,
		maxBytes: opts.MaxBytes,
	}
	rd.Inner = action.NewTypedTool("read",
		"Read file contents. Supports text files (returned as-is) and binary files (base64-encoded with MIME type hint). Use absolute paths. For large files, use offset and limit to paginate.",
		rd.handle)
	return rd
}

// SetRisk changes the risk level. Call before Register.
func (r *Read) SetRisk(level sdkcore.RiskLevel) { r.Inner.SetRisk(level) }

// --- core.Tool interface ---

func (r *Read) Name() string                       { return r.Inner.Name() }
func (r *Read) Description() string                { return r.Inner.Description() }
func (r *Read) Schema() sdkcore.ToolSpec         { return r.Inner.Schema() }
func (r *Read) Risk() sdkcore.RiskLevel            { return r.Inner.Risk() }
func (r *Read) Call(ctx context.Context, raw json.RawMessage) (sdkcore.ToolResult, error) {
	return r.Inner.Call(ctx, raw)
}

// handle is the pure business logic — no JSON, no schema.
func (r *Read) handle(ctx context.Context, a ReadArgs) (ReadOutput, error) {
	wd, err := safeCwd(r.rootDir)
	if err != nil {
		return ReadOutput{}, fmt.Errorf("read: working dir: %w", err)
	}
	resolved, err := resolvePath(r.policy, "read", a.Path, wd)
	if err != nil {
		return ReadOutput{}, fmt.Errorf("read: %w", err)
	}

	f, err := os.Open(resolved)
	if err != nil {
		return ReadOutput{}, fmt.Errorf("read: open %q: %w", resolved, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return ReadOutput{}, fmt.Errorf("read: stat %q: %w", resolved, err)
	}

	// Sniff MIME type.
	mime, err := sniffMime(io.NewSectionReader(f, 0, 512))
	if err != nil {
		return ReadOutput{}, fmt.Errorf("read: detect mime for %q: %w", resolved, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ReadOutput{}, fmt.Errorf("read: seek %q: %w", resolved, err)
	}

	isText := isTextMime(mime)

	if isText {
		return r.readText(f, info.Size(), a.Offset, a.Limit, mime)
	}
	return r.readBinary(f, info.Size(), mime)
}

func (r *Read) readText(f *os.File, fileSize int64, offset, limit int, mime string) (ReadOutput, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = int(r.maxBytes)
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), int(r.maxBytes))

	// Skip to offset.
	lineNum := 0
	var lines []string
	for scanner.Scan() {
		if lineNum < offset {
			lineNum++
			continue
		}
		if len(lines) >= limit {
			return ReadOutput{
				Content:   strings.Join(lines, "\n"),
				Encoding:  "text",
				Truncated: true,
				Size:      fileSize,
				Mime:      mime,
			}, nil
		}
		lineNum++
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return ReadOutput{}, fmt.Errorf("read: scan: %w", err)
	}

	return ReadOutput{
		Content:  strings.Join(lines, "\n"),
		Encoding: "text",
		Size:     fileSize,
		Mime:     mime,
	}, nil
}

func (r *Read) readBinary(f *os.File, size int64, mime string) (ReadOutput, error) {
	lr := io.LimitReader(f, r.maxBytes)
	raw, err := io.ReadAll(lr)
	if err != nil {
		return ReadOutput{}, fmt.Errorf("read: read binary: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	truncated := int64(len(raw)) < size && size > r.maxBytes
	return ReadOutput{
		Content:   encoded,
		Encoding:  "base64",
		Truncated: truncated,
		Size:      size,
		Mime:      mime,
	}, nil
}
