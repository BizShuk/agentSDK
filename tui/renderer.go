package tui

import (
	"fmt"
	"strings"
)

// Synchronized-output brackets (CSI 2026): terminals apply the whole
// update atomically, eliminating flicker.
const (
	SYNC_BEGIN = "\x1b[?2026h"
	SYNC_END   = "\x1b[?2026l"

	CLEAR_LINE = "\r\x1b[2K"
)

// Renderer repaints a region of the main screen buffer differentially —
// never the alternate screen, so scrollback stays intact (the pi-tui
// design decision this module inherits).
//
// Skeleton strategy set:
//  1. first flush → full paint
//  2. unchanged frame → zero bytes written
//  3. changed or resized frame → cursor-up to region start, repaint
//     line-by-line, clear leftovers when the region shrank
//
// Per-line diff (skipping unchanged lines with cursor moves) is the
// documented next refinement.
type Renderer struct {
	Term Terminal

	prev      []string
	prevWidth int
}

// NewRenderer binds a terminal.
func NewRenderer(term Terminal) *Renderer { return &Renderer{Term: term} }

// Flush renders root at the current terminal width.
func (r *Renderer) Flush(root Component) error {
	width, _ := r.Term.Size()
	if width <= 0 {
		width = DEFAULT_WIDTH
	}
	lines := root.Render(width)
	if width == r.prevWidth && equalLines(lines, r.prev) {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(SYNC_BEGIN)
	if n := len(r.prev); n > 0 {
		fmt.Fprintf(&sb, "\x1b[%dA", n) // cursor to region start
	}
	for _, line := range lines {
		sb.WriteString(CLEAR_LINE)
		sb.WriteString(TruncateToWidth(line, width))
		sb.WriteString("\n")
	}
	extra := len(r.prev) - len(lines)
	for range extra {
		sb.WriteString(CLEAR_LINE)
		sb.WriteString("\n")
	}
	if extra > 0 {
		fmt.Fprintf(&sb, "\x1b[%dA", extra) // back to region end
	}
	sb.WriteString(SYNC_END)

	if _, err := r.Term.Write([]byte(sb.String())); err != nil {
		return err
	}
	r.prev = lines
	r.prevWidth = width
	return nil
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
