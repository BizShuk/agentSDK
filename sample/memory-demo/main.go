// Command memory-demo is a catalog sample for agentsdk 支柱 2 (memory):
// the bounded-context Window, the no-LLM HeadlineCompactor, and the
// checkpoint/recover durability protocol (StateStore + WriteAheadLog).
//
// Every demo is offline and deterministic — real SDK code (memory.Window,
// memory.HeadlineCompactor, checkpoint.Recoverer, filestore) driven over
// canned data and a temp dir. No provider, no API key, no network.
package main

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/sample/memory-demo/cmd"
)

func main() {
	if err := cmd.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}