package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bizshuk/agentsdk/utils/frontmatter"
)

// Registry indexes skills, commands, and subagents from any number of
// directories.
// Later discoveries override earlier ones by name (project over user).
type Registry struct {
	skills    map[string]Skill
	commands  map[string]Command
	subagents map[string]SubAgent
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		skills:    map[string]Skill{},
		commands:  map[string]Command{},
		subagents: map[string]SubAgent{},
	}
}

// DiscoverSkills indexes <dir>/<name>/SKILL.md entries. A missing dir is
// not an error.
func (r *Registry) DiscoverSkills(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("skill: read dir %q: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fields, _, _ := frontmatter.Parse(string(raw))
		name := fields["name"]
		if name == "" {
			name = e.Name()
		}
		r.skills[name] = Skill{
			Name:         name,
			Description:  fields["description"],
			AllowedTools: frontmatter.List(fields["allowed-tools"]),
			Path:         path,
		}
	}
	return nil
}

// DiscoverCommands indexes <dir>/*.md as slash commands named after the
// file base. A missing dir is not an error.
func (r *Registry) DiscoverCommands(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("skill: read commands dir %q: %w", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		base := strings.TrimSuffix(name, ".md")
		r.commands[base] = Command{Name: base, Path: filepath.Join(dir, name)}
	}
	return nil
}

// DiscoverSubagents indexes <dir>/*.md definitions. A missing dir is not an
// error.
func (r *Registry) DiscoverSubagents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("skill: read subagents dir %q: %w", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		def := ParseDef(strings.TrimSuffix(name, ".md"), string(raw))
		r.subagents[def.Name] = def
	}
	return nil
}

// Skills lists summaries sorted by name.
func (r *Registry) Skills() []Skill {
	out := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Commands lists commands sorted by name.
func (r *Registry) Commands() []Command {
	out := make([]Command, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Subagents lists definitions sorted by name.
func (r *Registry) Subagents() []SubAgent {
	out := make([]SubAgent, 0, len(r.subagents))
	for _, d := range r.subagents {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Body loads a skill's markdown body (frontmatter stripped) — the second
// stage of progressive disclosure.
func (r *Registry) Body(name string) (string, error) {
	s, ok := r.skills[name]
	if !ok {
		return "", fmt.Errorf("skill not found: %s", name)
	}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		return "", fmt.Errorf("skill %s: %w", name, err)
	}
	_, body, _ := frontmatter.Parse(string(raw))
	return strings.TrimSpace(body), nil
}

// SystemPrompt renders the progressive-disclosure skill listing for the
// system prompt — names and descriptions only.
func (r *Registry) SystemPrompt() string {
	skills := r.Skills()
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Available skills (load a body only when the task matches):\n")
	for _, s := range skills {
		fmt.Fprintf(&sb, "- %s: %s\n", s.Name, s.Description)
	}
	return strings.TrimSpace(sb.String())
}
