// Command auth-cli logs in to each LLM provider and verifies the stored
// credentials against the real provider APIs, and exposes the LLM protocol
// proxy as a subcommand.
//
//	auth-cli login apikey --provider anthropic   # API key from env/flag
//	auth-cli login anthropic                     # OAuth2 + PKCE (claude.ai)
//	auth-cli login openai                        # OAuth2 + PKCE (auth.openai.com)
//	auth-cli login vertex --sa-file sa.json      # Google service account
//	auth-cli list
//	auth-cli verify --all
//	auth-cli proxy                               # LLM protocol proxy server
package main

import (
	"fmt"
	"os"
	"strings"

	authcmd "github.com/bizshuk/auth/cmd"
	proxycmd "github.com/bizshuk/proxy/cmd"
	"github.com/spf13/cobra"
)

const (
	VERSION = "0.1.0"

	// APP_NAME 決定憑證落在 ~/.config/<APP_NAME>/data/auth/,沿用 gosdk 慣例。
	APP_NAME = "agentsdk"
)

func main() {
	root := &cobra.Command{
		Use:   "auth-cli",
		Short: "Log in to LLM providers and verify the stored credentials",
		Long: strings.TrimSpace(`
auth-cli exercises every authentication method the agentsdk auth package
implements, and stores each credential as a 0600 JSON file.

  PROVIDER   DEFAULT KIND     ALSO SUPPORTS
  anthropic  api_key          oauth (claude.ai, PKCE)
  openai     api_key          oauth (auth.openai.com, PKCE)
  google     api_key          -
  vertex     service_account  -

Every login verifies the credential against the real provider before saving it,
so a stored credential is one that was proven to work.

The "proxy" subcommand starts the LLM protocol proxy server that bridges
between Anthropic Messages, OpenAI Chat Completions, and OpenAI Responses
wire formats.`),
		Version: VERSION,
		// main() 負責印錯誤與設定 exit code,cobra 不要再印一次。
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Install 只在 root nil 或 appName 空白時失敗,兩者在此都不可能。
	_ = authcmd.Install(root, APP_NAME)
	root.AddCommand(proxycmd.NewCommand())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
