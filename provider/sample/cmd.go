package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/sample/config"
	"github.com/spf13/cobra"
)

// DEFAULT_TIMEOUT bounds one sample request without constraining provider APIs.
const DEFAULT_TIMEOUT = 2 * time.Minute

func newRootCmd() *cobra.Command {
	cfg := config.Config{
		Provider:     provider.DEFAULT_NAME,
		Type:         config.API_TYPE_CHAT,
		Auth:         config.AUTH_MODE_AUTO,
		OutputFormat: "url",
		SampleRate:   44100,
		Bitrate:      256000,
		AudioFormat:  "mp3",
	}
	var list bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "provider-sample [flags] <prompt>",
		Short: "Sample CLI demonstrating provider capabilities and access modes",
		Long: strings.TrimSpace(`
provider-sample is a reference CLI demonstrating provider integration,
capability discovery, and auth mode resolution.

Examples:
  go run ./provider/sample --list
  go run ./provider/sample --provider ollama --type chat "hello"
  go run ./provider/sample --provider google --type image "a paper fox"
  go run ./provider/sample --provider minimax --type music \
    --model music-cover --audio-url https://example.com/song.mp3 \
    "Jazz, smooth, late night lounge, saxophone"
`),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if list {
				return writeProviderMatrix(cmd.OutOrStdout())
			}
			if timeout <= 0 {
				return fmt.Errorf("timeout must be greater than zero")
			}
			cfg.Prompt = strings.TrimSpace(strings.Join(args, " "))

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			requestCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return run(requestCtx, cfg, cmd.OutOrStdout(), os.Getenv)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&cfg.Provider, "provider", cfg.Provider,
		"provider name; use --list to see linked providers")
	flags.Var(&cfg.Type, "type", "API type: chat | image | music | audio")
	flags.Var(&cfg.Auth, "auth", "credential source: auto | api_key | oauth")
	flags.StringVar(&cfg.Model, "model", "", "model id; empty uses the provider default")
	flags.StringVar(&cfg.BaseURL, "base-url", "", "provider base URL override")
	flags.StringVar(&cfg.Lyrics, "lyrics", "", "music lyrics; use newline characters between lines")
	flags.StringVar(&cfg.AudioURL, "audio-url", "", "reference audio URL for music cover generation")
	flags.StringVar(&cfg.OutputFormat, "output-format", cfg.OutputFormat,
		"music response encoding: url | hex")
	flags.IntVar(&cfg.SampleRate, "sample-rate", cfg.SampleRate, "music sample rate in Hz")
	flags.IntVar(&cfg.Bitrate, "bitrate", cfg.Bitrate, "music bitrate in bits per second")
	flags.StringVar(&cfg.AudioFormat, "audio-format", cfg.AudioFormat,
		"generated music format: mp3 | wav | pcm")
	flags.BoolVar(&cfg.JSON, "json", false, "print the complete response as JSON")
	flags.BoolVar(&list, "list", false, "list provider, API type, and auth support")
	flags.DurationVar(&timeout, "timeout", DEFAULT_TIMEOUT, "request timeout")

	return cmd
}

var rootCmd = newRootCmd()

func execute(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
) error {
	cmd := newRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd.ExecuteContext(ctx)
}
