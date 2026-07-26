package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Version 是 demo-strategy 的版本字串。
const Version = "0.1.0"

// RootCmd 組出 demo-strategy 的指令樹。
//
//	demo-strategy            # 追蹤全部六條策略
//	demo-strategy list       # 列出策略 id 與標題
//	demo-strategy run <id>   # 只追蹤一條
var RootCmd = &cobra.Command{
	Use:     "demo-strategy",
	Short:   "Trace all six agentsdk reasoning strategies offline",
	Version: Version,
	Long: strings.TrimSpace(`
demo-strategy 把 agentsdk 的六條 reasoning 策略各驅動一次,印出 phase 轉移
與 emitted instruction。推理邏輯是真實 SDK code(reasoning.*.NextStep),
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

// ListCmd 列出可用策略。
var ListCmd = &cobra.Command{
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

// RunCmd 只追蹤指定 id 的一條策略。
var RunCmd = &cobra.Command{
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

func init() {
	RootCmd.SetVersionTemplate("demo-strategy {{.Version}}\n")
	RootCmd.AddCommand(ListCmd, RunCmd)
}
