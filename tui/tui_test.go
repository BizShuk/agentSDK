package tui

import (
	"strings"
	"testing"
)

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"plain", "hello", 5},
		{"sgr color", "\x1b[31mred\x1b[0m", 3},
		{"osc title", "\x1b]0;title\x07text", 4},
		{"unicode runes", "héllo", 5},
		{"empty", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VisibleWidth(tt.in); got != tt.want {
				t.Fatalf("VisibleWidth(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestTruncateToWidth(t *testing.T) {
	if got := TruncateToWidth("hello world", 5); got != "hell…" {
		t.Fatalf("got %q", got)
	}
	if got := TruncateToWidth("hi", 5); got != "hi" {
		t.Fatalf("short strings pass through, got %q", got)
	}
	colored := "\x1b[31mhello world\x1b[0m"
	got := TruncateToWidth(colored, 5)
	if VisibleWidth(got) != 5 {
		t.Fatalf("visible width = %d, want 5 (%q)", VisibleWidth(got), got)
	}
	if !strings.Contains(got, "\x1b[31m") {
		t.Fatalf("escape stripped: %q", got)
	}
}

func TestWrapText(t *testing.T) {
	lines := WrapText("the quick brown fox jumps", 10)
	for _, l := range lines {
		if VisibleWidth(l) > 10 {
			t.Fatalf("line %q exceeds width", l)
		}
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines: %v", len(lines), lines)
	}

	hard := WrapText("abcdefghijklmnop", 5)
	if len(hard) != 4 {
		t.Fatalf("hard split got %v", hard)
	}

	kept := WrapText("a\n\nb", 10)
	if len(kept) != 3 || kept[1] != "" {
		t.Fatalf("newlines preserved, got %v", kept)
	}
}

func TestRendererDifferential(t *testing.T) {
	term := NewVirtualTerminal(20, 10)
	r := NewRenderer(term)
	text := &Text{Content: "hello"}

	// 1. first flush paints fully inside a synchronized bracket.
	if err := r.Flush(text); err != nil {
		t.Fatal(err)
	}
	first := term.Output()
	if !strings.Contains(first, SYNC_BEGIN) || !strings.Contains(first, "hello") {
		t.Fatalf("first flush missing paint: %q", first)
	}

	// 2. unchanged frame writes zero bytes.
	term.Reset()
	if err := r.Flush(text); err != nil {
		t.Fatal(err)
	}
	if term.Output() != "" {
		t.Fatalf("unchanged flush wrote %q", term.Output())
	}

	// 3. changed frame repaints from region start.
	term.Reset()
	text.Content = "hello\nworld"
	if err := r.Flush(text); err != nil {
		t.Fatal(err)
	}
	out := term.Output()
	if !strings.Contains(out, "\x1b[1A") {
		t.Fatalf("expected cursor-up to region start: %q", out)
	}
	if !strings.Contains(out, "world") {
		t.Fatalf("expected new content: %q", out)
	}

	// 4. shrink clears the leftover line.
	term.Reset()
	text.Content = "bye"
	if err := r.Flush(text); err != nil {
		t.Fatal(err)
	}
	out = term.Output()
	if strings.Count(out, CLEAR_LINE) < 2 {
		t.Fatalf("expected leftover line clear: %q", out)
	}
}

func TestComponents(t *testing.T) {
	c := Container{
		Children: []Component{Text{Content: "a"}, nil, Rule{}, Spacer{N: 2}, Text{Content: "b"}},
		Gap:      1,
	}
	lines := c.Render(4)
	want := []string{"a", "", "────", "", "", "", "", "b"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines %v", len(lines), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}

	l := &Loader{Message: "thinking"}
	before := l.Render(40)[0]
	l.Tick()
	after := l.Render(40)[0]
	if before == after {
		t.Fatal("Tick should change the frame")
	}
}
