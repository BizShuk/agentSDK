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
//	<dir>/<name>.md            # subagent: frontmatter (name, description, provider, model, tools) + body prompt
//
// Trust decisions for project-local dirs belong to the composition root
// (hook / permission), not here.
//
// File layout inside the package:
//
//	skill.go     — package doc, Skill summary type, RenderTemplate helper
//	command.go   — Command summary type, ARGUMENTS_PLACEHOLDER, Registry.ExpandCommand
//	registry.go  — Registry, NewRegistry, Discover{Skills,Commands,Subagents}, listings, Body, SystemPrompt
//	subagent.go  — SubAgent, ParseDef, Spawner (core.Tool "task"), Depth/WithDepth
package skill

import "strings"

// Skill is one SKILL.md summary — the body stays on disk until Body().
type Skill struct {
	Name         string
	Description  string
	AllowedTools []string
	Path         string
}

// RenderTemplate substitutes {{var}} placeholders in a prompt template.
func RenderTemplate(body string, vars map[string]string) string {
	for k, v := range vars {
		body = strings.ReplaceAll(body, "{{"+k+"}}", v)
	}
	return body
}
