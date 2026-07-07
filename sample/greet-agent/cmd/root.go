// Package cmd hosts the greet-agent CLI.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/memory/filestore"
	"github.com/bizshuk/agentsdk/planning"
	anthropicprovider "github.com/bizshuk/agentsdk/provider/anthropic"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/sample/greet-agent/tool"
	"github.com/bizshuk/gosdk/config"
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
		// 提早跑 PersistentPreRunE,讓子命令的 RunE 拿到的 APP_*_DIR 已是就緒狀態。
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			// 啟用 gosdk/config — 決定 APP_CONFIG_DIR = ~/.config/greet-agent。
			// 必須在讀取 GetAppConfigDir / GetAppLogDir / GetAppDataDir 之前呼叫。
			config.Default(config.WithAppName(appName))
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// 路徑完全走 gosdk 慣例,沒有自訂旗標。
			dataDir := config.GetAppDataDir() // ~/.config/greet-agent/data
			logDir := config.GetAppLogDir()   // ~/.config/greet-agent/log
			if dataDir == "" || logDir == "" {
				return fmt.Errorf("gosdk/config 未初始化 APP_*_DIR (WithAppName 是否呼叫?)")
			}
			statesDir := filepath.Join(dataDir, "states")
			walDir := filepath.Join(dataDir, "wal")
			if err := os.MkdirAll(statesDir, 0o750); err != nil {
				return fmt.Errorf("mkdir states: %w", err)
			}
			if err := os.MkdirAll(walDir, 0o750); err != nil {
				return fmt.Errorf("mkdir wal: %w", err)
			}

			// runID = UnixNano,直接當 states/wal/log 的檔名。
			// appName 已隱含在 ~/.config/greet-agent/ 這層,不需在檔名重複。
			runID := fmt.Sprintf("%d", time.Now().UnixNano())

			// 把 slog 接到 <APP_LOG_DIR>/<runID>.log,後續任何 slog.* 都會落地。
			if err := initFileLogger(logDir, runID); err != nil {
				return err
			}
			slog.Info("greet-agent starting", "runID", runID, "dataDir", dataDir, "statesDir", statesDir, "walDir", walDir)

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
			reg := action.NewRegistry()
			reg.Register(tool.NewGreet())

			// --- thinking pattern ---
			step := core.NewStep(map[core.ThinkingKind]core.ThinkingPattern{
				core.THINK_REACT: planning.NewReAct(),
			})

			// --- runtime loop ---
			loop := runtime.NewLoop(step, provider, reg)
			loop.Emitter = func(eff core.Effect) {
				writeEnvelope(cmd, eff)
			}

			// --- persistence: directly under <APP_DATA_DIR>/{states,wal} ---
			// filestore.NewFileStateStore/NewFileWAL 會在傳入的 baseDir 下
			// 再建 states/ 與 wal/ 目錄,所以 baseDir = <APP_DATA_DIR> 就剛好。
			if store, err := filestore.NewFileStateStore(dataDir); err == nil {
				loop.Store = store
			} else {
				return fmt.Errorf("state store: %w", err)
			}
			if wal, err := filestore.NewFileWAL(dataDir); err == nil {
				loop.WAL = wal
			} else {
				return fmt.Errorf("wal: %w", err)
			}

			// --- initial state ---
			state := core.State{
				RunID:        runID,
				ThinkingKind: core.THINK_REACT,
				Autonomy:     core.AUTONOMY_L2,
				Messages: []core.Message{{
					Role: core.ROLE_USER,
					Chunks: []core.Chunk{
						{Kind: core.CHUNK_KIND_TEXT, Text: fmt.Sprintf("Please greet %s using the greet tool.", greetName)},
					},
					Ts: time.Now().UTC(),
				}},
				Budget: core.Budget{MaxTurns: maxTurns},
			}

			// --- run ---
			final, err := loop.Run(context.Background(), state)
			if err != nil {
				slog.Error("loop run failed", "err", err, "runID", runID)
				return err
			}

			writeEnvelope(cmd, core.Effect{Kind: core.EFFECT_DONE})
			slog.Info("greet-agent done", "runID", runID, "turns", final.Turn, "status", final.Status)
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

// initFileLogger 把 slog default 換成 <logDir>/<runID>.log 的 JSON handler。
// 等級透過既有 LOG_LEVEL env 決定 (gosdk/log 會讀),預設 info。
func initFileLogger(logDir, runID string) error {
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return fmt.Errorf("mkdir log dir: %w", err)
	}
	logFile := filepath.Join(logDir, runID+".log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	// 等級同 gosdk/log 的預設行為:讀 LOG_LEVEL,fallback info。
	level := parseLogLevel(os.Getenv("LOG_LEVEL"))
	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h).With(slog.String("runID", runID)))
	return nil
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "warn", "warning", "WARN", "WARNING":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// writeEnvelope serializes one effect as a JSONL line on stdout.
func writeEnvelope(cmd *cobra.Command, eff core.Effect) {
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

func payloadFor(eff core.Effect) any {
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
