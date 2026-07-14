// Command auth-cli logs in to each LLM provider and verifies the stored
// credentials against the real provider APIs.
//
//	auth-cli login apikey --provider anthropic   # API key from env/flag
//	auth-cli login anthropic                     # OAuth2 + PKCE (claude.ai)
//	auth-cli login openai                        # OAuth2 + PKCE (auth.openai.com)
//	auth-cli login vertex --sa-file sa.json      # Google service account
//	auth-cli list
//	auth-cli verify --all
package main

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/cmd/auth"
)

func main() {
	if err := auth.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
