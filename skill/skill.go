// Package skill loads SKILL.md skills, slash commands, and prompt
// templates with claude-code-style progressive disclosure: only name +
// description enter the system prompt; a skill's body is read lazily when
// invoked.
//
// Discovery layout (dir is injected — e.g. ~/.config/<app>/skills and
// <project>/.agentsdk/skills):
//
//	<dir>/<name>/SKILL.md      # skill: frontmatter (name, description, allowed-tools) + body
//	<dir>/commands/<name>.md   # slash command: body with $ARGUMENTS placeholder
//
// Trust decisions for project-local dirs belong to the composition root
// (hook / permission), not here.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bizshuk/agentsdk/internal/frontmatter"
)

// ARGUMENTS_PLACEHOLDER is substituted by ExpandCommand.
const ARGUMENTS_PLACEHOLDER = "$ARGUMENTS"

// Skill is one SKILL.md summary — the body stays on disk until Body().
type Skill struct {
	Name         string
	Description  string
	AllowedTools []string
	Path         string
}

// Command is one slash command definition.
type Command struct {
	Name string
	Path string
}

// Registry indexes skills and commands from any number of directories.
// Later discoveries override earlier ones by name (project over user).
type Registry struct {
	skills   map[string]Skill
	commands map[string]Command
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{skills: map[string]Skill{}, commands: map[string]Command{}}
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
		fields, _ := frontmatter.Parse(string(raw))
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
	_, body := frontmatter.Parse(string(raw))
	return strings.TrimSpace(body), nil
}

// ExpandCommand renders a slash command: $ARGUMENTS is substituted; when
// the body has no placeholder and args are given, they are appended.
func (r *Registry) ExpandCommand(name, args string) (string, error) {
	c, ok := r.commands[name]
	if !ok {
		return "", fmt.Errorf("command not found: %s", name)
	}
	raw, err := os.ReadFile(c.Path)
	if err != nil {
		return "", fmt.Errorf("command %s: %w", name, err)
	}
	_, body := frontmatter.Parse(string(raw))
	body = strings.TrimSpace(body)
	if strings.Contains(body, ARGUMENTS_PLACEHOLDER) {
		return strings.ReplaceAll(body, ARGUMENTS_PLACEHOLDER, args), nil
	}
	if args != "" {
		return body + "\n\n" + args, nil
	}
	return body, nil
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

// RenderTemplate substitutes {{var}} placeholders in a prompt template.
func RenderTemplate(body string, vars map[string]string) string {
	for k, v := range vars {
		body = strings.ReplaceAll(body, "{{"+k+"}}", v)
	}
	return body
}
