package tool

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// NAME_EDIT is the registry name of the edit tool.
const NAME_EDIT = "edit"

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

// EditDesc is the tool description sent to the LLM.
const EditDesc = "Replace text in a file by exact string match. Single replacement by default; set replace_all to true to replace every occurrence. The edit is atomic (temp file + rename). old_text must match exactly (including whitespace). Refuses to edit if old_text is empty or not found in the file."

// Edit performs exact string replacement in a file. It is NOT regex-based
// — OldText must match exactly. Single replacement by default; pass
// ReplaceAll for multi-occurrence replacement. Refuses empty OldText and
// zero-match results.
type Edit struct {
	policy  Sandbox
	rootDir string
}

// NewEdit constructs an Edit tool. policy must be non-nil.
func NewEdit(opts EditOptions, policy Sandbox, rootDir string) (*Edit, error) {
	if policy == nil {
		return nil, errPolicyRequired("edit")
	}
	return &Edit{
		policy:  policy,
		rootDir: rootDir,
	}, nil
}

// ToolName returns the tool's registry name.
func (e *Edit) ToolName() string { return NAME_EDIT }

// ToolRisk returns the risk level.
func (e *Edit) ToolRisk() core.RiskLevel { return core.RISK_LEVEL_HIGH }

// Handle is the pure business logic — no JSON, no schema.
func (e *Edit) Handle(_ context.Context, a EditArgs) (EditOutput, error) {
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
