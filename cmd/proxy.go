package cmd

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/proxy"
	"github.com/spf13/cobra"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Start the proxy server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProxy()
	},
}

func runProxy() error {
	cfg, err := config.LoadProxyConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return proxy.New(cfg).Run(ctx)
}
