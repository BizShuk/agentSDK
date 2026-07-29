package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	MODULE_PATH = "github.com/bizshuk/agentsdk"
	AUTH_MODULE = "github.com/bizshuk/auth"
)

// goListDeps 回傳 pkg 的完整 dependency closure（含 stdlib）。
func goListDeps(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", pkg).Output()
	require.NoError(t, err, "go list -deps %s", pkg)
	return strings.Fields(string(out))
}

// internalDeps 只保留本 module 的 package，濾掉 stdlib 與第三方。
func internalDeps(deps []string) []string {
	kept := make([]string, 0, len(deps))
	for _, dep := range deps {
		if strings.HasPrefix(dep, MODULE_PATH) {
			kept = append(kept, dep)
		}
	}
	return kept
}

// goListField 一次取得所有 package 的指定欄位，避免 per-package 迴圈。
func goListField(t *testing.T, field string) map[string][]string {
	t.Helper()
	tmpl := "{{.ImportPath}}\t{{join ." + field + " \" \"}}"
	out, err := exec.Command("go", "list", "-f", tmpl, "./...").Output()
	require.NoError(t, err, "go list -f %s ./...", tmpl)

	result := make(map[string][]string)
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		path, rest, found := strings.Cut(line, "\t")
		if !found {
			continue
		}
		result[path] = strings.Fields(rest)
	}
	return result
}

// TestDeclarativeLayersOnlySeeCore 鎖定宣告層的依賴閉包。
//
// 注意：go list -deps 是`遞移`閉包，所以 prompt/source 必然看得到 prompt 帶進來的
// core——這不是違規。真正的規則是「閉包內不得出現清單以外的 agentsdk package」。
func TestDeclarativeLayersOnlySeeCore(t *testing.T) {
	cases := []struct {
		pkg     string
		allowed []string
	}{
		{
			pkg:     "./agent/spec",
			allowed: []string{MODULE_PATH + "/core", MODULE_PATH + "/agent/spec"},
		},
		{
			pkg:     "./prompt",
			allowed: []string{MODULE_PATH + "/core", MODULE_PATH + "/prompt"},
		},
		{
			pkg: "./prompt/source",
			allowed: []string{
				MODULE_PATH + "/core",
				MODULE_PATH + "/prompt",
				MODULE_PATH + "/prompt/source",
			},
		},
		{
			pkg:     "./agent/permission",
			allowed: []string{MODULE_PATH + "/core", MODULE_PATH + "/agent/permission"},
		},
		{
			pkg:     "./agent/session",
			allowed: []string{MODULE_PATH + "/core", MODULE_PATH + "/agent/session"},
		},
		{
			pkg:     "./agent/wire",
			allowed: []string{MODULE_PATH + "/core", MODULE_PATH + "/agent/wire"},
		},
	}

	for _, tc := range cases {
		// subtest 名稱不能含 '/'，否則 go test -run 會把它當成巢狀 subtest 分隔符。
		name := strings.ReplaceAll(strings.TrimPrefix(tc.pkg, "./"), "/", "_")
		t.Run(name, func(t *testing.T) {
			got := internalDeps(goListDeps(t, tc.pkg))
			require.ElementsMatch(t, tc.allowed, got,
				"%s 的 agentsdk 依賴閉包超出宣告層允許範圍", tc.pkg)
		})
	}
}

// TestCoreImportsStdlibOnly 取代原本 grep vendor 名稱的做法。
//
// grep 'anthropic' core/*.go 會誤判註解與測試字串（實測 3 個 false positive）。
// 真正要守的是 import：core 不得依賴任何非 stdlib package。stdlib import path 的
// 第一段永遠不含 '.'，第三方永遠是 domain 開頭。
func TestCoreImportsStdlibOnly(t *testing.T) {
	for _, dep := range goListDeps(t, "./core") {
		if dep == MODULE_PATH+"/core" {
			continue
		}
		head, _, _ := strings.Cut(dep, "/")
		require.NotContains(t, head, ".",
			"core 只能依賴 stdlib，實際看到 %s", dep)
	}
}

// TestAuthImportedOnlyByProviderCredential 斷言 direct imports 而非遞移閉包：
// 「誰可以 import auth」是 ownership 規則，不隨 credential 的 caller 增減而變。
func TestAuthImportedOnlyByProviderCredential(t *testing.T) {
	const allowed = MODULE_PATH + "/provider/credential"

	imports := goListField(t, "Imports")
	require.NotEmpty(t, imports, "go list ./... 未回傳任何 package")

	var importers []string
	for pkg, deps := range imports {
		for _, dep := range deps {
			if strings.HasPrefix(dep, AUTH_MODULE) {
				importers = append(importers, pkg)
				break
			}
		}
	}

	for _, pkg := range importers {
		require.Equal(t, allowed, pkg,
			"只有 provider/credential 可 import %s，實際 %s 也 import 了", AUTH_MODULE, pkg)
	}
	require.NotEmpty(t, importers,
		"預期 provider/credential 仍 import %s；若已移除請一併更新 CLAUDE.md", AUTH_MODULE)
}
