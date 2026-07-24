package skill

import (
	"fmt"
	"os"
	"strings"

	"github.com/bizshuk/agentsdk/utils/frontmatter"
)

// ARGUMENTS_PLACEHOLDER is substituted by ExpandCommand.
const ARGUMENTS_PLACEHOLDER = "$ARGUMENTS"

// Command is one slash command definition.
type Command struct {
	Name string
	Path string
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
	_, body, _ := frontmatter.Parse(string(raw))
	body = strings.TrimSpace(body)
	if strings.Contains(body, ARGUMENTS_PLACEHOLDER) {
		return strings.ReplaceAll(body, ARGUMENTS_PLACEHOLDER, args), nil
	}
	if args != "" {
		return body + "\n\n" + args, nil
	}
	return body, nil
}
