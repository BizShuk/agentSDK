package prompt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// The built-in Sources: everything that can be assembled from this
// package plus the standard library.
//
// They live here rather than in the composition layer because they are a
// CONTENT decision, not a WIRING one. A Source that needs nothing but
// prompt's own vocabulary has no business sitting in the package whose
// job is knowing that two other packages exist — that layer should hold
// only the adapters it alone can write (a skill registry's index, an
// application's own data).
//
// The dependency rule is unchanged: this file imports core (via the
// package) and the standard library, nothing else.

// PersonaSource contributes fixed identity text.
//
// It is a thin alias for Static at ORDER_PERSONA, named because "the
// persona" is a concept the config layer spells out and a reader should
// be able to grep for.
func PersonaSource(persona string) Source {
	return Static(SLOT_SYSTEM, "persona", persona, ORDER_PERSONA)
}

// ContextFileSource adapts the AGENTS.md / CLAUDE.md hierarchy.
//
// It re-reads on every call rather than caching: the files are a
// project's live instructions, and a long-running agent should see an
// edit without a restart. LoadContextFiles caps its own byte budget.
func ContextFileSource(userDir string) Source {
	return SourceFunc(func(_ context.Context, req Req) ([]Section, error) {
		text, _, err := LoadContextFiles(req.Cwd, userDir)
		if err != nil {
			return nil, fmt.Errorf("context files: %w", err)
		}
		return []Section{{
			Slot: SLOT_SYSTEM, Name: "context_files",
			Text: text, Order: ORDER_FILES,
		}}, nil
	})
}

// EnvSource contributes the working environment: directory, date, and git
// branch when there is one.
//
// It sorts last among system sections because it changes every run. A
// provider's prompt cache keys on a stable prefix, so volatile facts have
// to sit behind the stable ones or they invalidate the whole thing.
func EnvSource() Source {
	return SourceFunc(func(ctx context.Context, req Req) ([]Section, error) {
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
		return []Section{{
			Slot: SLOT_SYSTEM, Name: "env",
			Text: strings.Join(lines, "\n"), Order: ORDER_ENV,
		}}, nil
	})
}

// ReminderSource re-states the run's remaining budget each turn.
//
// This is the seam the design leaves open for "remind the model of the
// last response" or "restate the outstanding TODOs": a reminder reads
// Req.State and contributes to SLOT_REMINDER, which rides with the user
// message. It never rewrites the system prompt — doing so would break the
// cached prefix every turn, and trimming history stays memory's job.
func ReminderSource() Source {
	return SourceFunc(func(_ context.Context, req Req) ([]Section, error) {
		max := req.State.Budget.MaxTurns
		if max <= 0 {
			return nil, nil
		}
		left := max - req.State.Turn
		if left > 3 || left < 0 {
			// Only worth saying when it starts to matter. A reminder on
			// every turn is noise the model learns to ignore.
			return nil, nil
		}
		return []Section{{
			Slot: SLOT_REMINDER, Name: "budget",
			Text:  fmt.Sprintf("<budget>%d of %d turns remaining — wrap up.</budget>", left, max),
			Order: ORDER_REMINDER,
		}}, nil
	})
}

// gitBranch returns the current branch, or "" when cwd is not a work tree
// or git is unavailable. Failure is not an error: the environment section
// is informational, and an agent must still run outside a repository.
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
