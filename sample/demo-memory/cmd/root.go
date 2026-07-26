package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// Version 是 demo-memory 的版本字串。
const Version = "0.1.0"

// NewRoot 組出 demo-memory 的指令樹。
//
//	demo-memory               # 跑全部三個 demo
//	demo-memory list          # 列出 demo id
//	demo-memory run <id>      # 只跑一個
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "demo-memory",
		Short:   "Offline tour of agentsdk memory: window / compact / checkpoint",
		Version: Version,
		Long: strings.TrimSpace(`
demo-memory 把 agentsdk 支柱 2 (memory) 的三個子元件各跑一次:
有界上下文 Window、無 LLM 的 HeadlineCompactor、以及 checkpoint/recover
的快照 + WAL 重放。全部離線、決定性,不需要 provider 或 API key。`),
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
	root.SetVersionTemplate("demo-memory {{.Version}}\n")
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
			return fmt.Errorf("unknown demo %q — run `demo-memory list` to see the ids", id)
		},
	}
}

func printHeader(w io.Writer, d demo) {
	fmt.Fprintf(w, "\n=== %s ===\n%s\n\n", d.title, d.blurb)
}