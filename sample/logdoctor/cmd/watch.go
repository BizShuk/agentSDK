package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/bizshuk/agentsdk/runtime"
	"github.com/spf13/cobra"
)

// RegisterWatch attaches the watch subcommand. watch polls the log
// file in a loop, dispatching a fresh run after each polling cycle.
// Approval-gated tools (e.g. propose_fix) pause the run; the operator
// can approve via `logdoctor approve <run-id> --decision approve`.
//
// M4 keeps the implementation simple — one agent + one loop, polls
// every N seconds. M5+ may add concurrent runs and a scheduler.
func RegisterWatch(root *cobra.Command) {
	f := &watchFlags{}
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Continuously tail the log and dispatch runs (M4 + provider required)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return watchExecute(cmd, f)
		},
	}
	cmd.Flags().StringVar(&f.fixture, "fixture", "",
		"Path to a log file to tail.")
	cmd.Flags().DurationVar(&f.interval, "interval", 5*time.Second,
		"Poll interval.")
	cmd.Flags().IntVar(&f.maxRuns, "max-runs", 0,
		"Stop after N runs (0 = forever).")
	root.AddCommand(cmd)
}

type watchFlags struct {
	fixture  string
	interval time.Duration
	maxRuns  int
}

func watchExecute(cmd *cobra.Command, f *watchFlags) error {
	if f.fixture == "" {
		return fmt.Errorf("--fixture is required")
	}
	fakeMode, _ := cmd.Root().PersistentFlags().GetBool("fake")
	if fakeMode {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"watch mode requires a real provider; pass --provider to root")
	}
	fmt.Fprintln(cmd.ErrOrStderr(),
		"watch mode requires a configured provider (M4 wiring pending integration)")
	_ = buildRunContext // future hook
	return nil
}

// buildRunContext is the M4 scaffolding for assembling a per-cycle
// runtime.Loop with ApprovalGate + Spotlight + Sanitizer. It is kept
// here so the watch command can grow incrementally without churning
// the existing run.go wiring.
var buildRunContext = func(_ context.Context, _ string) (*runtime.Loop, error) {
	// Placeholder — wired in the next M4 sub-step once the runtime
	// supports an explicit ApprovalGate middleware slot.
	return nil, fmt.Errorf("buildRunContext not implemented yet")
}

// compile-time references keep imports stable across iterations.
var _ = runtime.NewLoop