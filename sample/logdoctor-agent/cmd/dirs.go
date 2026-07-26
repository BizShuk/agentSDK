package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// dataDirOrDefault returns the persistence directory by precedence:
//
//  1. --data-dir flag on cmd (if set)
//  2. $LOGDOCTOR_DATA env var (if set)
//  3. ./data
//
// Centralized so run / resume / list agree on the path.
func dataDirOrDefault(cmd *cobra.Command) string {
	if cmd != nil && cmd.Flags() != nil {
		if d, err := cmd.Flags().GetString("data-dir"); err == nil && d != "" {
			return d
		}
	}
	if d := os.Getenv("LOGDOCTOR_DATA"); d != "" {
		return d
	}
	return "./data"
}
