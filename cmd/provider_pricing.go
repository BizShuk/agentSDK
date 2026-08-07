package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/bizshuk/agentsdk/provider/pricing"
	"github.com/spf13/cobra"
)

const DEFAULT_PRICING_TIMEOUT = 30 * time.Second

var (
	ProviderPricingWrite bool

	providerPricingURL          = pricing.OPENROUTER_MODELS_URL
	providerPricingHTTPClient   = &http.Client{Timeout: DEFAULT_PRICING_TIMEOUT}
	providerPricingSnapshotPath = pricing.DefaultSnapshotPath()
	providerPricingNow          = time.Now
)

// ProviderPricingCmd groups provider pricing maintenance actions.
var ProviderPricingCmd = &cobra.Command{
	Use:   "pricing",
	Short: "Inspect and refresh the versioned provider pricing snapshot",
}

// ProviderPricingRefreshCmd previews the latest OpenRouter pricing diff and
// updates the checked-in snapshot only with --write.
var ProviderPricingRefreshCmd = &cobra.Command{
	Use:          "refresh",
	Short:        "Fetch and validate the latest OpenRouter model pricing",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		next, err := pricing.Fetch(
			cmd.Context(),
			providerPricingHTTPClient,
			providerPricingURL,
			providerPricingNow(),
		)
		if err != nil {
			return err
		}
		current, err := pricing.ReadSnapshot(providerPricingSnapshotPath)
		if err != nil {
			return err
		}
		diff := pricing.Compare(current, next)
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "pricing_as_of=%s models=%d added=%d changed=%d removed=%d\n",
			next.PricingAsOf, len(next.Models), len(diff.Added), len(diff.Changed), len(diff.Removed))
		for _, id := range diff.Added {
			fmt.Fprintln(out, "+ "+id)
		}
		for _, id := range diff.Changed {
			fmt.Fprintln(out, "~ "+id)
		}
		for _, id := range diff.Removed {
			fmt.Fprintln(out, "- "+id)
		}
		if !ProviderPricingWrite {
			fmt.Fprintln(out, "preview only; pass --write to update the versioned snapshot")
			return nil
		}
		if err := pricing.WriteSnapshot(providerPricingSnapshotPath, next); err != nil {
			return err
		}
		fmt.Fprintln(out, "updated "+providerPricingSnapshotPath)
		return nil
	},
}

func init() {
	ProviderPricingRefreshCmd.Flags().BoolVar(
		&ProviderPricingWrite,
		"write",
		false,
		"Atomically update the checked-in pricing snapshot after validation.",
	)
	ProviderPricingCmd.AddCommand(ProviderPricingRefreshCmd)
	ProviderCmd.AddCommand(ProviderPricingCmd)
}
