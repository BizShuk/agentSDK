package cmd

import (
	"fmt"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/memory/filestore"
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
	switch f.decision {
	case "approve":
		f.decision = string(core.APPROVAL_DECISION_APPROVE)
	case "reject":
		f.decision = string(core.APPROVAL_DECISION_REJECT)
	default:
		return fmt.Errorf("--decision must be 'approve' or 'reject' (got %q)", f.decision)
	}

	dataDir := f.dataDir
	if dataDir == "" {
		dataDir = dataDirOrDefault(cmd)
	}
	store, err := filestore.NewJSONFileStateStore(dataDir)
	if err != nil {
		return err
	}
	s, err := store.Load(cmd.Context(), f.runID)
	if err != nil {
		return fmt.Errorf("load run %q: %w", f.runID, err)
	}

	updated := false
	for i := range s.PendingApprovals {
		if s.PendingApprovals[i].Decision == "" {
			s.PendingApprovals[i].Decision = core.ApprovalDecision(f.decision)
			now := time.Now().UTC()
			s.PendingApprovals[i].DecidedAt = &now
			s.PendingApprovals[i].DecidedBy = f.by
			updated = true
			break
		}
	}
	if !updated {
		return fmt.Errorf("no open PendingApproval on run %q", f.runID)
	}
	if err := store.Save(cmd.Context(), s); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "approval %s recorded on run %q by %s\n",
		f.decision, f.runID, f.by)
	return nil
}
