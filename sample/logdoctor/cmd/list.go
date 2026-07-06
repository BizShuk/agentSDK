package cmd

import (
	"fmt"

	"github.com/bizshuk/agentsdk/memory/filestore"
	"github.com/spf13/cobra"
)

// RegisterList attaches the `list` subcommand to root. It enumerates
// the runs persisted under $LOGDOCTOR_DATA/states and prints a
// one-line summary per run-id. Useful as an `ls`-style operator view.
func RegisterList(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List persisted runs (read StateStore metadata)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listExecute(cmd)
		},
	}
	cmd.Flags().String("data-dir", "",
		"Directory containing states/ (default: $LOGDOCTOR_DATA or ./data).")
	root.AddCommand(cmd)
}

func listExecute(cmd *cobra.Command) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	if dataDir == "" {
		dataDir = dataDirOrDefault(cmd)
	}
	store, err := filestore.NewFileStateStore(dataDir)
	if err != nil {
		return err
	}
	runs, err := store.List(cmd.Context())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(runs) == 0 {
		fmt.Fprintln(out, "(no persisted runs)")
		return nil
	}
	fmt.Fprintf(out, "%d run(s) in %s/states:\n", len(runs), dataDir)
	for _, id := range runs {
		s, err := store.Load(cmd.Context(), id)
		if err != nil {
			fmt.Fprintf(out, "  %s  <error: %v>\n", id, err)
			continue
		}
		fmt.Fprintf(out, "  %s  turns=%d status=%s\n", id, s.Turn, s.Status)
	}
	return nil
}