// Command logdoctor watches application logs and prints read-only diagnoses.
package main

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/cmd"
)

func main() {
	if err := cmd.New().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
