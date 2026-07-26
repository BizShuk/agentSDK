package builtin

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
)

// NAME_WRITE is the registry name of the write tool.
const NAME_WRITE = "write"

// WriteArgs is the input shape the LLM sees for the write tool.
type WriteArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// WriteOutput is the output shape returned to the LLM.
type WriteOutput struct {
	Wrote   int64 `json:"wrote"`
	Created bool  `json:"created,omitempty"`
}

// WriteDesc is the tool description sent to the LLM.
const WriteDesc = "Create or overwrite a file with the given content. The write is atomic (temp file + rename). Use absolute paths. Content is written as UTF-8 text."

// Write creates or overwrites a file atomically (write to temp + rename).
// Requires a sandbox Policy — registration fails without one.
type Write struct {
	policy  tool.Sandbox
	rootDir string
	mode    int
}

// NewWrite constructs a Write tool. policy must be non-nil.
func NewWrite(policy tool.Sandbox, rootDir string, opts ...WriteOption) (*Write, error) {
	if policy == nil {
		return nil, errPolicyRequired("write")
	}
	w := &Write{
		policy:  policy,
		rootDir: rootDir,
		mode:    0o644,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(w)
		}
	}
	return w, nil
}

var _ tool.Tool[WriteArgs, WriteOutput] = (*Write)(nil)

// Name returns the tool's registry name.
func (w *Write) Name() string { return NAME_WRITE }

// Risk returns the risk level.
func (w *Write) Risk() core.RiskLevel { return core.RISK_LEVEL_HIGH }

// Desc returns the tool description.
func (w *Write) Desc() string { return WriteDesc }

// Handle is the pure business logic — no JSON, no schema.
func (w *Write) Handle(_ context.Context, a WriteArgs) (WriteOutput, error) {
	wd, err := safeCwd(w.rootDir)
	if err != nil {
		return WriteOutput{}, fmt.Errorf("write: working dir: %w", err)
	}
	resolved, err := resolvePath(w.policy, "write", a.Path, wd)
	if err != nil {
		return WriteOutput{}, fmt.Errorf("write: %w", err)
	}

	n, created, err := atomicWrite(resolved, []byte(a.Content), fs.FileMode(w.mode))
	if err != nil {
		return WriteOutput{}, fmt.Errorf("write: %w", err)
	}
	return WriteOutput{Wrote: n, Created: created}, nil
}
