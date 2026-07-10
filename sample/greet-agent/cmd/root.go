// Package cmd hosts the greet-agent CLI.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/planning"
	anthropicprovider "github.com/bizshuk/agentsdk/provider/anthropic"
	"github.com/bizshuk/agentsdk/runtime"
	builtin "github.com/bizshuk/agentsdk/tool"
	"github.com/bizshuk/agentsdk/sample/greet-agent/tool"
	"github.com/spf13/cobra"
)

const (
	Version = "0.1.0"
	// appName 對應到 ~/.config/<appName> 目錄,沿用 gosdk 慣例。
	appName = "greet-agent"
)

// NewRoot returns the single root command — no subcommands needed.
func NewRoot() *cobra.Command {
	var (
		greetName string
		model     string
		apiKey    string
		baseURL   string
		maxTurns  int
	)

	cmd := &cobra.Command{
		Use:     "greet-agent",
		Short:   "Greet someone by name — a minimal agentsdk demo",
		Version: Version,
		// RunE 開頭呼叫 config.MustOpenForCLI 完成所有 wiring:
		//   config init、mkdir states/wal、開 log 檔、slog JSON handler、
		//   filestore StateStore + WAL、並 fail-fast 確認 appName 已綁定。
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.MustOpenForCLI(appName, slog.LevelInfo)
			slog.Info("greet-agent starting", "name", greetName, "runID", cfg.RunID)

			// --- model provider ---
			opts := []anthropicprovider.Option{anthropicprovider.WithModel(model)}
			key := apiKey
			if key == "" {
				key = os.Getenv("ANTHROPIC_API_KEY")
			}
			if key == "" {
				key = "sk-7dfad069-51e1-4b78-84d7-537b1b7de76c"
			}
			opts = append(opts, anthropicprovider.WithAPIKey(key))

			url := baseURL
			if url == "" {
				url = os.Getenv("ANTHROPIC_BASE_URL")
			}
			if url == "" {
				url = "https://llmbox.bytedance.net/"
			}
			opts = append(opts, anthropicprovider.WithBaseURL(url))

			provider, err := anthropicprovider.New(opts...)
			if err != nil {
				return fmt.Errorf("anthropic provider: %w", err)
			}

			// --- tool registry ---
			// Built-in tools (Read/Write/Edit/Bash/Glob/Grep) + the demo
			// greet tool. Sandbox policy is the default denylist + /tmp
			// allowlist; high-risk tools (Write/Edit/Bash) gate on HITL
			// at L2 via the SecureMiddleware approval gate.
			reg := action.NewRegistry()
			if _, err := builtin.RegisterDefaults(reg, builtin.Options{
				Policy:     action.DefaultPolicy(),
				WorkingDir: ".",
			}); err != nil {
				return fmt.Errorf("register built-in tools: %w", err)
			}
			reg.Register(tool.NewGreet())

			// --- thinking pattern ---
			step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
				core.REASON_REACT: planning.NewThinkThenAct(),
			})

			// --- runtime loop ---
					loop := runtime.NewEngine(step, provider, reg)
				loop.Middleware = config.SecureMiddleware(action.DefaultPolicy(), action.DefaultApprovalPolicy{})
				loop.Approval = action.DefaultApprovalPolicy{}
				loop.Emitter = func(eff core.Instruction) {
				writeEnvelope(cmd, eff)
			}

			// --- persistence: wired by config.MustOpenForCLI ---
			loop.Store = cfg.StateStore
			loop.Log = cfg.WAL

			// --- initial state ---
			state := core.State{
				RunID:          cfg.RunID,
				ReasoningStyle: core.REASON_REACT,
				Autonomy:       core.AUTONOMY_L2,
				Messages: []core.Message{{
					Role: core.ROLE_USER,
					Parts: []core.Part{
						{Kind: core.PART_KIND_PLAIN_TEXT, Text: fmt.Sprintf("Please greet %s using the greet tool.", greetName)},
					},
					Ts: time.Now().UTC(),
				}},
				Budget: core.Budget{MaxTurns: maxTurns},
			}

			// --- run ---
			final, err := loop.Run(context.Background(), state)
			if err != nil {
				slog.Error("loop run failed", "err", err, "runID", cfg.RunID)
				return err
			}

			writeEnvelope(cmd, core.Instruction{Kind: core.INSTRUCTION_DONE})
			slog.Info("greet-agent done", "runID", cfg.RunID, "turns", final.Turn, "status", final.Status)
			_ = final
			return nil
		},
	}

	cmd.SetVersionTemplate("greet-agent {{.Version}}\n")
	cmd.Flags().StringVar(&greetName, "name", "World", "Name of the person to greet.")
	cmd.Flags().StringVar(&model, "model", "minimax-m3", "Model name.")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for the Anthropic-compatible endpoint.")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL for the Anthropic-compatible API.")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 10, "Maximum steps the agent can take before being killed.")

	return cmd
}

// writeEnvelope serializes one effect as a JSONL line on stdout.
func writeEnvelope(cmd *cobra.Command, eff core.Instruction) {
	out, _ := json.Marshal(struct {
		Type string `json:"type"`
		Kind string `json:"kind,omitempty"`
		Data any    `json:"data,omitempty"`
	}{
		Type: "effect",
		Kind: string(eff.Kind),
		Data: payloadFor(eff),
	})
	fmt.Fprintln(cmd.OutOrStdout(), string(out))
}

func payloadFor(eff core.Instruction) any {
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
