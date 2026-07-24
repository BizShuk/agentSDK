package wizard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/bizshuk/agentsdk/agent/spec"
)

func (w *wizard) section(title string) {
	if w.yes {
		return
	}
	fmt.Fprintf(w.out, "\n=== %s ===\n", title)
}

func (w *wizard) readLine() string {
	if !w.in.Scan() {
		w.yes = true
		return ""
	}
	return strings.TrimSpace(w.in.Text())
}

func (w *wizard) ask(label, current string) string {
	if w.yes {
		return current
	}
	if current != "" {
		fmt.Fprintf(w.out, "%s [%s]: ", label, current)
	} else {
		fmt.Fprintf(w.out, "%s []: ", label)
	}
	if line := w.readLine(); line != "" {
		return line
	}
	return current
}

func (w *wizard) askInt(label string, current int) int {
	got := w.ask(label, strconv.Itoa(current))
	n, err := strconv.Atoi(got)
	if err != nil {
		return current
	}
	return n
}

func (w *wizard) askList(label string, current []string) []string {
	got := w.ask(label, strings.Join(current, ", "))
	if strings.TrimSpace(got) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(got, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (w *wizard) confirm(label string, current bool) bool {
	def := "y"
	if !current {
		def = "n"
	}
	got := strings.ToLower(w.ask(label+" (y/n)", def))
	return strings.HasPrefix(got, "y")
}

func (w *wizard) choose(label string, choices []spec.Choice, current string) string {
	if current == "" {
		current = spec.DefaultOf(choices)
	}
	if w.yes || len(choices) == 0 {
		return current
	}
	if w.isTTY {
		if sel, ok := w.chooseTUI(label, choices, current, false); ok && len(sel) > 0 {
			return sel[0]
		}
	}

	fmt.Fprintf(w.out, "\n%s:\n", label)
	for i, c := range choices {
		mark := " "
		if c.Value == current {
			mark = "*"
		}
		fmt.Fprintf(w.out, " %s %d) %-18s %s\n", mark, i+1, c.Value, c.Note)
	}
	fmt.Fprintf(w.out, "select [%s]: ", current)

	line := w.readLine()
	if line == "" {
		return current
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(choices) {
		return choices[n-1].Value
	}
	for _, c := range choices {
		if strings.EqualFold(c.Value, line) {
			return c.Value
		}
	}
	fmt.Fprintf(w.out, "  (unrecognized %q, keeping %s)\n", line, current)
	return current
}

func (w *wizard) chooseMulti(label string, choices []spec.Choice, current []string) []string {
	if w.yes || len(choices) == 0 {
		return current
	}
	if w.isTTY {
		if sel, ok := w.chooseTUI(label, choices, strings.Join(current, ","), true); ok {
			return sel
		}
	}

	fmt.Fprintf(w.out, "\n%s:\n", label)
	for i, c := range choices {
		mark := " "
		if slices.Contains(current, c.Value) {
			mark = "*"
		}
		fmt.Fprintf(w.out, " %s %d) %-18s %s\n", mark, i+1, c.Value, c.Note)
	}
	fmt.Fprintf(w.out, "select, comma separated [%s]: ", strings.Join(current, ","))

	line := w.readLine()
	if line == "" {
		return current
	}
	var out []string
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n >= 1 && n <= len(choices) {
			out = append(out, choices[n-1].Value)
			continue
		}
		for _, c := range choices {
			if strings.EqualFold(c.Value, part) {
				out = append(out, c.Value)
			}
		}
	}
	return out
}
