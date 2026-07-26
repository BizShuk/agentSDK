package source

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/prompt"
)

// ContextFileSource adapts the AGENTS.md / CLAUDE.md hierarchy.
//
// It re-reads on every call rather than caching: the files are a
// project's live instructions, and a long-running agent should see an
// edit without a restart. LoadContextFiles caps its own byte budget.
func ContextFileSource(userDir string) prompt.Source {
	return prompt.SourceFunc(func(_ context.Context, req prompt.Req) ([]prompt.Section, error) {
		text, _, err := prompt.LoadContextFiles(req.Cwd, userDir)
		if err != nil {
			return nil, fmt.Errorf("context files: %w", err)
		}
		return []prompt.Section{{
			Slot:  prompt.SLOT_SYSTEM,
			Name:  "context_files",
			Text:  text,
			Order: prompt.ORDER_FILES,
		}}, nil
	})
}
