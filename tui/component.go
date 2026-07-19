package tui

import "strings"

// Component is the minimal render contract (pi-tui's shape): given a
// width, produce display lines. Input handling is an optional second
// interface so pure-output components stay trivial.
type Component interface {
	Render(width int) []string
}

// InputHandler is implemented by focusable components. Raw-mode key
// dispatch arrives with the interactive editor follow-up; the contract is
// fixed now so components can already implement it.
type InputHandler interface {
	HandleInput(data []byte) bool // true = consumed
}

// Text is a word-wrapped text block.
type Text struct {
	Content string
}

// Render implements Component.
func (t Text) Render(width int) []string {
	return WrapText(t.Content, width)
}

// Spacer renders N blank lines.
type Spacer struct {
	N int
}

// Render implements Component.
func (s Spacer) Render(int) []string {
	if s.N <= 0 {
		return nil
	}
	return make([]string, s.N)
}

// LOADER_FRAMES is the braille spinner cycle.
var LOADER_FRAMES = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Loader is an animated spinner line; call Tick between flushes.
type Loader struct {
	Message string
	frame   int
}

// Tick advances the spinner.
func (l *Loader) Tick() { l.frame++ }

// Render implements Component.
func (l *Loader) Render(width int) []string {
	frame := LOADER_FRAMES[l.frame%len(LOADER_FRAMES)]
	return []string{TruncateToWidth(frame+" "+l.Message, width)}
}

// Container stacks children vertically with Gap blank lines between.
type Container struct {
	Children []Component
	Gap      int
}

// Render implements Component.
func (c Container) Render(width int) []string {
	var out []string
	for i, child := range c.Children {
		if child == nil {
			continue
		}
		if i > 0 && c.Gap > 0 {
			out = append(out, make([]string, c.Gap)...)
		}
		out = append(out, child.Render(width)...)
	}
	return out
}

// Rule renders a horizontal divider.
type Rule struct{}

// Render implements Component.
func (Rule) Render(width int) []string {
	if width <= 0 {
		return []string{""}
	}
	return []string{strings.Repeat("─", width)}
}
