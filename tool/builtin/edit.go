package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
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
const EditDesc = "Replace text in a file. Returns error if old_text is not found or matches multiple times (unless replace_all is true)."

// Edit performs exact substring replacement in a file.
// Requires a sandbox Policy — registration fails without one.
type Edit struct {
	policy  tool.Sandbox
	rootDir string
}

// NewEdit constructs an Edit tool. policy must be non-nil.
func NewEdit(policy tool.Sandbox, rootDir string, opts ...EditOption) (*Edit, error) {
	if policy == nil {
		return nil, errPolicyRequired("edit")
	}
	e := &Edit{
		policy:  policy,
		rootDir: rootDir,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	return e, nil
}

var _ tool.Tool = (*Edit)(nil)

// Name returns the tool's registry name.
func (e *Edit) Name() string { return NAME_EDIT }

// Spec returns the tool's metadata and reflected argument schema.
func (e *Edit) Spec() core.ToolSpec {
	return tool.MustSchemaForTool[EditArgs](
		NAME_EDIT,
		EditDesc,
		core.RISK_LEVEL_HIGH,
	)
}

// Call converts raw JSON arguments and executes the edit operation.
func (e *Edit) Call(
	ctx context.Context,
	raw json.RawMessage,
) (core.ToolResult, error) {
	return tool.CallWithRawMessage(ctx, e.Name(), raw, e.execute)
}

func (e *Edit) execute(_ context.Context, a EditArgs) (EditOutput, error) {
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

	data, err := os.ReadFile(resolved)
	if err != nil {
		return EditOutput{}, fmt.Errorf("edit: read %q: %w", resolved, err)
	}

	content := string(data)
	count := strings.Count(content, a.OldText)
	if count == 0 {
		return EditOutput{}, fmt.Errorf("edit: old_text not found in %q", resolved)
	}
	if count > 1 && !a.ReplaceAll {
		return EditOutput{}, fmt.Errorf("edit: old_text found %d times in %q; set replace_all to true to replace all occurrences", count, resolved)
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

	wrote, _, err := atomicWrite(resolved, []byte(result), 0644)
	if err != nil {
		return EditOutput{}, fmt.Errorf("edit: write %q: %w", resolved, err)
	}
	return EditOutput{Replacements: replacements, BytesAfter: wrote}, nil
}
