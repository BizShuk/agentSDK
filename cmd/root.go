// Package cmd is the aggregation shell of the auth-cli binary: it mounts the
// command sets carried by the auth, proxy, and utils/video modules onto one
// cobra root. Command logic lives with its module — nothing here but wiring.
package cmd

import (
	"strings"

	proxycmd "github.com/bizshuk/proxy/cmd"
	videocmd "github.com/bizshuk/agentsdk/utils/video/cmd"
	authcmd "github.com/bizshuk/auth/cmd"
	"github.com/spf13/cobra"
)

const (
	VERSION = "0.1.0"

	// APP_NAME 決定憑證落在 ~/.config/<APP_NAME>/data/auth/,沿用 gosdk 慣例。
	APP_NAME = "agentsdk"
)

// NewRoot 組出 auth-cli 的指令樹。
func NewRoot() *cobra.Command {
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
so a stored credential is one that was proven to work.`),
		Version: VERSION,
		// main() 負責印錯誤與設定 exit code,cobra 不要再印一次。
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Install 只在 root nil 或 appName 空白時失敗,兩者在此都不可能。
	_ = authcmd.Install(root, APP_NAME)
	root.AddCommand(
		proxycmd.NewCommand(),
		videocmd.NewCommand(),
	)
	return root
}
