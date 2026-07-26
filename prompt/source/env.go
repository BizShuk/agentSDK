package source

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/prompt"
)

// EnvSource contributes the working environment: directory, date, and git
// branch when there is one.
//
// It sorts last among system sections because it changes every run. A
// provider's prompt cache keys on a stable prefix, so volatile facts have
// to sit behind the stable ones or they invalidate the whole thing.
func EnvSource() prompt.Source {
	return prompt.SourceFunc(func(ctx context.Context, req prompt.Req) ([]prompt.Section, error) {
		cwd := req.Cwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		lines := []string{
			"## Environment",
			"",
			"working directory: " + cwd,
			"date: " + time.Now().Format("2006-01-02"),
		}
		if branch := gitBranch(ctx, cwd); branch != "" {
			lines = append(lines, "git branch: "+branch)
		}
		return []prompt.Section{{
			Slot:  prompt.SLOT_SYSTEM,
			Name:  "env",
			Text:  strings.Join(lines, "\n"),
			Order: prompt.ORDER_ENV,
		}}, nil
	})
}

// gitBranch returns the current branch, or "" when cwd is not a work tree
// or git is unavailable. Failure is not an error: the environment section
// is informational, and an agent must still run outside a repository.
//
// gitBranch is intentionally unexported and lives here because it is used
// only by EnvSource — copy if you need similar logic in another source.
func gitBranch(ctx context.Context, dir string) string {
	if dir == "" {
		return ""
	}
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
