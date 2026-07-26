package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
)

// NAME_READ is the registry name of the read tool.
const NAME_READ = "read"

// ReadArgs is the input shape the LLM sees for the read tool.
type ReadArgs struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	LineCount int    `json:"line_count,omitempty"`
}

// ReadOutput is the output shape returned to the LLM.
type ReadOutput struct {
	Path      string   `json:"path"`
	Lines     []string `json:"lines"`
	Total     int      `json:"total"`
	Truncated bool     `json:"truncated"`
}

// ReadDesc is the tool description sent to the LLM.
const ReadDesc = "Read text from a file. Supports line ranges via start_line and line_count. Default max lines per call is 800."

// Read reads file contents cleanly, preventing accidental binary output.
type Read struct {
	policy   tool.Sandbox
	rootDir  string
	maxBytes int64
}

// NewRead constructs a Read tool. policy may be nil.
func NewRead(policy tool.Sandbox, rootDir string, opts ...ReadOption) *Read {
	r := &Read{
		policy:   policy,
		rootDir:  rootDir,
		maxBytes: defaultMaxBytes(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

var _ tool.Tool = (*Read)(nil)

// Name returns the tool's registry name.
func (r *Read) Name() string { return NAME_READ }

// Spec returns the tool's metadata and reflected argument schema.
func (r *Read) Spec() core.ToolSpec {
	return tool.MustSchemaForTool[ReadArgs](
		NAME_READ,
		ReadDesc,
		core.RISK_LEVEL_LOW,
	)
}

// Call converts raw JSON arguments and executes the read operation.
func (r *Read) Call(
	ctx context.Context,
	raw json.RawMessage,
) (core.ToolResult, error) {
	return tool.CallWithRawMessage(ctx, r.Name(), raw, r.execute)
}

func (r *Read) execute(_ context.Context, a ReadArgs) (ReadOutput, error) {
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
		return ReadOutput{}, fmt.Errorf("read %q: %w", resolved, err)
	}
	defer f.Close()

	// Check file size against maxBytes limit.
	st, err := f.Stat()
	if err == nil && st.Size() > r.maxBytes {
		return ReadOutput{}, fmt.Errorf("read %q: file size %d exceeds limit %d", resolved, st.Size(), r.maxBytes)
	}

	// Sniff MIME type before reading all lines.
	mime, err := sniffMime(f)
	if err != nil {
		return ReadOutput{}, fmt.Errorf("read %q: MIME check: %w", resolved, err)
	}
	if !isTextMime(mime) {
		return ReadOutput{}, fmt.Errorf("read %q: binary content (%s) cannot be read as text", resolved, mime)
	}

	// Rewind after sniffMime.
	if _, err := f.Seek(0, 0); err != nil {
		return ReadOutput{}, fmt.Errorf("read %q: seek: %w", resolved, err)
	}

	maxLines := a.LineCount
	if maxLines <= 0 {
		maxLines = 800
	}

	lines, total, truncated, err := readLines(f, a.StartLine, maxLines)
	if err != nil {
		return ReadOutput{}, fmt.Errorf("read %q: %w", resolved, err)
	}

	rel, err := filepath.Rel(wd, resolved)
	if err != nil {
		rel = resolved
	}

	return ReadOutput{
		Path:      rel,
		Lines:     lines,
		Total:     total,
		Truncated: truncated,
	}, nil
}
