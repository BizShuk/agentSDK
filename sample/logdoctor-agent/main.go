// Command logdoctor is the sample CLI that demonstrates agentsdk by
// watching a log file, diagnosing errors, and queueing fixes.
package main

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/cmd"
)

func main() {
	root := cmd.NewRoot()
	cmd.RegisterRun(root)
	cmd.RegisterResume(root)
	cmd.RegisterList(root)
	cmd.RegisterWatch(root)
	cmd.RegisterApprove(root)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}