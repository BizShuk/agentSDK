package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware/preset"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/internal/fake"
	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/tool"
	builtin "github.com/bizshuk/agentsdk/tool"
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

	// Listener — emits a single Percept when the log file is tailed.
	listener, err := domain.NewLogFileListener(f.fixture)
	if err != nil {
		return err
	}

	// Provider: --fake (offline scripted) or --provider (real).
	provider, isFake, err := resolveProvider(cmd)
	if err != nil {
		return err
	}
	if isFake {
		provider = fake.NewScriptedProvider()
	}
	maxTurns, _ := cmd.Root().PersistentFlags().GetInt("max-turns")

	// Tool registry.
	// 內建工具 (Read/Write/Edit/Bash/Glob/Grep) 透過 RegisterDefaults 一次註冊,
	// 與既有 read_log_tail / notify 並存。Write/Edit/Bash 需要 non-nil Policy。
	reg := action.NewRegistry()
	if err := builtin.RegisterDefaults(reg, builtin.Options{
		Policy:     action.DefaultPolicy(),
		WorkingDir: ".",
	}); err != nil {
		return fmt.Errorf("register built-in tools: %w", err)
	}
	rdt := tool.NewReadLogTail(listener)
	rdt.Register(reg)
	nt := tool.NewNotify(cmd.OutOrStdout())
	nt.Register(reg)

	// Step: ReAct is the simplest pattern; matches the sample's flow.
	step := agent.ReActStep()

	engine := agent.NewEngine(step, provider, reg)
	// M4: wire the full security chain (sandbox + approval + spotlight +
	// sanitizer). approval uses the L0-L4 enterprise grid so propose_fix
	// (HIGH risk) triggers a REQUEST_APPROVAL at L2. sandbox policy must
	// be non-nil so the built-in Write/Edit/Bash tools can path-check
	// their args — matches the Policy passed to RegisterDefaults above.
	engine.Middleware = preset.Secure(action.DefaultPolicy(), action.DefaultApprovalPolicy{})
	engine.Emitter = func(eff core.Instruction) {
		writeEnvelope(cmd, eff)
	}
	engine.Approval = action.DefaultApprovalPolicy{}

	// Optional persistence — if --data-dir or env, wire up store+WAL.
	dataDir := f.dataDir
	if dataDir == "" {
		dataDir = os.Getenv("LOGDOCTOR_DATA")
	}
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return fmt.Errorf("mkdir data dir: %w", err)
	}
	host, err := agent.Open("logdoctor")
	if err != nil {
		return fmt.Errorf("open host: %w", err)
	}
	if engine.Store == nil {
		engine.Store = host.StateStore
	}
	if engine.Log == nil {
		engine.Log = host.WAL
	}

	// Initial state.
	state := core.State{
		RunID:          fmt.Sprintf("run-%d", time.Now().UnixNano()),
		ReasoningStyle: core.REASON_REACT,
		Autonomy:       core.AUTONOMY_L2,
		Budget:         core.Budget{MaxTurns: maxTurns},
	}

	// Seed the percept via RunWithEvent so ReAct sees the user instruction.
	perceptCh := listener.Observations(context.Background())
	var first core.Observation
	select {
	case first = <-perceptCh:
	case <-time.After(2 * time.Second):
		return fmt.Errorf("listener produced no percept within 2s")
	}
	final, err := engine.RunWithEvent(context.Background(), state, core.Event{
		Kind:        core.EVENT_OBSERVATION,
		Observation: &first,
	})
	if err != nil {
		return err
	}
	writeEnvelope(cmd, core.Instruction{
		Kind: core.INSTRUCTION_DONE,
	})
	_ = final
	return nil
}

// writeEnvelope serializes one effect as a JSONL line on stdout.
// Downstream tooling (tailers, dashboards) consume this stream.
func writeEnvelope(cmd *cobra.Command, eff core.Instruction) {
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

func payloadFor(eff core.Instruction) interface{} {
	switch eff.Kind {
	case core.INSTRUCTION_CALL_TOOL:
		return eff.CallTool
	case core.INSTRUCTION_CALL_MODEL:
		return eff.CallModel
	case core.INSTRUCTION_NOTIFY:
		return eff.Notify
	default:
		return nil
	}
}

// AllowAllPolicy is the M1 default — every tool call runs.
type AllowAllPolicy struct{}

func (AllowAllPolicy) Decide(_ struct{}, _ core.AutonomyLevel, _ core.CallToolInstruction, _ core.ToolSpec) core.ApprovalAction {
	return core.APPROVAL_ACTION_ALLOW
}

// init no-op — verb registration is centralized in main.go so this file
// can be tested without Execute() side effects.
func init() {}
