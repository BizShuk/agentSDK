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

// BuildSources turns configured prompt source names into live sources.
// Persona is always included when set.
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
			// Avoid wrapping a typed nil registry in a non-nil interface.
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
			return nil, fmt.Errorf("agent: unknown prompt source %q", name)
		}
	}
	return out, nil
}

func promptUserDir(cfg Config, fallback string) string {
	if cfg.Prompt != nil && cfg.Prompt.UserDir != "" {
		return cfg.Prompt.UserDir
	}
	return fallback
}

// discoveryRoots returns user then project roots, so project definitions
// win registration conflicts.
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
		cmdDir := filepath.Join(filepath.Dir(dir), "commands")
		if err := reg.DiscoverCommands(cmdDir); err != nil {
			return nil, fmt.Errorf("discover commands in %s: %w", cmdDir, err)
		}
	}
	return reg, nil
}
