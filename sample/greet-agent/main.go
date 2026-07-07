// Command greet-agent is the minimal sample that demonstrates agentsdk
// by greeting a person by name.
package main

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/sample/greet-agent/cmd"
)

func main() {
	if err := cmd.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
