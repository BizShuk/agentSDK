package tui

import (
	"bytes"
	"io"
	"os"
	"strconv"
)

// DEFAULT_WIDTH is used when the terminal width cannot be determined.
const DEFAULT_WIDTH = 80

// Terminal is the pluggable backend (pi-tui's core abstraction): a writer
// plus a size probe. ProcessTerminal targets a real TTY; VirtualTerminal
// backs headless tests.
type Terminal interface {
	io.Writer
	Size() (width, height int)
}

// ProcessTerminal writes to an os.File (typically os.Stdout). Width comes
// from the COLUMNS environment variable for now — ioctl-based sizing (and
// raw-mode input) arrive with the interactive editor follow-up.
type ProcessTerminal struct {
	Out *os.File
}

// NewProcessTerminal returns a stdout-backed terminal.
func NewProcessTerminal() *ProcessTerminal { return &ProcessTerminal{Out: os.Stdout} }

// Write implements io.Writer.
func (p *ProcessTerminal) Write(b []byte) (int, error) { return p.Out.Write(b) }

// Size implements Terminal.
func (p *ProcessTerminal) Size() (int, int) {
	if v, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && v > 0 {
		return v, 0
	}
	return DEFAULT_WIDTH, 0
}

// VirtualTerminal is the in-memory test double.
type VirtualTerminal struct {
	W, H int
	buf  bytes.Buffer
}

// NewVirtualTerminal returns a fixed-size virtual terminal.
func NewVirtualTerminal(w, h int) *VirtualTerminal { return &VirtualTerminal{W: w, H: h} }

// Write implements io.Writer.
func (v *VirtualTerminal) Write(b []byte) (int, error) { return v.buf.Write(b) }

// Size implements Terminal.
func (v *VirtualTerminal) Size() (int, int) { return v.W, v.H }

// Output returns everything written so far.
func (v *VirtualTerminal) Output() string { return v.buf.String() }

// Reset clears captured output (size is kept).
func (v *VirtualTerminal) Reset() { v.buf.Reset() }
