// Package cmd hosts the logdoctor CLI verbs (root + run + watch + resume + approve + list).
//
// M1 provides only root + run. The other verbs land in M2-M4 alongside
// the feature surface they govern.
//
// Convention: each verb lives in its own file under cmd/<verb>.go,
// matching the gosdk / playground CLI convention.
package cmd

import (
	"github.com/spf13/cobra"
)

// Version / build info — populated via -ldflags in later milestones.
const Version = "0.1.0-m1"

// NewRoot constructs the root command. Subcommands register themselves
// on it via AddCommand. Caller is responsible for Execute().
//
// In M1 only `run` is wired; other verbs hang off this same root.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "logdoctor",
		Short:   "Watch a log, diagnose errors, queue fixes",
		Version: Version,
	}
	root.SetVersionTemplate("logdoctor {{.Version}}\n")
	root.PersistentFlags().Bool("fake", false,
		"Use the deterministic FakeProvider for offline E2E testing. "+
			"No network or API key required. Mutually exclusive with --provider.")
	root.PersistentFlags().String("provider", "",
		"LLM provider: anthropic | ollama | google. "+
			"Mutually exclusive with --fake. Credentials are read from the "+
			"provider's env var (ANTHROPIC_API_KEY / OPENAI_API_KEY / GOOGLE_API_KEY).")
	root.PersistentFlags().Int("max-turns", 5,
		"Maximum steps the agent can take before being killed.")
	return root
}
