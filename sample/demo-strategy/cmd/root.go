package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Version 是 demo-strategy 的版本字串。
const Version = "0.1.0"

// NewRoot 組出 demo-strategy 的指令樹。
//
//	demo-strategy            # 追蹤全部六條策略
//	demo-strategy list       # 列出策略 id 與標題
//	demo-strategy run <id>   # 只追蹤一條
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "demo-strategy",
		Short:   "Trace all six agentsdk planning strategies offline",
		Version: Version,
		Long: strings.TrimSpace(`
demo-strategy 把 agentsdk 的六條 planning 策略各驅動一次,印出 phase 轉移
與 emitted instruction。推理邏輯是真實 SDK code(planning.*.NextStep),
只有環境(model 回覆、tool 結果、review 評語)被腳本化,因此離線、決定性、
不需要 API key。`),
		// 無子指令時 = 追蹤全部。
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, st := range strategies() {
				traceStrategy(cmd.OutOrStdout(), st)
			}
			return nil
		},
	}
	root.SetVersionTemplate("demo-strategy {{.Version}}\n")
	root.AddCommand(newListCmd(), newRunCmd())
	return root
}

// newListCmd 列出可用策略。
func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the available strategies",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-12s %-40s %s\n", "ID", "TITLE", "STYLE")
			for _, st := range strategies() {
				fmt.Fprintf(w, "%-12s %-40s %s\n", st.id, st.title, st.style)
			}
			return nil
		},
	}
}

// newRunCmd 只追蹤指定 id 的一條策略。
func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <id>",
		Short: "Trace a single strategy by id (see `list`)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			for _, st := range strategies() {
				if st.id == id {
					traceStrategy(cmd.OutOrStdout(), st)
					return nil
				}
			}
			return fmt.Errorf("unknown strategy %q — run `demo-strategy list` to see the ids", id)
		},
	}
}