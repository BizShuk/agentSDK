// Package cmd hosts the code-agent CLI: flag surface, mode dispatch
// (interactive / print / stream-json), and session flags. All harness
// wiring lives in compose.go — this file only routes.
package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/wire"
	"github.com/spf13/cobra"
)

const (
	Version = "0.1.0"
	// appName 對應 ~/.config/<appName>，沿 gosdk 慣例。
	appName = "code-agent"
)

// NewRoot builds the single root command.
func NewRoot() *cobra.Command {
	var (
		prompt       string
		jsonMode     bool
		fake         bool
		providerName string
		model        string
		apiKey       string
		baseURL      string
		maxTurns     int
		permMode     string
		contFlag     bool
		resumeID     string
		forkID       string
		listSessions bool
	)

	cmd := &cobra.Command{
		Use:     "code-agent",
		Short:   "Full-harness agentsdk demo: tui + wire + hooks/permission/session/skill/subagent",
		Version: Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.MustOpenForCLI(appName, slog.LevelInfo)
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			parts, err := compose(composeOptions{
				cfg: cfg, fake: fake, provider: providerName, model: model, apiKey: apiKey,
				baseURL: baseURL, maxTurns: maxTurns, permissionMode: permMode,
			})
			if err != nil {
				return err
			}

			if listSessions {
				return printSessions(cmd.OutOrStdout(), parts)
			}

			state, err := parts.openState(ctx, sessionRequest{
				continueLatest: contFlag, resumeID: resumeID, forkID: forkID,
			})
			if err != nil {
				return err
			}

			// Headless when -p given or stdin is piped; interactive otherwise.
			task := strings.TrimSpace(prompt)
			if task == "" && stdinIsPipe() {
				raw, _ := io.ReadAll(os.Stdin)
				task = strings.TrimSpace(string(raw))
			}
			if task != "" {
				return runOnce(ctx, cmd.OutOrStdout(), parts, state, task, jsonMode)
			}
			if jsonMode {
				return fmt.Errorf("--json needs -p or piped stdin (interactive json is not supported)")
			}
			return runInteractive(ctx, parts, state)
		},
	}

	cmd.SetVersionTemplate("code-agent {{.Version}}\n")
	f := cmd.Flags()
	f.StringVarP(&prompt, "print", "p", "", "One-shot prompt: run headless and print the final answer.")
	f.BoolVar(&jsonMode, "json", false, "Emit stream-json envelopes (wire) instead of text; implies headless.")
	f.BoolVar(&fake, "fake", false, "Use the scripted fake provider (no network, echoes after the script).")
	f.StringVar(&providerName, "provider", "minimax", "LLM provider: minimax (reads MINIMAX_API_KEY) | anthropic (reads ANTHROPIC_API_KEY).")
	f.StringVar(&model, "model", "", "Model id; empty uses the provider's flagship default (MiniMax-M3 / claude-3-5-sonnet-latest).")
	f.StringVar(&apiKey, "api-key", "", "API key override (else the provider's own env var).")
	f.StringVar(&baseURL, "base-url", "", "Base URL override (else the provider's own default / env var).")
	f.IntVar(&maxTurns, "max-turns", 20, "Maximum loop turns per run.")
	f.StringVar(&permMode, "permission-mode", "default", "Permission mode: default | acceptEdits | plan | bypassPermissions.")
	f.BoolVarP(&contFlag, "continue", "c", false, "Continue the latest session for this directory.")
	f.StringVarP(&resumeID, "resume", "r", "", "Resume a session by ID.")
	f.StringVar(&forkID, "fork", "", "Fork a session by ID and continue on the copy.")
	f.BoolVar(&listSessions, "sessions", false, "List sessions for this directory and exit.")
	return cmd
}

// runOnce is the headless surface: print mode writes progress to stderr and
// the final text to stdout; json mode writes wire envelopes to stdout.
func runOnce(ctx context.Context, out io.Writer, parts *agentParts, state core.State, task string, jsonMode bool) error {
	enc := wire.NewEncoder(out)
	if jsonMode {
		parts.Engine.Sink = wire.NewSink(out)
	} else {
		parts.Engine.Sink = progressSink{w: os.Stderr}
	}

	state.Messages = append(state.Messages, userMessage(task))
	final, err := parts.Engine.Run(ctx, state)
	if err != nil {
		if jsonMode {
			_ = enc.Encode(wire.Envelope{Type: wire.TYPE_ERROR, Error: &wire.ErrorPayload{Message: err.Error()}, Ts: time.Now().UTC()})
		}
		return err
	}
	if jsonMode {
		return enc.Encode(wire.Envelope{Type: wire.TYPE_RESULT, Result: &wire.Result{
			RunID: final.RunID, Status: final.Status, Text: lastAssistantText(final),
		}, Ts: time.Now().UTC()})
	}
	fmt.Fprintln(out, lastAssistantText(final))
	return nil
}

// progressSink prints tool progress lines to stderr in print mode; the
// final answer goes to stdout separately, so message events are skipped.
type progressSink struct{ w io.Writer }

func (p progressSink) OnStreamEvent(ev core.StreamEvent) {
	switch ev.Kind {
	case core.STREAM_TOOL_START, core.STREAM_TOOL_RESULT:
		if line := wire.FormatStream(ev); line != "" {
			fmt.Fprintln(p.w, line)
		}
	}
}

func printSessions(out io.Writer, parts *agentParts) error {
	metas, err := parts.Sessions.List(parts.Cwd)
	if err != nil {
		return err
	}
	if len(metas) == 0 {
		fmt.Fprintln(out, "（此目錄尚無 session）")
		return nil
	}
	for _, m := range metas {
		parent := ""
		if m.Parent != "" {
			parent = "  ← fork of " + m.Parent
		}
		fmt.Fprintf(out, "%s  %s  %s%s\n", m.ID, m.CreatedAt.Local().Format("2006-01-02 15:04"), m.Title, parent)
	}
	return nil
}

func stdinIsPipe() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}
