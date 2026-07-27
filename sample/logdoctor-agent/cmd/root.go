// Package cmd hosts the logdoctor CLI.
package cmd

import (
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/minimax"
	"github.com/spf13/cobra"
)

// New returns a fresh logdoctor command.
func New() *cobra.Command {
	root := &cobra.Command{
		Use:               "logdoctor",
		Short:             "Continuously analyze application logs and suggest fixes",
		Version:           "0.1.0-m1",
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.SetVersionTemplate("logdoctor {{.Version}}\n")
	root.AddCommand(newWatchCommand())
	return root
}

// newMiniMaxProvider constructs logdoctor's only model provider.
func newMiniMaxProvider() (core.Provider, error) {
	selected, err := provider.New(provider.DEFAULT_NAME, provider.Options{})
	if err != nil {
		return nil, fmt.Errorf("create MiniMax provider: %w", err)
	}
	return selected, nil
}
