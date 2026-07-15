package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bizshuk/agentsdk/auth"
	"github.com/spf13/cobra"
)

// newUseCmd creates the "use" command to switch between multiple credentials.
func newUseCmd(root *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "use <credential-name>",
		Short: "Select the active credential for a provider",
		Long: `When you have multiple credentials for the same provider, this command allows
you to select which one the proxy server should use. The choice is saved to active.json
under the credential directory.`,
		Example: `  agentsdk use anthropic-dev@example.com_oauth
  agentsdk use openai-apikey`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			store, err := root.store()
			if err != nil {
				return err
			}

			// 1. Verify credential exists
			cred, err := store.Load(name)
			if err != nil {
				return fmt.Errorf("credential %q not found. Run 'agentsdk list' to see available credentials", name)
			}

			// 2. Read existing active.json map
			activePath := filepath.Join(store.Dir(), "active.json")
			active := make(map[string]string)
			if data, err := os.ReadFile(activePath); err == nil {
				_ = json.Unmarshal(data, &active)
			}

			// 3. Update the provider map entry
			active[cred.Provider] = name

			// 4. Write back to active.json atomically
			payload, err := json.MarshalIndent(active, "", "  ")
			if err != nil {
				return fmt.Errorf("serialize active config: %w", err)
			}

			tmp, err := os.CreateTemp(store.Dir(), ".tmp-active-*.json")
			if err != nil {
				return fmt.Errorf("create temp config file: %w", err)
			}
			tmpName := tmp.Name()
			defer func() { _ = os.Remove(tmpName) }()

			if err := tmp.Chmod(auth.AUTH_FILE_PERM); err != nil {
				_ = tmp.Close()
				return fmt.Errorf("chmod config file: %w", err)
			}

			if _, err := tmp.Write(payload); err != nil {
				_ = tmp.Close()
				return fmt.Errorf("write config file: %w", err)
			}
			_ = tmp.Close()

			if err := os.Rename(tmpName, activePath); err != nil {
				return fmt.Errorf("commit active config file: %w", err)
			}

			fmt.Printf("✅ Active credential for provider %q set to %q\n", cred.Provider, name)
			return nil
		},
	}
}
