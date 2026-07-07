package runtime

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenForCLIRequiresAppName 確認空 appName 直接回 error。
func TestOpenForCLIRequiresAppName(t *testing.T) {
	_, err := OpenForCLI("", slog.LevelInfo)
	if err == nil {
		t.Fatal("expected error for empty appName, got nil")
	}
	if !strings.Contains(err.Error(), "appName") {
		t.Errorf("error message should mention appName, got: %v", err)
	}
}

// TestMustOpenForCLIPanicsOnEmptyAppName 確認 Must 版本在空 appName 時 panic。
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
	_ = MustOpenForCLI("", slog.LevelInfo)
}

// TestOpenForCLIRequiresWithAppName 確保即使 caller 沒先呼叫 WithAppName,
// OpenForCLI 內部會主動呼叫並正確綁定 appName — 這是 helper 的 contract。
// 不可能模擬「忘了呼叫」,因為 OpenForCLI 內部自己呼叫,
// 但仍可驗證傳入 appName 之後 GetAppName() 確實被設值。
func TestOpenForCLISetsAppName(t *testing.T) {
	// 用獨特 appName 避免與本機其他測試/CLI 衝突。
	appName := "agentsdk-runtime-test-app"
	dirs, err := OpenForCLI(appName, slog.LevelInfo)
	if err != nil {
		t.Fatalf("OpenForCLI: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(dirs.DataDir)) })

	// DataDir 必須以 ~/.config/<appName> 為前綴。
	home, _ := os.UserHomeDir()
	wantPrefix := filepath.Join(home, ".config", appName)
	if !strings.HasPrefix(dirs.DataDir, wantPrefix) {
		t.Errorf("DataDir %q should start with %q", dirs.DataDir, wantPrefix)
	}
	if !strings.HasPrefix(dirs.LogDir, wantPrefix) {
		t.Errorf("LogDir %q should start with %q", dirs.LogDir, wantPrefix)
	}
	if !strings.HasSuffix(dirs.DataDir, "/data") {
		t.Errorf("DataDir %q should end with /data", dirs.DataDir)
	}
	if !strings.HasSuffix(dirs.LogDir, "/log") {
		t.Errorf("LogDir %q should end with /log", dirs.LogDir)
	}
}

// TestOpenForCLICreatesFiles 驗證 wiring 之後磁碟上真的有 states/、wal/、log 檔。
func TestOpenForCLICreatesFiles(t *testing.T) {
	dirs, err := OpenForCLI("agentsdk-runtime-test-create", slog.LevelInfo)
	if err != nil {
		t.Fatalf("OpenForCLI: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(dirs.DataDir)) })

	for _, p := range []string{
		filepath.Join(dirs.DataDir, "states"),
		filepath.Join(dirs.DataDir, "wal"),
		dirs.LogFile,
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %q to exist: %v", p, err)
		}
	}

	// RunID 必須是 UnixNano 純數字。
	for _, c := range dirs.RunID {
		if c < '0' || c > '9' {
			t.Errorf("RunID should be digits only, got: %q", dirs.RunID)
			break
		}
	}
}
