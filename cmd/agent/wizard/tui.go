package wizard

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bizshuk/agentsdk/agent/spec"
)

// chooseTUI handles interactive menu navigation using raw terminal mode.
// It returns the selected string(s) and true, or nil/false if raw mode fails.
func (w *wizard) chooseTUI(label string, choices []spec.Choice, currentInitial string, multi bool) ([]string, bool) {
	sttyGet := exec.Command("stty", "-g")
	sttyGet.Stdin = os.Stdin
	sttyState, err := sttyGet.Output()
	if err != nil {
		return nil, false
	}

	sttyRaw := exec.Command("stty", "raw", "-echo")
	sttyRaw.Stdin = os.Stdin
	if err := sttyRaw.Run(); err != nil {
		return nil, false
	}

	defer func() {
		restore := exec.Command("stty", strings.TrimSpace(string(sttyState)))
		restore.Stdin = os.Stdin
		_ = restore.Run()
	}()

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		tty = os.Stdin
	} else {
		defer tty.Close()
	}

	cursor := 0
	selectedMap := make(map[string]bool)
	if multi {
		for _, v := range strings.Split(currentInitial, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				selectedMap[v] = true
			}
		}
	} else {
		for i, c := range choices {
			if c.Value == currentInitial {
				cursor = i
				break
			}
		}
	}

	render := func(first bool) {
		linesCount := len(choices) + 2
		if !first {
			fmt.Fprintf(w.out, "\x1b[%dA", linesCount)
		}
		promptInfo := "Enter to select"
		if multi {
			promptInfo = "Space to toggle, Enter to confirm"
		}
		fmt.Fprintf(w.out, "\r\x1b[2K%s (use ↑/↓ arrow keys to navigate, %s):\r\n", label, promptInfo)
		for i, c := range choices {
			pointer := "  "
			if i == cursor {
				pointer = "> "
			}
			mark := "( )"
			if multi {
				if selectedMap[c.Value] {
					mark = "[*]"
				} else {
					mark = "[ ]"
				}
			} else {
				if i == cursor {
					mark = "(*)"
				}
			}
			fmt.Fprintf(w.out, "\r\x1b[2K%s%s %-18s %s\r\n", pointer, mark, c.Value, c.Note)
		}
		fmt.Fprintf(w.out, "\r\x1b[2KPress Enter to accept current selection\r\n")
	}

	render(true)

	buf := make([]byte, 16)
	for {
		n, err := tty.Read(buf)
		if err != nil || n == 0 {
			break
		}
		if buf[0] == 3 { // Ctrl+C
			return nil, false
		}
		if buf[0] == '\r' || buf[0] == '\n' { // Enter
			break
		}
		if multi && buf[0] == ' ' { // Space
			val := choices[cursor].Value
			selectedMap[val] = !selectedMap[val]
			render(false)
			continue
		}
		// Arrow keys or k/j
		if (n >= 3 && buf[0] == 27 && buf[1] == 91 && buf[2] == 65) || buf[0] == 'k' || buf[0] == 'K' {
			cursor--
			if cursor < 0 {
				cursor = len(choices) - 1
			}
			render(false)
			continue
		}
		if (n >= 3 && buf[0] == 27 && buf[1] == 91 && buf[2] == 66) || buf[0] == 'j' || buf[0] == 'J' {
			cursor++
			if cursor >= len(choices) {
				cursor = 0
			}
			render(false)
			continue
		}
	}

	if multi {
		var out []string
		for _, c := range choices {
			if selectedMap[c.Value] {
				out = append(out, c.Value)
			}
		}
		return out, true
	}
	return []string{choices[cursor].Value}, true
}
