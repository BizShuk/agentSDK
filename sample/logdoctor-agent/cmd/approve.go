package cmd

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/core"
	"github.com/spf13/cobra"
)

// RegisterApprove attaches the approve subcommand. The operator uses
// it to record a decision on a PendingApproval out-of-band; the
// persisted state carries the decision so the next `resume` consumes
// it as an APPROVAL_DECISION input.
func RegisterApprove(root *cobra.Command) {
	f := &approveFlags{}
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve or reject a PendingApproval on a paused run",
		RunE: func(cmd *cobra.Command, args []string) error {
			return approveExecute(cmd, f)
		},
	}
	cmd.Flags().StringVar(&f.runID, "run-id", "",
		"Run ID whose pending approval to decide.")
	cmd.Flags().StringVar(&f.decision, "decision", "approve",
		"Decision: 'approve' | 'reject'.")
	cmd.Flags().StringVar(&f.by, "by", "operator",
		"Operator identifier (recorded on the decision).")
	cmd.Flags().StringVar(&f.dataDir, "data-dir", "",
		"Persistence directory (default: $LOGDOCTOR_DATA or ./data).")
	root.AddCommand(cmd)
}

type approveFlags struct {
	runID    string
	decision string
	by       string
	dataDir  string
}

func approveExecute(cmd *cobra.Command, f *approveFlags) error {
	if f.runID == "" {
		return fmt.Errorf("--run-id is required")
	}
	var decision core.ApprovalDecision
	switch f.decision {
	case "approve":
		decision = core.APPROVAL_DECISION_APPROVE
	case "reject":
		decision = core.APPROVAL_DECISION_REJECT
	default:
		return fmt.Errorf("--decision must be 'approve' or 'reject' (got %q)", f.decision)
	}

	host, err := agent.Open("logdoctor")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := agent.Approve(ctx, host, f.runID, decision, f.by); err != nil {
		return fmt.Errorf("approve run %q: %w", f.runID, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "approval %s recorded on run %q by %s\n",
		string(decision), f.runID, f.by)
	return nil
}
