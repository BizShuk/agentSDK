package cmd

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/spf13/cobra"
)

// RegisterList attaches the `list` subcommand to root. It enumerates
// the runs persisted under $LOGDOCTOR_DATA/states and prints a
// one-line summary per run-id. Useful as an `ls`-style operator view.
// ListCmd lists persisted runs.
var ListCmd = &cobra.Command{
	Use:   "list",
	Short: "List persisted runs (read StateStore metadata)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listExecute(cmd)
	},
}

func init() {
	ListCmd.Flags().String("data-dir", "",
		"Directory containing states/ (default: $LOGDOCTOR_DATA or ./data).")
	RootCmd.AddCommand(ListCmd)
}

func listExecute(cmd *cobra.Command) error {
	if _, err := cmd.Flags().GetString("data-dir"); err != nil {
		// --data-dir is optional; bound to keep cobra quiet on typos.
	}
	host, err := agent.Open("logdoctor")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	runs, err := agent.ListRuns(ctx, host)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(runs) == 0 {
		fmt.Fprintln(out, "(no persisted runs)")
		return nil
	}
	fmt.Fprintf(out, "%d run(s):\n", len(runs))
	for _, id := range runs {
		s, err := host.StateStore.Load(ctx, id)
		if err != nil {
			fmt.Fprintf(out, "  %s  <error: %v>\n", id, err)
			continue
		}
		fmt.Fprintf(out, "  %s  turns=%d status=%s\n", id, s.Turn, s.Status)
	}
	return nil
}
