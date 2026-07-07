package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
)

// EditArgs is the input shape the LLM sees for the edit tool.
type EditArgs struct {
	Path       string `json:"path"`
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// EditOutput is the output shape returned to the LLM.
type EditOutput struct {
	Replacements int   `json:"replacements"`
	BytesAfter   int64 `json:"bytes_after"`
}

// Edit performs exact string replacement in a file. It is NOT regex-based
// — OldText must match exactly. Single replacement by default; pass
// ReplaceAll for multi-occurrence replacement. Refuses empty OldText and
// zero-match results.
type Edit struct {
	Inner   *action.TypedTool[EditArgs, EditOutput]
	policy  Sandbox
	rootDir string
}

// NewEdit constructs an Edit tool. policy must be non-nil.
func NewEdit(opts EditOptions, policy Sandbox, rootDir string) (*Edit, error) {
	if policy == nil {
		return nil, errPolicyRequired("edit")
	}
	ed := &Edit{
		policy:  policy,
		rootDir: rootDir,
	}
	ed.Inner = action.NewTypedTool("edit",
		"Replace text in a file by exact string match. Single replacement by default; set replace_all to true to replace every occurrence. The edit is atomic (temp file + rename). old_text must match exactly (including whitespace). Refuses to edit if old_text is empty or not found in the file.",
		ed.handle)
	ed.Inner.SetRisk(sdkcore.RISK_LEVEL_HIGH)
	return ed, nil
}

// SetRisk changes the risk level. Call before Register.
func (e *Edit) SetRisk(level sdkcore.RiskLevel) { e.Inner.SetRisk(level) }

// --- core.Tool interface ---

func (e *Edit) Name() string                       { return e.Inner.Name() }
func (e *Edit) Description() string                { return e.Inner.Description() }
func (e *Edit) Schema() sdkcore.ToolSpec         { return e.Inner.Schema() }
func (e *Edit) Risk() sdkcore.RiskLevel            { return e.Inner.Risk() }
func (e *Edit) Call(ctx context.Context, raw json.RawMessage) (sdkcore.ToolResult, error) {
	return e.Inner.Call(ctx, raw)
}

func (e *Edit) handle(_ context.Context, a EditArgs) (EditOutput, error) {
	if a.OldText == "" {
		return EditOutput{}, fmt.Errorf("edit: old_text must not be empty")
	}

	wd, err := safeCwd(e.rootDir)
	if err != nil {
		return EditOutput{}, fmt.Errorf("edit: working dir: %w", err)
	}
	resolved, err := resolvePath(e.policy, "edit", a.Path, wd)
	if err != nil {
		return EditOutput{}, fmt.Errorf("edit: %w", err)
	}

	raw, err := os.ReadFile(resolved)
	if err != nil {
		return EditOutput{}, fmt.Errorf("edit: read %q: %w", resolved, err)
	}
	content := string(raw)

	count := strings.Count(content, a.OldText)
	if count == 0 {
		return EditOutput{}, fmt.Errorf("edit: old_text not found in %q", resolved)
	}
	if count > 1 && !a.ReplaceAll {
		return EditOutput{}, fmt.Errorf("edit: old_text matches %d times in %q (use replace_all=true to replace all, or make old_text more specific)", count, resolved)
	}

	n := 1
	if a.ReplaceAll {
		n = -1
	}
	result := strings.Replace(content, a.OldText, a.NewText, n)
	replacements := count
	if !a.ReplaceAll {
		replacements = 1
	}

	wrote, _, err := atomicWrite(resolved, []byte(result), 0)
	if err != nil {
		return EditOutput{}, fmt.Errorf("edit: write %q: %w", resolved, err)
	}
	return EditOutput{Replacements: replacements, BytesAfter: wrote}, nil
}
