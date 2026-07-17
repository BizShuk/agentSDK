package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestNewRootBuildsIndependentVideoCommands(t *testing.T) {
	first := findRootChild(t, NewRoot(), "video")
	second := findRootChild(t, NewRoot(), "video")
	if first == second {
		t.Fatal("NewRoot reused the same video command pointer")
	}
}

func findRootChild(t *testing.T, root *cobra.Command, use string) *cobra.Command {
	t.Helper()
	for _, child := range root.Commands() {
		if child.Use == use {
			return child
		}
	}
	t.Fatalf("root command %q not found", use)
	return nil
}
