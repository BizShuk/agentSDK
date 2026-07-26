// Package main is the agentsdk workspace root binary.
//
// The auth CLI lives in the auth module (cd auth && go build .) and the
// LLM protocol proxy lives in the proxy module (cd proxy && go build .);
// both are standalone modules with their own main.go.
//
// This binary currently mounts one subcommand:
//
//	provider   — minimal smoke-test CLI that calls core.Provider directly
//	             (no Agent / Engine / harness); see cmd/provider.go.
//
// Sample agents under sample/* (code-agent, file-agent, greet-agent,
// logdoctor, demo-memory, demo-middleware, demo-strategy) are the canonical
// end-to-end demos.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/bizshuk/agentsdk/cmd"
	"github.com/bizshuk/agentsdk/cmd/agent/wizard"
	_ "github.com/bizshuk/agentsdk/provider/all"
	"github.com/spf13/cobra"
)

const VERSION = "0.1.0"

func main() {
	root := &cobra.Command{
		Use:   "agentsdk",
		Short: "agentsdk workspace root binary",
		Long: strings.TrimSpace(`
agentsdk is a Go agentic loop SDK, LLM protocol proxy, and provider adapter
workspace.

Subcommands:

  provider   smoke-test CLI that calls core.Provider directly

The auth CLI and the LLM protocol proxy live in their own modules and are
built separately:

  cd auth  && go build .   # produces the auth binary (login/list/verify/...)
  cd proxy && go build .   # produces the proxy binary (LLM protocol bridge)

Sample agents under sample/* are the canonical end-to-end demos.`),
		Version: VERSION,
		// main() 負責印錯誤與設定 exit code,cobra 不要再印一次。
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(cmd.ProviderCmd)
	root.AddCommand(wizard.WizardCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
