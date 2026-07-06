package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/memory/filestore"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor/internal/fake"
	"github.com/bizshuk/agentsdk/sample/logdoctor/tool"
	"github.com/spf13/cobra"
)

// resumeFlags holds CLI flags for the `resume` verb.
type resumeFlags struct {
	runID  string
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
	fakeMode, _ := cmd.Root().PersistentFlags().GetBool("fake")
	maxTurns, _ := cmd.Root().PersistentFlags().GetInt("max-turns")
	if !fakeMode {
		return fmt.Errorf("M2 only supports --fake; real providers are M4")
	}

	// Resolve data dir via shared helper (--data-dir / $LOGDOCTOR_DATA / ./data).
	dataDir := f.dataDir
	if dataDir == "" {
		dataDir = dataDirOrDefault(cmd)
	}
	if _, err := os.Stat(dataDir); err != nil {
		return fmt.Errorf("data dir not found: %s", dataDir)
	}

	store, err := filestore.NewFileStateStore(dataDir)
	if err != nil {
		return err
	}
	wal, err := filestore.NewFileWAL(dataDir)
	if err != nil {
		return err
	}

	// Confirm the run exists before wiring up the rest.
	list, err := store.List(context.Background())
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
	provider := fake.NewScriptedProvider()
	reg := action.NewRegistry()
	rdt := tool.NewReadLogTail(listener)
	reg.Register(rdt)
	nt := tool.NewNotify(cmd.OutOrStdout())
	reg.Register(nt)

	step := core.NewStep(map[core.ThinkingKind]core.ThinkingPattern{
		core.THINK_REACT: planning.NewReAct(),
	})

	loop := runtime.NewLoop(step, provider, reg)
	loop.Emitter = func(eff core.Effect) {
		writeEnvelope(cmd, eff)
	}
	loop.Approval = allowAllApproval{}
	loop.Store = store
	loop.WAL = wal

	// Resume path: load state, replay WAL, return final state.
	final, err := loop.Resume(context.Background(), f.runID)
	if err != nil {
		return err
	}
	if final.Budget.MaxTurns == 0 {
		final.Budget.MaxTurns = maxTurns
	}
	writeEnvelope(cmd, core.Effect{Kind: core.EFFECT_DONE})
	_ = final
	return nil
}