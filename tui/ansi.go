package tui

import "strings"

// ANSI-aware text measurement. Skeleton scope: every rune counts as width
// 1 — CJK double-width and grapheme clusters are a documented follow-up.

// VisibleWidth measures the displayed width of s, excluding ANSI CSI and
// OSC escape sequences.
func VisibleWidth(s string) int {
	w := 0
	for _, seg := range splitANSI(s) {
		if !seg.escape {
			w += len([]rune(seg.text))
		}
	}
	return w
}

// TruncateToWidth trims s to at most width columns, appending "…" when
// trimmed. Escape sequences are preserved and never counted.
func TruncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if VisibleWidth(s) <= width {
		return s
	}
	var sb strings.Builder
	used := 0
	for _, seg := range splitANSI(s) {
		if seg.escape {
			sb.WriteString(seg.text)
			continue
		}
		for _, r := range seg.text {
			if used >= width-1 {
				sb.WriteRune('…')
				return sb.String()
			}
			sb.WriteRune(r)
			used++
		}
	}
	return sb.String()
}

// WrapText greedily word-wraps s to width columns, honoring embedded
// newlines. Words longer than width are hard-split.
func WrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		out = append(out, wrapLine(line, width)...)
	}
	return out
}

func wrapLine(line string, width int) []string {
	if VisibleWidth(line) <= width {
		return []string{line}
	}
	var out []string
	var cur strings.Builder
	curWidth := 0
	flush := func() {
		out = append(out, cur.String())
		cur.Reset()
		curWidth = 0
	}
	for word := range strings.FieldsSeq(line) {
		ww := VisibleWidth(word)
		if curWidth > 0 && curWidth+1+ww > width {
			flush()
		}
		if ww > width {
			// Hard-split an over-long word rune-by-rune.
			for _, r := range word {
				if curWidth >= width {
					flush()
				}
				cur.WriteRune(r)
				curWidth++
			}
			continue
		}
		if curWidth > 0 {
			cur.WriteByte(' ')
			curWidth++
		}
		cur.WriteString(word)
		curWidth += ww
	}
	if curWidth > 0 || len(out) == 0 {
		flush()
	}
	return out
}

type ansiSegment struct {
	text   string
	escape bool
}

// splitANSI splits s into plain-text and escape-sequence segments. CSI
// (ESC [ … final byte 0x40–0x7E) and OSC (ESC ] … BEL or ESC \) are
// recognized; other escapes pass through as two-byte sequences.
func splitANSI(s string) []ansiSegment {
	var segs []ansiSegment
	plain := strings.Builder{}
	i := 0
	for i < len(s) {
		if s[i] != 0x1b {
			plain.WriteByte(s[i])
			i++
			continue
		}
		if plain.Len() > 0 {
			segs = append(segs, ansiSegment{text: plain.String()})
			plain.Reset()
		}
		start := i
		i++ // consume ESC
		switch {
		case i < len(s) && s[i] == '[': // CSI
			i++
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
				i++
			}
			if i < len(s) {
				i++ // final byte
			}
		case i < len(s) && s[i] == ']': // OSC
			i++
			for i < len(s) && s[i] != 0x07 && !(s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\') {
				i++
			}
			if i < len(s) {
				if s[i] == 0x07 {
					i++
				} else {
					i += 2 // ESC backslash
				}
			}
		default:
			if i < len(s) {
				i++
			}
		}
		segs = append(segs, ansiSegment{text: s[start:i], escape: true})
	}
	if plain.Len() > 0 {
		segs = append(segs, ansiSegment{text: plain.String()})
	}
	return segs
}
