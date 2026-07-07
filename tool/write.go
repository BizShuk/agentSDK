package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
)

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

// Write creates or overwrites a file atomically (write to temp + rename).
// Requires a sandbox Policy — registration fails without one.
type Write struct {
	Inner   *action.TypedTool[WriteArgs, WriteOutput]
	policy  Sandbox
	rootDir string
	mode    int
}

// NewWrite constructs a Write tool. opts.DefaultMode is the file
// permission for new files (0 → 0o644). policy must be non-nil.
func NewWrite(opts WriteOptions, policy Sandbox, rootDir string) (*Write, error) {
	if policy == nil {
		return nil, errPolicyRequired("write")
	}
	if opts.DefaultMode == 0 {
		opts.DefaultMode = 0o644
	}
	wr := &Write{
		policy:  policy,
		rootDir: rootDir,
		mode:    opts.DefaultMode,
	}
	wr.Inner = action.NewTypedTool("write",
		"Create or overwrite a file with the given content. The write is atomic (temp file + rename). Use absolute paths. Content is written as UTF-8 text.",
		wr.handle)
	wr.Inner.SetRisk(sdkcore.RISK_LEVEL_HIGH)
	return wr, nil
}

// SetRisk changes the risk level. Call before Register.
func (w *Write) SetRisk(level sdkcore.RiskLevel) { w.Inner.SetRisk(level) }

// --- core.Tool interface ---

func (w *Write) Name() string                       { return w.Inner.Name() }
func (w *Write) Description() string                { return w.Inner.Description() }
func (w *Write) Schema() sdkcore.ToolSpec         { return w.Inner.Schema() }
func (w *Write) Risk() sdkcore.RiskLevel            { return w.Inner.Risk() }
func (w *Write) Call(ctx context.Context, raw json.RawMessage) (sdkcore.ToolResult, error) {
	return w.Inner.Call(ctx, raw)
}

func (w *Write) handle(_ context.Context, a WriteArgs) (WriteOutput, error) {
	wd, err := safeCwd(w.rootDir)
	if err != nil {
		return WriteOutput{}, fmt.Errorf("write: working dir: %w", err)
	}
	resolved, err := resolvePath(w.policy, "write", a.Path, wd)
	if err != nil {
		return WriteOutput{}, fmt.Errorf("write: %w", err)
	}

	n, created, err := atomicWrite(resolved, []byte(a.Content), w.mode)
	if err != nil {
		return WriteOutput{}, fmt.Errorf("write: %w", err)
	}
	return WriteOutput{Wrote: n, Created: created}, nil
}
