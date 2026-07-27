package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/bizshuk/agentsdk/agent"
	sdkcore "github.com/bizshuk/agentsdk/core"
	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/spf13/cobra"
)

type watchFlags struct {
	interval time.Duration
	maxRuns  int
}

func newWatchCommand() *cobra.Command {
	var flags watchFlags
	command := &cobra.Command{
		Use:   "watch",
		Short: "Continuously analyze new ~/.config application logs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return watchExecute(cmd, flags)
		},
	}
	command.Flags().DurationVar(&flags.interval, "interval", time.Minute,
		"Polling interval.")
	command.Flags().IntVar(&flags.maxRuns, "max-runs", 0,
		"Stop after N polling cycles (0 = forever).")
	return command
}

func watchExecute(cmd *cobra.Command, f watchFlags) error {
	if f.interval <= 0 {
		return fmt.Errorf("--interval must be greater than zero")
	}
	if f.maxRuns < 0 {
		return fmt.Errorf("--max-runs must not be negative")
	}

	host, err := agent.Open("logdoctor")
	if err != nil {
		return fmt.Errorf("open logdoctor host: %w", err)
	}
	reader, err := domain.NewChunkReader(
		filepath.Dir(filepath.Dir(host.DataDir)),
		filepath.Join(host.DataDir, "log-cursors.json"),
	)
	if err != nil {
		return fmt.Errorf("create log reader: %w", err)
	}
	model, err := newMiniMaxProvider()
	if err != nil {
		return err
	}

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return watchLoop(
		ctx,
		reader,
		model,
		cmd.OutOrStdout(),
		cmd.ErrOrStderr(),
		ticker.C,
		f.maxRuns,
	)
}

func watchLoop(
	ctx context.Context,
	reader *domain.ChunkReader,
	model sdkcore.Provider,
	stdout io.Writer,
	stderr io.Writer,
	ticks <-chan time.Time,
	maxRuns int,
) error {
	if reader == nil {
		return fmt.Errorf("watch loop: reader must not be nil")
	}
	if model == nil {
		return fmt.Errorf("watch loop: provider must not be nil")
	}
	if stdout == nil || stderr == nil {
		return fmt.Errorf("watch loop: output writers must not be nil")
	}
	if ticks == nil {
		return fmt.Errorf("watch loop: tick channel must not be nil")
	}
	if maxRuns < 0 {
		return fmt.Errorf("watch loop: max runs must not be negative")
	}

	for cycle := 1; ; cycle++ {
		cycleErr := runWatchCycle(ctx, reader, model, stdout, stderr)
		if errors.Is(cycleErr, context.Canceled) ||
			errors.Is(cycleErr, context.DeadlineExceeded) {
			return cycleErr
		}
		if cycleErr != nil {
			if _, err := fmt.Fprintf(
				stderr,
				"watch cycle %d failed: %v\n",
				cycle,
				cycleErr,
			); err != nil {
				return fmt.Errorf(
					"watch loop: %w",
					errors.Join(cycleErr, fmt.Errorf("write cycle error: %w", err)),
				)
			}
		}
		if maxRuns > 0 && cycle >= maxRuns {
			return cycleErr
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticks:
		}
	}
}

func runWatchCycle(
	ctx context.Context,
	reader *domain.ChunkReader,
	model sdkcore.Provider,
	stdout io.Writer,
	stderr io.Writer,
) error {
	chunk, warnings, err := reader.Next(ctx)
	if err != nil {
		return fmt.Errorf("read next log chunk: %w", err)
	}
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(stderr, "log scan warning: %v\n", warning); err != nil {
			return fmt.Errorf("write log scan warning: %w", err)
		}
	}

	if chunk.Bytes == 0 {
		if err := reader.Commit(ctx, chunk); err != nil {
			return fmt.Errorf("commit idle log cursor: %w", err)
		}
		return nil
	}

	diagnosis, err := analyzeChunk(ctx, chunk, model, stderr)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, diagnosis); err != nil {
		return fmt.Errorf("write log diagnosis: %w", err)
	}
	if err := reader.Commit(ctx, chunk); err != nil {
		return fmt.Errorf("commit analyzed log cursor: %w", err)
	}
	return nil
}
