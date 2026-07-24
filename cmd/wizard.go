package cmd

import (
	"github.com/bizshuk/agentsdk/cmd/wizard"
	"github.com/spf13/cobra"
)

// WizardCmd is the package-level cobra command variable for wizard.
var WizardCmd = wizard.Command

// NewWizardCommand returns WizardCmd for backward compatibility.
func NewWizardCommand() *cobra.Command {
	wizard.ResetFlags()
	return WizardCmd
}
