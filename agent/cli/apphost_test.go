package cli_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/agent/cli"
)

// TestOpenForCLIRequiresAppName confirms empty appName returns an error.
func TestOpenForCLIRequiresAppName(t *testing.T) {
	_, err := cli.OpenForCLI("", slog.LevelInfo)
	if err == nil {
		t.Fatal("expected error for empty appName, got nil")
	}
	if !strings.Contains(err.Error(), "appName") {
		t.Errorf("error message should mention appName, got: %v", err)
	}
}

// TestMustOpenForCLIPanicsOnEmptyAppName confirms Must panics on empty appName.
func TestMustOpenForCLIPanicsOnEmptyAppName(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty appName, got nil")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected error panic, got: %T %v", r, r)
		}
		if !strings.Contains(err.Error(), "appName") {
			t.Errorf("panic message should mention appName, got: %v", err)
		}
	}()
	_ = cli.MustOpenForCLI("", slog.LevelInfo)
}

// TestOpenForCLISetsAppName ensures OpenForCLI internally calls gosdk/config
// and correctly binds appName — part of the helper contract.
func TestOpenForCLISetsAppName(t *testing.T) {
	appName := "agentsdk-config-test-app"
	cfg, err := cli.OpenForCLI(appName, slog.LevelInfo)
	if err != nil {
		t.Fatalf("OpenForCLI: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(cfg.DataDir)) })

	home, _ := os.UserHomeDir()
	wantPrefix := filepath.Join(home, ".config", appName)
	if !strings.HasPrefix(cfg.DataDir, wantPrefix) {
		t.Errorf("DataDir %q should start with %q", cfg.DataDir, wantPrefix)
	}
	if !strings.HasPrefix(cfg.LogDir, wantPrefix) {
		t.Errorf("LogDir %q should start with %q", cfg.LogDir, wantPrefix)
	}
	if !strings.HasSuffix(cfg.DataDir, "/data") {
		t.Errorf("DataDir %q should end with /data", cfg.DataDir)
	}
	if !strings.HasSuffix(cfg.LogDir, "/logs") {
		t.Errorf("LogDir %q should end with /logs", cfg.LogDir)
	}
}

// TestOpenForCLICreatesFiles verifies states/, wal/, log file exist on disk.
func TestOpenForCLICreatesFiles(t *testing.T) {
	cfg, err := cli.OpenForCLI("agentsdk-config-test-create", slog.LevelInfo)
	if err != nil {
		t.Fatalf("OpenForCLI: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(cfg.DataDir)) })

	for _, p := range []string{
		filepath.Join(cfg.DataDir, "states"),
		filepath.Join(cfg.DataDir, "wal"),
		cfg.LogFile,
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %q to exist: %v", p, err)
		}
	}

	// RunID must be UnixNano digits only.
	for _, c := range cfg.RunID {
		if c < '0' || c > '9' {
			t.Errorf("RunID should be digits only, got: %q", cfg.RunID)
			break
		}
	}

	// Store and WAL must be non-nil.
	if cfg.StateStore == nil {
		t.Error("StateStore must not be nil")
	}
	if cfg.WAL == nil {
		t.Error("WAL must not be nil")
	}
}
