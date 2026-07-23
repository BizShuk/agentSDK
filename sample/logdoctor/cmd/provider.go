package cmd

import (
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	anthropicprovider "github.com/bizshuk/agentsdk/provider/anthropic"
	googleprovider "github.com/bizshuk/agentsdk/provider/google"
	ollamaprovider "github.com/bizshuk/agentsdk/provider/ollama"
	"github.com/spf13/cobra"
)

// providerName is the value of --provider (empty when --fake is set).
type providerName string

const (
	providerAnthropic providerName = "anthropic"
	providerOllama    providerName = "ollama"
	providerGoogle    providerName = "google"
)

// selectProvider builds a real ModelProvider from --provider. --fake is
// handled by the caller (run/resume) before reaching here. Returns an
// error when the flag is empty or unknown so the CLI surfaces a clear
// message instead of a nil provider panic.
//
// Credentials default to the provider's canonical env var
// (ANTHROPIC_API_KEY / OPENAI_API_KEY / GOOGLE_API_KEY), matching the
// plan's "CI uses if [ -n "$KEY" ]" convention.
func selectProvider(name providerName) (core.Provider, error) {
	switch name {
	case providerAnthropic:
		p, err := anthropicprovider.New()
		if err != nil {
			return nil, fmt.Errorf("anthropic provider: %w", err)
		}
		return p, nil
	case providerOllama:
		p, err := ollamaprovider.New()
		if err != nil {
			return nil, fmt.Errorf("ollama provider: %w", err)
		}
		return p, nil
	case providerGoogle:
		p, err := googleprovider.New()
		if err != nil {
			return nil, fmt.Errorf("google provider: %w", err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (want anthropic|ollama|google)", name)
	}
}

// resolveProvider picks the provider per the root flags: --fake wins
// (offline), else --provider. Exactly one must be set.
func resolveProvider(cmd *cobra.Command) (core.Provider, bool /*isFake*/, error) {
	fakeMode, _ := cmd.Root().PersistentFlags().GetBool("fake")
	providerFlag, _ := cmd.Root().PersistentFlags().GetString("provider")

	if fakeMode && providerFlag != "" {
		return nil, false, fmt.Errorf("--fake and --provider are mutually exclusive")
	}
	if fakeMode {
		return nil, true, nil
	}
	if providerFlag == "" {
		return nil, false, fmt.Errorf("either --fake or --provider=<anthropic|ollama|google> is required")
	}
	p, err := selectProvider(providerName(providerFlag))
	if err != nil {
		return nil, false, err
	}
	return p, false, nil
}
