package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// Version 是 demo-middleware 的版本字串。
const Version = "0.1.0"

// NewRoot 組出 demo-middleware 的指令樹。
//
//	demo-middleware           # 跑全部五個 demo
//	demo-middleware list      # 列出 demo id
//	demo-middleware run <id>  # 只跑一個
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "demo-middleware",
		Short:   "Offline tour of the agentsdk M2 middleware chain",
		Version: Version,
		Long: strings.TrimSpace(`
demo-middleware 把 M2 middleware 鏈的每一層各跑一次:retry、budget、
timeout、loopguard,以及四層的組合 chain。每個 demo 都用一個腳本化的
base dispatcher 收尾,因此離線、決定性、不真的 sleep。`),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, d := range demos() {
				printHeader(cmd.OutOrStdout(), d)
				if err := d.run(cmd.OutOrStdout()); err != nil {
					return fmt.Errorf("%s: %w", d.id, err)
				}
			}
			return nil
		},
	}
	root.SetVersionTemplate("demo-middleware {{.Version}}\n")
	root.AddCommand(newListCmd(), newRunCmd())
	return root
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the available demos",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-12s %s\n", "ID", "TITLE")
			for _, d := range demos() {
				fmt.Fprintf(w, "%-12s %s\n", d.id, d.title)
			}
			return nil
		},
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <id>",
		Short: "Run a single demo by id (see `list`)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			for _, d := range demos() {
				if d.id == id {
					printHeader(cmd.OutOrStdout(), d)
					return d.run(cmd.OutOrStdout())
				}
			}
			return fmt.Errorf("unknown demo %q — run `demo-middleware list` to see the ids", id)
		},
	}
}

func printHeader(w io.Writer, d demo) {
	fmt.Fprintf(w, "\n=== %s ===\n%s\n\n", d.title, d.blurb)
}