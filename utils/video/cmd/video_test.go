package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestNewCommandBuildsVideoCommandTree(t *testing.T) {
	got := NewCommand()
	if got == nil {
		t.Fatal("NewCommand returned nil")
	}
	if got.Use != "video" {
		t.Fatalf("Use = %q, want %q", got.Use, "video")
	}

	for _, use := range []string{"audio <video>", "frames <video>", "subtitles <video>"} {
		if findCommand(got, use) == nil {
			t.Errorf("missing child command %q", use)
		}
	}
}

func TestNewCommandDoesNotShareFlagState(t *testing.T) {
	first := NewCommand()
	second := NewCommand()
	if first == second {
		t.Fatal("NewCommand returned the same command pointer twice")
	}

	firstAudio := findCommand(first, "audio <video>")
	secondAudio := findCommand(second, "audio <video>")
	if firstAudio == nil || secondAudio == nil {
		t.Fatal("audio command missing")
	}

	firstOut := firstAudio.Flags().Lookup("out")
	secondOut := secondAudio.Flags().Lookup("out")
	if firstOut == nil || secondOut == nil {
		t.Fatal("audio --out flag missing")
	}
	if firstOut == secondOut {
		t.Fatal("NewCommand shared the audio --out flag")
	}

	if err := firstAudio.Flags().Set("out", "first.wav"); err != nil {
		t.Fatalf("set first command flag: %v", err)
	}
	if got := secondOut.Value.String(); got != "./audio.wav" {
		t.Fatalf("second command --out = %q after mutating first, want %q", got, "./audio.wav")
	}
}

func findCommand(root *cobra.Command, use string) *cobra.Command {
	for _, child := range root.Commands() {
		if child.Use == use {
			return child
		}
	}
	return nil
}
