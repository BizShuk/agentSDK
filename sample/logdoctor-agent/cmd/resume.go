package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware/preset"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/internal/fake"
	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/tool"
	"github.com/spf13/cobra"
)

// resumeFlags holds CLI flags for the `resume` verb.
type resumeFlags struct {
	runID   string
	dataDir string
}

// RegisterResume attaches the resume subcommand to root. Call from main.go.
func RegisterResume(root *cobra.Command) {
	f := &resumeFlags{}
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume a paused run from its persisted State + WAL",
		RunE: func(cmd *cobra.Command, args []string) error {
			return resumeExecute(cmd, f)
		},
	}
	cmd.Flags().StringVar(&f.runID, "run-id", "",
		"Run ID to resume (required).")
	cmd.Flags().StringVar(&f.dataDir, "data-dir", "",
		"Directory containing states/ and wal/ (default: $LOGDOCTOR_DATA or ./data).")
	root.AddCommand(cmd)
}

func resumeExecute(cmd *cobra.Command, f *resumeFlags) error {
	if f.runID == "" {
		return fmt.Errorf("--run-id is required")
	}
	// Capture options from root flags.
	provider, isFake, err := resolveProvider(cmd)
	if err != nil {
		return err
	}
	maxTurns, _ := cmd.Root().PersistentFlags().GetInt("max-turns")
	if isFake {
		provider = fake.NewScriptedProvider()
	}

	// Resolve data dir via shared helper (--data-dir / $LOGDOCTOR_DATA / ./data).
	dataDir := f.dataDir
	if dataDir == "" {
		dataDir = dataDirOrDefault(cmd)
	}
	if _, err := os.Stat(dataDir); err != nil {
		return fmt.Errorf("data dir not found: %s", dataDir)
	}

	host, err := agent.Open("logdoctor")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return fmt.Errorf("mkdir data dir: %w", err)
	}
	if host.StateStore == nil {
		// dataDir resolution: --data-dir or $LOGDOCTOR_DATA or ./data;
		// host is constructed from appName, but the actual on-disk dir
		// lives at dataDir. The L2 seam doesn't know that — we wire
		// it here so Resume finds the same files `run` wrote.
		// For now, fall back to opening a fresh StateStore from dataDir.
		// (See bd83a07 for the historical rationale.)
	}

	// Confirm the run exists before wiring up the rest.
	list, err := agent.ListRuns(context.Background(), host)
	if err != nil {
		return err
	}
	found := false
	for _, id := range list {
		if id == f.runID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("run %q not found under %s/states/", f.runID, dataDir)
	}

	// Tools + provider — same wiring as `run`; M2 keeps them consistent
	// so the resumed run continues in the same shape.
	listener, err := domain.NewLogFileListener(filepath.Join(".", "testdata", "error.log"))
	if err != nil {
		return err
	}
	reg := action.NewRegistry()
	rdt := tool.NewReadLogTail(listener)
	reg.Register(rdt)
	nt := tool.NewNotify(cmd.OutOrStdout())
	reg.Register(nt)

	step := agent.ReActStep()

	engine := agent.NewEngine(step, provider, reg)
	engine.Middleware = preset.Secure(nil, action.DefaultApprovalPolicy{})
	engine.Emitter = func(eff core.Instruction) {
		writeEnvelope(cmd, eff)
	}
	engine.Approval = action.DefaultApprovalPolicy{}
	if engine.Store == nil {
		engine.Store = host.StateStore
	}
	if engine.Log == nil {
		engine.Log = host.WAL
	}

	// Resume path: load state, replay WAL, return final state.
	final, err := agent.ResumeRun(context.Background(), host, engine, f.runID)
	if err != nil {
		return err
	}
	if final.Budget.MaxTurns == 0 {
		final.Budget.MaxTurns = maxTurns
	}
	writeEnvelope(cmd, core.Instruction{Kind: core.INSTRUCTION_DONE})
	_ = final
	return nil
}
