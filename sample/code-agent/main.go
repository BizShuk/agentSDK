// Command code-agent is the composition sample that wires the whole
// harness — skill, hook, permission, session, subagent — into
// one CLI with three surfaces: interactive TUI (tui module), print mode
// (-p), and stream-json mode (--json, wire module). It is the first real
// caller of the tui module.
package main

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/sample/code-agent/cmd"
)

func main() {
	if err := cmd.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
