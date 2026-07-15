// Package cmd hosts the command tree. Each authentication method
// gets its own `login` subcommand so it can be exercised independently, and
// `verify` proves a stored credential against the live provider.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/bizshuk/agentsdk/config"
	sdkauth "github.com/bizshuk/agentsdk/auth"
	"github.com/spf13/cobra"
)

const (
	VERSION = "0.1.0"

	// APP_NAME 決定憑證落在 ~/.config/<APP_NAME>/data/auth/,沿用 gosdk 慣例。
	APP_NAME = "agentsdk"
)

// rootFlags 是所有子指令共用的旗標。
type rootFlags struct {
	authDir   string
	noBrowser bool
}

// NewRoot 組出 auth-cli 的指令樹。
func NewRoot() *cobra.Command {
	flags := &rootFlags{}

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

	root.PersistentFlags().StringVar(&flags.authDir, "auth-dir", "",
		"credential directory (default ~/.config/"+APP_NAME+"/data/auth)")
	root.PersistentFlags().BoolVar(&flags.noBrowser, "no-browser", false,
		"headless OAuth: print the URL and read the pasted code from stdin")

	root.AddCommand(
		newLoginCmd(flags),
		newListCmd(flags),
		newVerifyCmd(flags),
		newRefreshCmd(flags),
		newLogoutCmd(flags),
		proxyCmd,
		videoCmd,
	)
	return root
}

// store 解析出憑證目錄。
func (f *rootFlags) store() (*sdkauth.FileStore, error) {
	return config.OpenAuthStore(APP_NAME, f.authDir)
}

// authOptions 依旗標組出 auth 套件的 options。--no-browser 時切到手動貼碼模式。
func (f *rootFlags) authOptions(extra ...sdkauth.Option) []sdkauth.Option {
	opts := extra
	if f.noBrowser {
		opts = append(opts, sdkauth.WithManualCode(promptForCode))
	}
	return opts
}

// promptForCode 印出授權 URL 並從 stdin 讀回 authorization code。
func promptForCode(authURL string) (string, error) {
	fmt.Println("Open this URL in a browser and authorize:")
	fmt.Println()
	fmt.Println("  " + authURL)
	fmt.Println()
	fmt.Print("Paste the authorization code here: ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// saveAndReport 存下憑證並印出結果 — 每個 login 子指令的收尾都一樣。
func saveAndReport(f *rootFlags, cred *sdkauth.Credential) error {
	store, err := f.store()
	if err != nil {
		return err
	}
	if err := store.Save(cred); err != nil {
		return err
	}

	fmt.Printf("✅ logged in: %s (%s / %s)\n", cred.Name(), cred.Provider, cred.Kind)
	if cred.Account != "" {
		fmt.Printf("   account:   %s\n", cred.Account)
	}
	if !cred.ExpiresAt.IsZero() {
		fmt.Printf("   expires:   %s\n", cred.ExpiresAt.Format("2006-01-02 15:04:05 MST"))
	}
	fmt.Printf("   saved to:  %s\n", store.Path(cred.Name()))
	return nil
}
