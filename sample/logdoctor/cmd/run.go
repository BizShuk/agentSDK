package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

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

// runFlags holds CLI flags for the `run` verb.
type runFlags struct {
	once    bool
	fixture string
	dataDir string
}

// RegisterRun attaches the run subcommand to root. Call from main.go.
func RegisterRun(root *cobra.Command) {
	f := &runFlags{}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a single log-doctor pass against a log file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecute(cmd, f)
		},
	}
	cmd.Flags().BoolVar(&f.once, "once", false,
		"Read existing log lines once and exit (no watcher loop).")
	cmd.Flags().StringVar(&f.fixture, "fixture", "",
		"Path to a log file. Required when --once is set.")
	cmd.Flags().StringVar(&f.dataDir, "data-dir", "",
		"Directory for StateStore + WAL (default: $LOGDOCTOR_DATA or ./data).")
	root.AddCommand(cmd)
}

func runExecute(cmd *cobra.Command, f *runFlags) error {
	if !f.once {
		return fmt.Errorf("M1 only supports --once; watcher (--watch) is M2")
	}
	if f.fixture == "" {
		return fmt.Errorf("--fixture is required with --once")
	}
	if _, err := os.Stat(f.fixture); err != nil {
		return fmt.Errorf("fixture not found: %s", f.fixture)
	}

	// Capture options from the root persistent flags.
	fakeMode, _ := cmd.Root().PersistentFlags().GetBool("fake")
	maxTurns, _ := cmd.Root().PersistentFlags().GetInt("max-turns")
	if !fakeMode {
		return fmt.Errorf("M1 only supports --fake; real providers are M4")
	}

	// Listener — emits a single Percept when the log file is tailed.
	listener, err := domain.NewLogFileListener(f.fixture)
	if err != nil {
		return err
	}

	// Provider: scripted responses for the e2e demo.
	provider := fake.NewScriptedProvider()

	// Tool registry.
	reg := action.NewRegistry()
	rdt := tool.NewReadLogTail(listener)
	reg.Register(rdt)
	nt := tool.NewNotify(cmd.OutOrStdout())
	reg.Register(nt)

	// Step: ReAct is the simplest pattern; matches the sample's flow.
	step := core.NewStep(map[core.ThinkingKind]core.ThinkingPattern{
		core.THINK_REACT: planning.NewReAct(),
	})

	loop := runtime.NewLoop(step, provider, reg)
	loop.Emitter = func(eff core.Effect) {
		writeEnvelope(cmd, eff)
	}
	loop.Approval = allowAllApproval{}

	// Optional persistence — if --data-dir or env, wire up store+WAL.
	dataDir := f.dataDir
	if dataDir == "" {
		dataDir = os.Getenv("LOGDOCTOR_DATA")
	}
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0o750); err == nil {
		if store, err := filestore.NewFileStateStore(dataDir); err == nil {
			loop.Store = store
		}
		if wal, err := filestore.NewFileWAL(dataDir); err == nil {
			loop.WAL = wal
		}
	}

	// Initial state.
	state := core.State{
		RunID:        fmt.Sprintf("run-%d", time.Now().UnixNano()),
		ThinkingKind: core.THINK_REACT,
		Autonomy:     core.AUTONOMY_L2,
		Budget:       core.Budget{MaxTurns: maxTurns},
	}

	// Seed the percept via RunWithInput so ReAct sees the user instruction.
	perceptCh := listener.Percepts(context.Background())
	var first core.Percept
	select {
	case first = <-perceptCh:
	case <-time.After(2 * time.Second):
		return fmt.Errorf("listener produced no percept within 2s")
	}
	final, err := loop.RunWithInput(context.Background(), state, core.Input{
		Kind:    core.INPUT_KIND_PERCEPT,
		Percept: &first,
	})
	if err != nil {
		return err
	}
	writeEnvelope(cmd, core.Effect{
		Kind: core.EFFECT_DONE,
	})
	_ = final
	return nil
}

// writeEnvelope serializes one effect as a JSONL line on stdout.
// Downstream tooling (tailers, dashboards) consume this stream.
func writeEnvelope(cmd *cobra.Command, eff core.Effect) {
	out, _ := json.Marshal(struct {
		Type  string      `json:"type"`
		Kind  string      `json:"kind,omitempty"`
		Level string      `json:"level,omitempty"`
		Data  interface{} `json:"data,omitempty"`
	}{
		Type: "effect",
		Kind: string(eff.Kind),
		Data: payloadFor(eff),
	})
	fmt.Fprintln(cmd.OutOrStdout(), string(out))
}

func payloadFor(eff core.Effect) interface{} {
	switch eff.Kind {
	case core.EFFECT_CALL_TOOL:
		return eff.CallTool
	case core.EFFECT_CALL_MODEL:
		return eff.CallModel
	case core.EFFECT_NOTIFY:
		return eff.Notify
	default:
		return nil
	}
}

// allowAllApproval is the M1 default — every tool call runs.
type allowAllApproval struct{}

func (allowAllApproval) Decide(_ struct{}, _ core.AutonomyLevel, _ core.CallToolEffect, _ core.ToolSchema) core.ApprovalAction {
	return core.APPROVAL_ACTION_ALLOW
}

// init no-op — verb registration is centralized in main.go so this file
// can be tested without Execute() side effects.
func init() {}