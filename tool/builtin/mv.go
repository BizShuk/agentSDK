package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
)

// NAME_MV is the registry name of the mv tool.
const NAME_MV = "mv"

// MvArgs is the input shape the LLM sees for the mv tool.
type MvArgs struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// MvOutput is the output shape returned to the LLM.
type MvOutput struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// MvDesc is the tool description sent to the LLM.
const MvDesc = "Move or rename a file or directory from source to target. When target is an existing directory, source is moved into it under its own name. Missing parent directories of target are created. Never overwrites: fails if the final target already exists. Source and target must be on the same filesystem. Use absolute paths."

// Mv moves a file or directory with os.Rename, refusing to overwrite.
// Requires a sandbox Policy — registration fails without one.
type Mv struct {
	policy  tool.Sandbox
	rootDir string
}

// NewMv constructs an Mv tool. policy must be non-nil.
func NewMv(policy tool.Sandbox, rootDir string) (*Mv, error) {
	if policy == nil {
		return nil, errPolicyRequired(NAME_MV)
	}
	return &Mv{policy: policy, rootDir: rootDir}, nil
}

var _ tool.Tool = (*Mv)(nil)

// Name returns the tool's registry name.
func (m *Mv) Name() string { return NAME_MV }

// Spec returns the tool's metadata and reflected argument schema.
func (m *Mv) Spec() core.ToolSpec {
	return tool.MustSchemaForTool[MvArgs](
		NAME_MV,
		MvDesc,
		core.RISK_LEVEL_HIGH,
	)
}

// Call converts raw JSON arguments and executes the move.
func (m *Mv) Call(
	ctx context.Context,
	raw json.RawMessage,
) (core.ToolResult, error) {
	return tool.CallWithRawMessage(ctx, m.Name(), raw, m.execute)
}

func (m *Mv) execute(_ context.Context, a MvArgs) (MvOutput, error) {
	wd, err := safeCwd(m.rootDir)
	if err != nil {
		return MvOutput{}, fmt.Errorf("mv: working dir: %w", err)
	}
	source, err := m.resolveSource(a.Source, wd)
	if err != nil {
		return MvOutput{}, fmt.Errorf("mv: %w", err)
	}
	target, err := resolvePath(m.policy, NAME_MV, a.Target, wd)
	if err != nil {
		return MvOutput{}, fmt.Errorf("mv: %w", err)
	}
	if st, err := os.Stat(target); err == nil && st.IsDir() {
		target, err = resolvePath(m.policy, NAME_MV, filepath.Join(target, filepath.Base(source)), wd)
		if err != nil {
			return MvOutput{}, fmt.Errorf("mv: %w", err)
		}
	}

	if _, err := os.Lstat(target); err == nil {
		return MvOutput{}, fmt.Errorf("mv: target already exists: %s", target)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return MvOutput{}, fmt.Errorf("mv: stat target %q: %w", target, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return MvOutput{}, fmt.Errorf("mv: create target directory: %w", err)
	}
	if err := os.Rename(source, target); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return MvOutput{}, fmt.Errorf("mv: source and target are on different filesystems: %s -> %s", source, target)
		}
		return MvOutput{}, fmt.Errorf("mv: %w", err)
	}
	return MvOutput{Source: source, Target: target}, nil
}

// resolveSource checks the source against the sandbox and requires it to
// exist. A symlink source is moved as the link itself, so its own location
// is what gets renamed; resolvePath still vets where the link points.
func (m *Mv) resolveSource(path, wd string) (string, error) {
	if _, err := resolvePath(m.policy, NAME_MV, path, wd); err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(wd, path)
	}
	path = filepath.Clean(path)
	if _, err := os.Lstat(path); err != nil {
		return "", fmt.Errorf("stat source %q: %w", path, err)
	}
	return path, nil
}
