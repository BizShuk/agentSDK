// Command file-agent is a sample that demonstrates agentsdk by driving the
// six built-in tools (Read/Write/Edit/Bash/Glob/Grep) to operate on files.
package main

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/sample/file-agent/cmd"
)

func main() {
	if err := cmd.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
