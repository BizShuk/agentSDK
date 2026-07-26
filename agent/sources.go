package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/prompt/source"
	"github.com/bizshuk/agentsdk/skill"
)

// This file is the dispatch site that turns the config's source names
// into live Sources. The Sources themselves live in prompt/source — the
// content concerns (Persona, ContextFile, Env, Reminder) and the one
// cross-package adapter (SkillSource, which uses a SkillProvider
// interface to keep skill invisible to prompt/source).
//
// prompt/source depends only on prompt and the standard library; this
// file is the only place in the tree where skill, prompt/source, and
// agent/spec are brought together, because translating the config's
// string names into the right Source is the composition policy and
// belongs at the composition layer.

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
		out = append(out, source.PersonaSource(cfg.Persona))
	}
	if cfg.Prompt == nil {
		return out, nil
	}
	for _, name := range cfg.Prompt.Sources {
		switch name {
		case spec.SOURCE_FILES:
			out = append(out, source.ContextFileSource(promptUserDir(cfg, userDir)))
		case spec.SOURCE_SKILLS:
			// Convert typed-nil *skill.Registry to a true-nil
			// SkillProvider interface so the Source's nil check
			// (interface == nil) catches it. Without this, a
			// `*skill.Registry(nil)` would wrap into a non-nil
			// interface and SkillSource would call SystemPrompt on
			// a nil pointer.
			var prov source.SkillProvider
			if reg != nil {
				prov = reg
			}
			out = append(out, source.SkillSource(prov))
		case spec.SOURCE_ENV:
			out = append(out, source.EnvSource())
		case spec.SOURCE_REMINDER:
			out = append(out, source.ReminderSource())
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

// discoveryRoots returns the ordered directories to search for one kind
// of on-disk extension ("skills", "commands", "agents").
//
// User level first, project second: later registration wins a name
// clash, so a project may override a user's definition. The three
// discovery paths had grown three copies of this walk, each with its own
// copy of the DEFAULT_PROJECT_DIR override — one drifting from the
// others was a matter of time.
func discoveryRoots(cfg Config, userDir, cwd, kind string) []string {
	projectDir := spec.DEFAULT_PROJECT_DIR
	if cfg.Prompt != nil && cfg.Prompt.ProjectDir != "" {
		projectDir = cfg.Prompt.ProjectDir
	}
	return []string{
		filepath.Join(userDir, kind),
		filepath.Join(cwd, projectDir, kind),
	}
}

// discoverSkills loads skills and commands from the configured
// directories, or the conventional roots when the config names none.
func discoverSkills(cfg Config, userDir, cwd string) (*skill.Registry, error) {
	if cfg.Skills == nil {
		return nil, nil
	}
	reg := skill.NewRegistry()

	dirs := cfg.Skills.Dirs
	if len(dirs) == 0 {
		dirs = discoveryRoots(cfg, userDir, cwd, "skills")
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
