package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/contextfile"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/skill"
)

// This file is the adapter layer that keeps the dependency rule intact.
//
// prompt defines the Source interface but imports nothing except core.
// contextfile and skill produce content but know nothing about prompt.
// Neither imports the other. The wiring lives here, in the composition
// layer, which is the only package allowed to know both sides exist.

// PersonaSource contributes the fixed identity text from Config.Persona.
func PersonaSource(persona string) prompt.Source {
	return prompt.Static(prompt.SLOT_SYSTEM, "persona", persona, prompt.ORDER_PERSONA)
}

// ContextFileSource adapts the AGENTS.md / CLAUDE.md hierarchy.
//
// It re-reads on every call rather than caching: the files are a
// project's live instructions, and a long-running agent should see an
// edit without a restart. The loader already caps its own byte budget.
func ContextFileSource(userDir string, maxBytes int) prompt.Source {
	loader := contextfile.Loader{UserDir: userDir, MaxBytes: maxBytes}
	return prompt.SourceFunc(func(_ context.Context, req prompt.Req) ([]prompt.Section, error) {
		text, _, err := loader.Load(req.Cwd)
		if err != nil {
			return nil, fmt.Errorf("context files: %w", err)
		}
		return []prompt.Section{{
			Slot: prompt.SLOT_SYSTEM, Name: "context_files",
			Text: text, Order: prompt.ORDER_FILES,
		}}, nil
	})
}

// SkillSource adapts a skill registry's progressive-disclosure listing —
// names and descriptions only, with bodies loaded on demand by the skill
// tool rather than pushed into every request.
func SkillSource(reg *skill.Registry) prompt.Source {
	return prompt.SourceFunc(func(context.Context, prompt.Req) ([]prompt.Section, error) {
		if reg == nil {
			return nil, nil
		}
		return []prompt.Section{{
			Slot: prompt.SLOT_SYSTEM, Name: "skills",
			Text: reg.SystemPrompt(), Order: prompt.ORDER_SKILLS,
		}}, nil
	})
}

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
			Slot: prompt.SLOT_SYSTEM, Name: "env",
			Text: strings.Join(lines, "\n"), Order: prompt.ORDER_ENV,
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
func ReminderSource() prompt.Source {
	return prompt.SourceFunc(func(_ context.Context, req prompt.Req) ([]prompt.Section, error) {
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
		return []prompt.Section{{
			Slot: prompt.SLOT_REMINDER, Name: "budget",
			Text:  fmt.Sprintf("<budget>%d of %d turns remaining — wrap up.</budget>", left, max),
			Order: prompt.ORDER_REMINDER,
		}}, nil
	})
}

// BuildSources turns the Prompt block's source names into live Sources.
// Slice position does not matter — each Source carries an Order, and the
// Builder sorts by that, so the config's list is a set, not a sequence.
//
// Persona is added unconditionally because it lives outside the Prompt
// block: it is available at every tier, including one-shot, where the
// block does not exist at all.
//
// Exported so a caller can assemble content without building a whole
// Agent — a preview command, or an application that drives its own loop.
func BuildSources(cfg Config, reg *skill.Registry, userDir string) ([]prompt.Source, error) {
	var out []prompt.Source
	if strings.TrimSpace(cfg.Persona) != "" {
		out = append(out, PersonaSource(cfg.Persona))
	}
	if cfg.Prompt == nil {
		return out, nil
	}
	for _, name := range cfg.Prompt.Sources {
		switch name {
		case spec.SOURCE_FILES:
			out = append(out, ContextFileSource(promptUserDir(cfg, userDir), cfg.Prompt.MaxBytes))
		case spec.SOURCE_SKILLS:
			out = append(out, SkillSource(reg))
		case spec.SOURCE_ENV:
			out = append(out, EnvSource())
		case spec.SOURCE_REMINDER:
			out = append(out, ReminderSource())
		default:
			// spec.Validate already rejects unknown names; reaching here
			// means the two lists drifted apart.
			return nil, fmt.Errorf("agent: unknown prompt source %q", name)
		}
	}
	return out, nil
}

// promptUserDir resolves the user-level context directory: the config's
// explicit value, else the app config root the caller supplied.
func promptUserDir(cfg Config, fallback string) string {
	if cfg.Prompt != nil && cfg.Prompt.UserDir != "" {
		return cfg.Prompt.UserDir
	}
	return fallback
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

// discoverSkills loads skills and commands from the configured directories,
// or the conventional pair when the config names none.
func discoverSkills(cfg Config, userDir, cwd string) (*skill.Registry, error) {
	if cfg.Skills == nil {
		return nil, nil
	}
	reg := skill.NewRegistry()

	dirs := cfg.Skills.Dirs
	if len(dirs) == 0 {
		projectDir := spec.DEFAULT_PROJECT_DIR
		if cfg.Prompt != nil && cfg.Prompt.ProjectDir != "" {
			projectDir = cfg.Prompt.ProjectDir
		}
		// User level first, project second: later registration wins on a
		// name clash, so a project may override a user's skill.
		dirs = []string{
			filepath.Join(userDir, "skills"),
			filepath.Join(cwd, projectDir, "skills"),
		}
	}
	for _, dir := range dirs {
		if err := reg.DiscoverSkills(dir); err != nil {
			return nil, fmt.Errorf("discover skills in %s: %w", dir, err)
		}
		// Commands live beside skills under the same root.
		cmdDir := filepath.Join(filepath.Dir(dir), "commands")
		if err := reg.DiscoverCommands(cmdDir); err != nil {
			return nil, fmt.Errorf("discover commands in %s: %w", cmdDir, err)
		}
	}
	return reg, nil
}
