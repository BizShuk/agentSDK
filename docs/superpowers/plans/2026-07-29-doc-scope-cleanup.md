# README / CLAUDE.md 範疇清理 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `README.md` 與 `CLAUDE.md` 裡不屬於它們的內容（外部 repo 內部細節、已進 CHANGELOG 的歷史、會腐爛的計數、兩檔重複段落、沒人執行的驗證指令）移到正確位置或刪除，並把其中的依賴紀律斷言變成 `go test ./...` 會跑的測試。

**Architecture:** 先把「文件裡的手動驗證指令」自動化成 Go test 與一支 workspace script（Task 1–2），文件才有東西可以指過去；接著才動文件（Task 3–7）。文件編輯一律以 anchor 文字定位，不依賴行號——刪除會讓後續行號位移。每個 task 結束於一次 commit。

**Tech Stack:** Go `1.26.0`、`testify v1.11.1`、`go list` (`-deps` / `-f` template)、bash、Markdown。

## Global Constraints

- 文件語言：繁體中文敘述 + 英文技術關鍵字，沿用兩檔既有風格。
- 常數命名：`SCREAMING_SNAKE_CASE`（含 unexported、block-scoped），與 gosdk 一致。
- 測試：table-driven + `t.Run` + `testify`。
- 文件內不得出現 `/Users/shuk/...` 絕對路徑；用相對路徑或 `$(git rev-parse --show-toplevel)`。
- `CLAUDE.md` 只記錄`現行`契約與 ownership；歷史一律屬於 `docs/CHANGELOG.md`，未完成工作一律屬於 `README.todo`。
- 每個保留在 `CLAUDE.md` 的不變式，必須要嘛有對應的測試/腳本，要嘛是一句可讀的規則——不得是無人執行的指令清單。
- 每個 commit message 結尾附：
  ```
  Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01XqVnAZGod2equMrMT3oN2p
  ```

## 基線事實 (已於撰寫本計畫時實測)

執行任何 task 前，這些是`已驗證`的現況，測試的期望值以此為準：

| 事實 | 實測結果 |
| --- | --- |
| `go test ./...`（root module） | 全綠 |
| `go list -deps ./agent/spec` 之 agentsdk package | `core`, `agent/spec` |
| `go list -deps ./prompt` | `core`, `prompt` |
| `go list -deps ./prompt/source` | `core`, `prompt`, `prompt/source` ← **`CLAUDE.md:433` 宣稱不該出現 `core`，是錯的** |
| `go list -deps ./agent/permission` / `./agent/session` / `./agent/wire` | 各為 `core` + 自己 |
| `go list -deps ./core` 非 stdlib 依賴 | 無（stdlib only） |
| `go list -deps ./provider/credential \| grep -c bizshuk/auth` | `10` |
| `./agent`、`./provider`、`./provider/anthropic` 之 auth 依賴 | `0` |
| `grep -rn 'minimax\|anthropic\|...' core/*.go` | **3 hits**（`credential.go:40` 註解 + `message_test.go` 兩處字串）← `CLAUDE.md:441` 的斷言今天就是紅的 |
| `test ! -d config` | `config/` 已不存在 |
| `sample/code-agent/compose.go` | **不存在**；已搬到 `sample/code-agent/cmd/compose.go` |
| `sample/code-agent/main.go` | `20` 行 |
| `utils/` 子套件 | `agentconfig`, `configfile`, `frontmatter`, `testutil`（共 4 個） |
| `provider/sample/` | 另有 `config/`、`svc/` 兩個子套件 |
| `docs/specs/` | 只有 4 個檔案；`2026-07-07-mcp-client.md`、`2026-07-07-perception-input-pillar.md` **不存在** |
| `docs/CHANGELOG.md:26-35` | 已完整收錄 M1–M6、proxy 3×3、37-entity catalog、auth/proxy 外部化、`config/` 解體、`perception/` 移除 |
| `docs/CHANGELOG.md:205` / `:213` | 已收錄 `mcp/` 與 `perception/` 移除 |

**兩個斷言今天是紅的、必須以正確形式重寫，不得原樣搬進測試：**

1. `prompt/source` 不該看到 `core` —— 錯。`prompt` 本身 import `core`，`-deps` 是遞移閉包，`core` 必然出現。正確斷言是「只能看到 `core` + `prompt` + 自己」。
2. `grep 'anthropic' core/*.go` 必須為空 —— 錯。註解與測試字串合法。正確斷言是「`core` 的 direct imports 全為 stdlib」。

---

## File Structure

| 檔案 | 動作 | 責任 |
| --- | --- | --- |
| `layering_test.go` | 建立 | workspace 依賴紀律的唯一執行點：宣告層閉包、`core` stdlib-only、`bizshuk/auth` containment |
| `scripts/verify-workspace.sh` | 建立 | 跨 10 個 module 的 build + test 迴圈（`go test ./...` 只涵蓋 root module） |
| `CLAUDE.md` | 修改 | 只留現行技術契約、ownership、結構樹、慣例 |
| `README.md` | 修改 | 只留五大支柱、tier 階梯、Getting Started、兩層 opt-in、Option 表、執行範例 |
| `README.todo` | 修改 | 收下唯一一條跨 repo 約束 |

`layering_test.go` 放 repo 根目錄（`package main`）而非 `internal/`：它斷言的是 workspace 級不變式，根目錄是它的自然歸屬，且不需要為此新增一層目錄結構。

---

## Task 1: 依賴紀律測試 (`layering_test.go`)

**Files:**
- Create: `layering_test.go`
- Reference (不修改): `CLAUDE.md:404-424`（被取代的手動指令）

**Interfaces:**
- Consumes: 無
- Produces: `TestDeclarativeLayersOnlySeeCore`、`TestCoreImportsStdlibOnly`、`TestAuthImportedOnlyByProviderCredential` —— Task 3 會在 `CLAUDE.md` 指名這三個測試。
- Helper 簽章（同檔內共用）：`goListDeps(t *testing.T, pkg string) []string`、`internalDeps(deps []string) []string`、`goListField(t *testing.T, field string) map[string][]string`。

- [ ] **Step 1: 寫下失敗測試（宣告層閉包）**

建立 `layering_test.go`：

```go
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
		t.Run(tc.pkg, func(t *testing.T) {
			got := internalDeps(goListDeps(t, tc.pkg))
			require.ElementsMatch(t, tc.allowed, got,
				"%s 的 agentsdk 依賴閉包超出宣告層允許範圍", tc.pkg)
		})
	}
}
```

- [ ] **Step 2: 執行，確認通過**

Run: `go test . -run TestDeclarativeLayersOnlySeeCore -v`
Expected: 6 個 subtest 全 PASS（基線事實表已實測）。

- [ ] **Step 3: 證明測試抓得到違規**

此測試守護的是既有不變式，天生是綠的。必須先證明它會紅，否則等於沒寫。

暫時在 `agent/spec/` 任一 `.go` 檔（例如 `agent/spec/config.go`）的 import block 加入：

```go
	_ "github.com/bizshuk/agentsdk/tool"
```

Run: `go test . -run TestDeclarativeLayersOnlySeeCore/agent -v`
Expected: **FAIL**，訊息含 `agent/spec 的 agentsdk 依賴閉包超出宣告層允許範圍`，且 diff 列出 `github.com/bizshuk/agentsdk/tool`。

- [ ] **Step 4: 還原違規**

Run: `git checkout agent/spec/config.go`
Run: `go test . -run TestDeclarativeLayersOnlySeeCore`
Expected: PASS

- [ ] **Step 5: 加入 `core` stdlib-only 測試**

附加到 `layering_test.go`：

```go
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
		head := dep
		if idx := strings.Index(dep, "/"); idx >= 0 {
			head = dep[:idx]
		}
		require.NotContains(t, head, ".",
			"core 只能依賴 stdlib，實際看到 %s", dep)
	}
}
```

- [ ] **Step 6: 執行，確認通過**

Run: `go test . -run TestCoreImportsStdlibOnly -v`
Expected: PASS

- [ ] **Step 7: 加入 auth containment 測試**

附加到 `layering_test.go`：

```go
// goListField 一次取得所有 package 的指定欄位，避免 per-package 迴圈。
func goListField(t *testing.T, field string) map[string][]string {
	t.Helper()
	tmpl := "{{.ImportPath}}\t{{join ." + field + " \" \"}}"
	out, err := exec.Command("go", "list", "-f", tmpl, "./...").Output()
	require.NoError(t, err, "go list -f %s ./...", tmpl)

	result := make(map[string][]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path, rest, found := strings.Cut(line, "\t")
		if !found {
			continue
		}
		result[path] = strings.Fields(rest)
	}
	return result
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
```

- [ ] **Step 8: 執行，確認通過**

Run: `go test . -run TestAuthImportedOnlyByProviderCredential -v`
Expected: PASS，且不列出 `provider/credential` 以外的 importer。

- [ ] **Step 9: 證明 auth 測試抓得到違規**

暫時在 `agent/build.go` 的 import block 加入：

```go
	_ "github.com/bizshuk/auth/model"
```

Run: `go test . -run TestAuthImportedOnlyByProviderCredential -v`
Expected: **FAIL**，訊息含 `只有 provider/credential 可 import github.com/bizshuk/auth，實際 github.com/bizshuk/agentsdk/agent 也 import 了`。

- [ ] **Step 10: 還原並跑完整測試**

Run: `git checkout agent/build.go`
Run: `go test ./... 2>&1 | tail -5`
Expected: 全綠，無 FAIL。

- [ ] **Step 11: Commit**

```bash
git add layering_test.go
git commit -m "$(cat <<'EOF'
test: enforce layering invariants in go test instead of doc commands

宣告層閉包、core stdlib-only、auth containment 三條規則原本只是 CLAUDE.md
裡沒人執行的指令。其中兩條寫法本身就是錯的：prompt/source 的遞移閉包必然
含 core，core/*.go 的 vendor 名稱 grep 會誤判註解與測試字串。改以 direct
import 與閉包白名單斷言，納入 go test ./...。

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01XqVnAZGod2equMrMT3oN2p
EOF
)"
```

---

## Task 2: Workspace 建置腳本 (`scripts/verify-workspace.sh`)

**Files:**
- Create: `scripts/verify-workspace.sh`
- Reference (不修改): `CLAUDE.md:396-402`（被取代的 `for mod in ...` 迴圈）

**Interfaces:**
- Consumes: `go.work` 的 10 個 `use` entries
- Produces: 可執行檔 `scripts/verify-workspace.sh`，成功時 exit 0 —— Task 3 會在 `CLAUDE.md` 指名這個路徑。

`go test ./...` 只跑 root module；9 個 sample module 需要各自進去跑。這是 workflow 不是斷言，所以留在腳本而非測試。

- [ ] **Step 1: 建立腳本**

建立 `scripts/verify-workspace.sh`：

```bash
#!/usr/bin/env bash
#
# 跨 go.work 全部 module 執行 build + test。
# go test ./... 只涵蓋 root module；sample/* 各自是獨立 module。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

# 直接從 go.work 讀取，避免腳本與 go.work 各存一份 module 清單。
MODULES="$(go list -m -f '{{.Dir}}' 2>/dev/null || true)"
if [[ -z "${MODULES}" ]]; then
	echo "verify-workspace: 無法從 go.work 取得 module 清單" >&2
	exit 1
fi

FAILED=0
while IFS= read -r dir; do
	[[ -z "${dir}" ]] && continue
	rel="${dir#"${ROOT}"/}"
	[[ "${dir}" == "${ROOT}" ]] && rel="."
	printf '=== %s\n' "${rel}"
	if ! (cd "${dir}" && go build ./... && go test ./... -count=1 -timeout=120s); then
		echo "verify-workspace: ${rel} FAILED" >&2
		FAILED=1
	fi
done <<< "${MODULES}"

if [[ "${FAILED}" -ne 0 ]]; then
	echo "verify-workspace: 有 module 未通過" >&2
	exit 1
fi
echo "verify-workspace: 全部 module 通過"
```

- [ ] **Step 2: 設為可執行並驗證 module 列舉正確**

Run: `chmod +x scripts/verify-workspace.sh && go list -m -f '{{.Dir}}' | wc -l`
Expected: `10`（root + 9 samples，與 `go.work` 的 `use` entries 一致）

- [ ] **Step 3: 執行腳本**

Run: `bash scripts/verify-workspace.sh 2>&1 | tail -15`
Expected: 10 個 `=== <module>` 區塊，最後一行 `verify-workspace: 全部 module 通過`，exit code 0。

若某個 sample module 未通過：**不要**在本 task 修它。記錄下來、把該失敗回報給計畫發起人，本 task 只負責腳本本身正確。

- [ ] **Step 4: 驗證失敗時會 exit 非零**

Run: `bash -c 'cd "$(git rev-parse --show-toplevel)" && sed "s/exit 0/exit 0/" scripts/verify-workspace.sh >/dev/null; echo $?'`
Expected: `0`

接著實測非零路徑——暫時在 `sample/greet-agent/main.go` 開頭加入一行語法錯誤 `!!!`：

Run: `bash scripts/verify-workspace.sh >/dev/null 2>&1; echo "exit=$?"`
Expected: `exit=1`

- [ ] **Step 5: 還原**

Run: `git checkout sample/greet-agent/main.go && bash scripts/verify-workspace.sh >/dev/null 2>&1; echo "exit=$?"`
Expected: `exit=0`

- [ ] **Step 6: Commit**

```bash
git add scripts/verify-workspace.sh
git commit -m "$(cat <<'EOF'
chore: add scripts/verify-workspace.sh for cross-module build and test

module 清單直接讀 go.work，不在腳本內複製一份。

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01XqVnAZGod2equMrMT3oN2p
EOF
)"
```

---

## Task 3: `CLAUDE.md` 開發與驗證章節瘦身

**Files:**
- Modify: `CLAUDE.md`，`## 開發與驗證 (Development and Verification)` 整章（撰稿時為 `349-461`，約 110 行）

**Interfaces:**
- Consumes: Task 1 的 `TestDeclarativeLayersOnlySeeCore` / `TestCoreImportsStdlibOnly` / `TestAuthImportedOnlyByProviderCredential`；Task 2 的 `scripts/verify-workspace.sh`
- Produces: 一個約 40 行的章節，後續 task 不再改動它

- [ ] **Step 1: 確認起訖 anchor**

Run: `grep -n "^## 開發與驗證\|^## 專案追蹤文件" CLAUDE.md`
Expected: 兩個行號，前者約 `349`、後者約 `462`。要替換的是`前者那一行`到`後者前一行`（不含後者）。

- [ ] **Step 2: 以下列內容整段取代該章節**

```markdown
## 開發與驗證 (Development and Verification)

前置需求：Go `1.26+`；使用 provider adapter 時依該 module 的 API key/environment。

```bash
go work sync
go mod download
go test ./...                      # root module，含依賴紀律測試
bash scripts/verify-workspace.sh   # go.work 全部 module 的 build + test
```

依賴紀律不是文件裡的手動指令，是 `layering_test.go` 的三個測試：

| 不變式 | 測試 |
| --- | --- |
| `agent/spec`、`prompt`、`prompt/source`、`agent/{permission,session,wire}` 的 agentsdk 依賴閉包只含 `core` 與自身 | `TestDeclarativeLayersOnlySeeCore` |
| `core` 只依賴 stdlib | `TestCoreImportsStdlibOnly` |
| 只有 `provider/credential` 可 import `github.com/bizshuk/auth` | `TestAuthImportedOnlyByProviderCredential` |

依賴圖分析（外部 CLI，不在本 workspace）：

```bash
go-dependency-analysis --workspace ./go.work --format text
```

`go-tool-fact` 來自當次 Go toolchain/build context，`policy-heuristic` 才是建議。
`unused-direct-candidate` 必須先檢查 tests、build tags、platform files 與 generated
code，不能直接刪 require。完整 flags 見該 repo 的 README。

`provider` 子指令 smoke-test（不走 Agent/Engine，直接打 `core.Provider`）：

```bash
go run . provider --list-providers
go run . provider --list-models --provider minimax
go run . provider "ping" --provider minimax
go run . provider --stream "say hi in one word" --provider minimax
go run . provider "ping" --provider minimax --json | jq
```

`wizard` 子指令（產生 `agent.Config`，不打 provider、不驗憑證）：

```bash
go run . w                                  # 互動：逐階段問，Enter 收預設，寫 ./agent.yaml
go run . w -y --tier full -o -              # 非互動：全採預設，輸出 stdout
go run . w -y --tier oneshot -o agent.json  # 副檔名決定格式
go run . w --edit agent.yaml                # 以既有設定當預設值（round-trip 無損）
go run . w -o - --print-go                  # 額外印出等價的 Go literal
go run . w --list reasoning.style           # 列出單一欄位的選項
```

Sample 執行：

```bash
export MINIMAX_API_KEY=...
go run ./sample/log-agent-v2 --interval 1m   # 先等 interval 再掃描
(cd sample/logdoctor-agent && go run . watch)  # 啟動即掃描

cd sample/code-agent
go run . --fake -p "看看這個專案"   # print 模式（進度走 stderr）
go run . --fake --json -p "test"   # stream-json envelope（wire）
go run . --fake                     # 互動 TUI（執行中輸入 = Steer）
go run . --fake --sessions          # 列出本目錄 sessions；-c / -r / --fork 續跑
go run . --provider anthropic -p "..."  # 改讀 ANTHROPIC_API_KEY
```

`code-agent` 的 provider 選擇：`--provider minimax`（預設，讀 `MINIMAX_API_KEY` /
`MINIMAX_BASE_URL`）或 `--provider anthropic`（讀 `ANTHROPIC_API_KEY`）；`--model`
留空用 adapter flagship 預設；`--api-key` / `--base-url` 為顯式覆寫。

`sample/log-agent-v2` 固定使用 MiniMax，沒有 provider selector、fake mode、tools、
approval 或 session UI；以 `agent.New` / `Run` / `WithListener` / `WithSink` 展示完整
lifecycle，cursor 位於 `~/.config/log-agent-v2/data/log-cursor.json`。
`sample/logdoctor-agent` 是比較用的單一 `watch` command，走 `agent.OnceStream`。
兩者皆將 Markdown 寫 stdout、`core.StreamEvent` 寫 stderr。`sample/file-agent` 與
`sample/greet-agent` 使用 Anthropic-compatible adapter 與 `preset.Secure`。

`sample/skeleton-agent` 是 `wizard --print-go` 輸出範本逐字對應的單檔 sample：沒有
cobra、沒有四種 dispatch mode、不需要 `*Parts.Sessions` / `*Parts.Skills`；`stdinAgent`
包裝負責把 stdin 內容塞進 Bootstrap 回傳的 opening state。對比 shape 見
`sample/skeleton-agent/README.md`。
```

本步驟同時完成三件事，勿分拆：刪掉 `for mod in ...` 迴圈與 11 條 `go list -deps` / `grep` 斷言（已由 Task 1–2 承接）、刪掉 `test ! -d config`（`config/` 早已不存在，斷言一個不存在目錄的不存在是 changelog）、把所有 `/Users/shuk/...` 換成相對路徑，並修正 `cmd/agent/wizard.go::goLiteral` → `wizard --print-go`（實際檔案是 `cmd/agent/wizard/command.go` 與 `helper.go`），移除 `~260 行`、`12 行` 兩個計數。

- [ ] **Step 3: 驗證絕對路徑與死斷言都消失**

Run: `grep -n "/Users/shuk\|test ! -d config\|go list -deps\|wizard.go::goLiteral\|260 行" CLAUDE.md`
Expected: 無輸出（exit 1）

- [ ] **Step 4: 驗證新指向有效**

Run: `grep -n "layering_test.go\|verify-workspace.sh" CLAUDE.md && ls layering_test.go scripts/verify-workspace.sh`
Expected: `CLAUDE.md` 各有命中，兩個檔案都存在。

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: point CLAUDE.md verification section at tests instead of inlining them

110 行手動指令縮為約 40 行：斷言交給 layering_test.go，跨 module 迴圈交給
scripts/verify-workspace.sh，並移除機器專屬絕對路徑與已消失目錄的斷言。

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01XqVnAZGod2equMrMT3oN2p
EOF
)"
```

---

## Task 4: `CLAUDE.md` 移除外部 repo 內部細節

**Files:**
- Modify: `CLAUDE.md`
  - 刪除 `## 認證與 provider 決策 (Auth and Providers)` 整章（撰稿時 `222-247`）
  - 刪除 `## Proxy pairwise 決策 (Current Proxy Architecture)` 與其下 `### Proxy HTTP surface`（撰稿時 `249-295`）
  - 刪除 `CLI、設定與持久化` 內的 proxy defaults 段（撰稿時 `312`，`Proxy defaults（proxy/config.go）...` 起始）
  - 修改 `模組對應` 表的 `authentication` 與 `proxy` 兩列
  - 新增兩則 bullet 到 `## 核心架構決策 (Core Decisions)` 末端

**Interfaces:**
- Consumes: 無
- Produces: `CLAUDE.md` 不再描述任何無法在本 repo build/test 的程式碼

理由：兩章各自以 `> 範圍：本節描述外部 repo ...` 開頭，等於自承不屬於這裡。`auth/provider.ROUTES`（9 個 id）、`0700`/`0600` 權限、temp+rename、`proxy/config.go` 的 port `8317` / body limit `200 MB` / stream timeout `600s`、`/v1` 路由表與 admin `501` —— 這些會在對方 repo 改動時無聲腐爛，本 repo 沒有任何測試會發現。

- [ ] **Step 1: 在核心架構決策末端新增消費端契約**

定位 anchor：

Run: `grep -n "core.CREDENTIAL_KIND_\*\`，\`Resolve\` 產生 canonical" CLAUDE.md`
Expected: 一個行號（撰稿時 `220`），該 bullet 是 `## 核心架構決策` 的最後一項。

在該 bullet 之後、`## 認證與 provider 決策` 標題之前，插入：

```markdown
- 外部 `bizshuk/auth` 的消費邊界：只有 `provider/credential` 可 import 它（由
  `TestAuthImportedOnlyByProviderCredential` 把關），並擁有
  `(provider name, credential kind) → auth route id` 對照；auth 的扁平 route ID 不進
  `spec.Model.Provider`。endpoint 不進 `core.Auth`，一律由 `provider.Options.BaseURL`
  在 construction time 指定。credential 優先序固定為
  `單次 request Auth → 明示 Options.APIKey → Decorator → env`。auth 自身的 credential
  storage、OAuth / device-code flow 與 CLI 屬該 repo 的契約，見
  [`bizshuk/auth`](https://github.com/BizShuk/auth)。
- LLM protocol proxy 是外部 repo [`bizshuk/proxy`](https://github.com/BizShuk/proxy)：
  本 repo 無 `proxy/` 目錄、無 go.mod require、無任何 import，其架構與 HTTP surface
  由該 repo 自行記錄。
```

- [ ] **Step 2: 刪除兩個外部 repo 章節**

刪除從 `## 認證與 provider 決策 (Auth and Providers)` 這一行起，到 `## CLI、設定與持久化 (CLI, Config, Persistence)` 前一行為止的所有內容（含中間的 `## Proxy pairwise 決策` 與 `### Proxy HTTP surface`）。

Run: `grep -n "^## 認證與 provider 決策\|^## Proxy pairwise\|^### Proxy HTTP surface\|^## CLI、設定與持久化" CLAUDE.md`
Expected（刪除後）：只剩 `## CLI、設定與持久化` 一行。

- [ ] **Step 3: 刪除 proxy defaults 段**

刪除以 `Proxy defaults（\`proxy/config.go\`）：port \`8317\`` 開頭的整個段落。

Run: `grep -n "8317\|200 MB\|proxy/config.go" CLAUDE.md`
Expected: 無輸出

- [ ] **Step 4: 修正模組對應表兩列**

`authentication` 列改為：

```markdown
| authentication            | 外部 module `github.com/bizshuk/auth`：只由 `provider/credential` 消費；API 契約見該 repo                                                                                                                                                                                                                                                            |
```

`proxy` 列改為：

```markdown
| proxy                     | 外部 repo `github.com/bizshuk/proxy`：本 repo 無目錄、無 require、無 import                                                                                                                                                                                                                                                        |
```

- [ ] **Step 5: 驗證外部 repo 細節已清空**

Run: `grep -n "ROUTES\|0700\|0600\|anthropic-messages\|openai-responses\|count_tokens\|credential_unavailable\|3×3" CLAUDE.md`
Expected: 無輸出

Run: `grep -c "bizshuk/auth\|bizshuk/proxy" CLAUDE.md`
Expected: 一個小數字（約 `4`–`8`）——只剩邊界宣告與模組對應表的指向，不再有內部細節。

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: drop external auth/proxy internals from CLAUDE.md

兩章各自以「本節描述外部 repo」開頭，記錄的是 bizshuk/auth 與 bizshuk/proxy
的實作細節——本 repo 無法 build、無法 test、無測試會在對方改動時發現漂移。
保留消費端契約（誰能 import auth、endpoint 不進 core.Auth、credential 優先序）
與指向連結。

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01XqVnAZGod2equMrMT3oN2p
EOF
)"
```

---

## Task 5: `CLAUDE.md` 移除歷史敘述、易腐計數與過時事實

**Files:**
- Modify: `CLAUDE.md`（`技術基準`、`專案結構`、`核心架構決策` 三章的零星行）

**Interfaces:**
- Consumes: 無
- Produces: `CLAUDE.md` 只陳述現行不變式

- [ ] **Step 1: 修正 code-agent 樹狀項（過時事實）**

`sample/code-agent/compose.go` 不存在，已搬到 `sample/code-agent/cmd/compose.go`；`main.go` 現為 20 行。

原文：

```
│   ├── code-agent/                   # 全 harness 組合 CLI：tui 互動 / -p print / --json（wire）+ session flags；compose.go 用 agent 宣告（333→101 行）
```

改為：

```
│   ├── code-agent/                   # 全 harness 組合 CLI：tui 互動 / -p print / --json（wire）+ session flags；composition 位於 cmd/
```

- [ ] **Step 2: 補上樹狀圖缺漏的實際子套件**

`utils/` 實際有 4 個子套件，樹狀圖只列了 3 個（缺 `agentconfig`）。

原文：

```
├── utils/                            # 根層共用 utilities umbrella：utils/frontmatter/（adrg/frontmatter YAML/TOML/JSON wrapper,key:value 攤平為 string map）+ utils/configfile/（副檔名決定編碼、一律以 JSON 呈現給 caller,故 `json` tag 是唯一真相）+ utils/testutil/（in-process fake provider/state store/notifier）
```

改為：

```
├── utils/                            # 根層共用 utilities umbrella
│   ├── agentconfig/                  # agent 設定檔 I/O：Decode/Encode/LoadFile/SaveFile（re-export utils/configfile）
│   ├── configfile/                   # 副檔名決定編碼、一律以 JSON 呈現給 caller,故 `json` tag 是唯一真相
│   ├── frontmatter/                  # adrg/frontmatter YAML/TOML/JSON wrapper,key:value 攤平為 string map
│   └── testutil/                     # in-process fake provider / state store / notifier（僅測試可用）
```

`provider/sample/` 另有 `config/` 與 `svc/` 兩個子套件，原文未提。原文：

```
│   ├── sample/                       # provider/auth/chat-image-audio capability matrix + direct access CLI
```

改為：

```
│   ├── sample/                       # provider/auth/chat-image-audio capability matrix + direct access CLI（含 config/、svc/ 子套件）
```

Run: `ls utils && ls provider/sample`
Expected: `utils` 列出 `agentconfig`、`configfile`、`frontmatter`、`testutil`；`provider/sample` 含 `config`、`svc`。

Run: `grep -n "utils/agentconfig" CLAUDE.md`
Expected: 至少兩個命中（樹狀圖 + 模組對應表既有的 `agent 設定檔 I/O` 列）

- [ ] **Step 3: 刪除易腐計數**

移除下列數字（保留其所在句子的結構性陳述）：

| 位置 | 刪除 | 保留 |
| --- | --- | --- |
| `技術基準` provider image 段 | `128 MiB` / `1 MiB` / `16 KiB` 三個上限 | 「成功 response、error body 與 details 各有大小上限，數值由 provider layer 常數擁有」 |
| `技術基準` 開頭 | `共 10 個 module entries（root + 9 個 sample modules）` 的計數 | 「`go.work` 納入 root 與 `sample/` 下各 module」 |
| `核心架構決策` proxy 相關殘句 | `3×3`、`37` 等外部 repo 計數（若 Task 4 後仍有殘留） | — |

Run: `grep -n "128 MiB\|16 KiB\|333→101\|~260\|37 個" CLAUDE.md`
Expected: 無輸出

- [ ] **Step 4: 刪除歷史敘述子句**

搜尋並逐一處理：

Run: `grep -n "已移除\|已解體\|已併回\|已下沉\|已於 \`[0-9a-f]\{7\}\`\|原 \`" CLAUDE.md`

對每個命中：保留句子的`現況`部分，刪掉`曾經如何`的部分。例：

- 「`Instruction` 是 tagged union，只保留有 production producer/consumer 的 `call_model`、`call_tool`、`request_approval`、`notify`、`done`；持久化由 runtime 的 StateStore/WAL lifecycle 負責，presentation 由 `core.EventSink` / `Engine.Emitter` 負責，**不另立 `checkpoint` / `emit` command**。」
  → 刪除粗體子句。`README.todo` 的「Agent 組裝與設定」已追蹤恢復這兩個 instruction，那才是它的歸屬。

- 「`auth` 是外部 module ...，**`proxy` 已無任何殘留（無目錄、無 require、無 import）**」
  → 粗體部分在 Task 4 已由新 bullet 涵蓋，刪除重複。

- [ ] **Step 5: 驗證**

Run: `grep -n "已移除\|已解體\|已併回\|已下沉" CLAUDE.md`
Expected: 無輸出

Run: `grep -c "" CLAUDE.md`
Expected: 約 `240`–`270`（原 `478`）

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: strip history prose and rotting counts from CLAUDE.md

修正三個已過時的事實（compose.go 路徑、utils 子套件數、provider/sample 結構），
移除行數/位元組上限/module 計數等會腐爛的量測值，並刪除「曾經如何」的敘述——
歷史屬於 docs/CHANGELOG.md，未完成項目屬於 README.todo。

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01XqVnAZGod2equMrMT3oN2p
EOF
)"
```

---

## Task 6: `README.md` 移除歷史表格與外部 proxy 章節

**Files:**
- Modify: `README.md`
  - 刪除 `## 開發狀態 (Milestones)` 表格（撰稿時 `271-284`），保留其後的 spec 連結清單
  - 刪除 `## 已淘汰功能 (Deprecated Features)` 整節（撰稿時 `307-312`）
  - 刪除 `## Proxy protocol bridge` 整節（撰稿時 `199-218`）
- Modify: `README.todo`（收下 `core.Part` 跨 repo 約束）

**Interfaces:**
- Consumes: 無
- Produces: `README.md` 不再重複 `docs/CHANGELOG.md` 的內容

- [ ] **Step 1: 確認歷史已在 CHANGELOG（刪除前的前提）**

Run: `sed -n '26,35p' docs/CHANGELOG.md`
Expected: 「遷移時里程碑快照」段落，含 M1–M6、proxy `3×3`、`37` entity catalog、auth/proxy 外部化、`config/` 解體、`perception/` 移除。

Run: `sed -n '205p;213p' docs/CHANGELOG.md`
Expected: 分別是 `mcp/` client 移除與 `perception/` 移除的記載。

前提成立 → 這是`刪除`不是`搬移`。若上述任一不成立，先把缺的內容補進 `docs/CHANGELOG.md` 再繼續。

- [ ] **Step 2: 刪除 Milestones 表格，保留 spec 索引**

刪除 `## 開發狀態 (Milestones)` 標題與其下整個表格（`| Milestone | 範疇 | 狀態 |` 到最後一列）。表格之後的段落改寫為：

```markdown
## 規格與歷史

- 現行技術契約：[`CLAUDE.md`](CLAUDE.md)
- 歷史變更與已完成里程碑：[`docs/CHANGELOG.md`](docs/CHANGELOG.md)
- 尚未完成的工作：[`README.todo`](README.todo)
- 已實作規格：
  - [`2026-07-29-Summary.md`](docs/specs/2026-07-29-Summary.md)（M1–M5 歷史摘要）
  - [`2026-07-18-continuous-logdoctor-minimax.md`](docs/specs/2026-07-18-continuous-logdoctor-minimax.md)
  - [`2026-07-27-agent-sdk-contract-alignment.md`](docs/specs/2026-07-27-agent-sdk-contract-alignment.md)
  - [`2026-07-27-provider-auth-image-capabilities.md`](docs/specs/2026-07-27-provider-auth-image-capabilities.md)
```

- [ ] **Step 3: 刪除已淘汰功能表**

刪除 `## 已淘汰功能 (Deprecated Features)` 標題與其下表格。該表的兩列都已在 `docs/CHANGELOG.md:205` 與 `:213`，且兩個 `原始文件` 連結（`2026-07-07-mcp-client.md`、`2026-07-07-perception-input-pillar.md`）指向`不存在`的檔案。

Run: `ls docs/specs/`
Expected: 4 個檔案，均非 `2026-07-07-*`。

- [ ] **Step 4: 把跨 repo 約束移進 `README.todo`**

`## Proxy protocol bridge` 中唯一對本 repo 有效的內容是一條`未來約束`。在 `README.todo` 末端新增：

```markdown
## 跨 repo 約束 (Cross-repo Constraints)

- [ ] 若外部 `bizshuk/proxy` 之後改用 `core.Part` 作 canonical message IR：Anthropic
      `thinking` 與 Responses `reasoning` 必須映射為 `PART_KIND_REASONING`，opaque
      continuation state 不可降格成 plain text。該 repo 負責解碼其版本化 signature
      envelope；本 repo 負責維持 `core.Part` / `ReasoningState` 契約不變。
```

- [ ] **Step 5: 刪除 Proxy protocol bridge 整節**

刪除 `## Proxy protocol bridge` 標題到 `## 設計原則` 前一行為止的全部內容（含 `> 範圍：` 引言、ASCII 流程圖與 6 條 bullet）。

- [ ] **Step 6: 驗證**

Run: `grep -n "Milestone\|已淘汰\|Proxy protocol bridge\|2026-07-07-\|3×3" README.md`
Expected: 無輸出

Run: `grep -n "PART_KIND_REASONING" README.todo`
Expected: 一個命中

- [ ] **Step 7: Commit**

```bash
git add README.md README.todo
git commit -m "$(cat <<'EOF'
docs: remove changelog tables and external proxy section from README

Milestones 與已淘汰功能兩表已完整存在於 docs/CHANGELOG.md，且後者的兩個
spec 連結都指向已不存在的檔案。Proxy protocol bridge 描述外部 repo，其中
唯一對本 repo 有效的 core.Part 約束移入 README.todo。

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01XqVnAZGod2equMrMT3oN2p
EOF
)"
```

---

## Task 7: `README.md` 與 `CLAUDE.md` 去重

**Files:**
- Modify: `README.md`
  - 刪除 provider/reasoning/auth 內部細節段（撰稿時 `19-51` 與 `63-68`），保留 image 程式碼範例
  - 以指標取代模組結構樹（撰稿時 `157-197`）
  - 刪除 `## 設計原則`（撰稿時 `220-229`），其中兩則獨有內容併入 `CLAUDE.md`
  - 刪除 `## 慣例衝突` 與 `## 慣例`（撰稿時 `294-305`）
  - 修正 `執行範例` 的 `101 行` 宣稱（撰稿時 `233`）
- Modify: `CLAUDE.md`（`核心架構決策` 收下兩則 README 獨有的決策）

**Interfaces:**
- Consumes: Task 4 已在 `CLAUDE.md` 建立的外部邊界 bullet
- Produces: 每個事實只有一個 owner

ownership 規則：**README = 為什麼用它、怎麼開始；CLAUDE = 邊界是什麼、誰擁有什麼。**

- [ ] **Step 1: 刪除 README 的深層技術段落**

刪除從 `模型執行介面刻意保持最小：` 起，到 `provider.ImageGenerator optional capability` 段落`之前`為止的全部內容（撰稿時 `19-51`）。這些段落逐條對應 `CLAUDE.md` 的 `核心架構決策`：provider capability boundary、reasoning content boundary、`Options.Resolve` pipeline、credential 優先序、protocol codec 共用條件。

保留：`provider.NewImage` 的 Go 程式碼範例與其前後各一句（`errors.Is(err, provider.ErrUnsupportedCapability)`、blank-import 說明、`provider/sample` 指向）——那是 getting-started 素材。

- [ ] **Step 2: 以指標取代模組結構樹**

刪除 `## 模組結構` 標題下的整棵 ```tree``` 區塊與其後的說明段落，改為：

```markdown
## 模組結構

五大支柱對應到頂層 package（見上表）。完整目錄樹、每個 package 的 ownership 與
架構不變式由 [`CLAUDE.md`](CLAUDE.md) 擁有，不在此重複——重複的兩份樹已經開始分岔。
```

理由：README 那棵樹缺 `utils/agentconfig`、`utils/configfile` 與 `cmd/agent/wizard/`，`CLAUDE.md` 那棵是準確的。

- [ ] **Step 3: 把 README 獨有的兩則決策併入 CLAUDE.md**

`## 設計原則` 中只有兩則不重複：

在 `CLAUDE.md` 的 `## 核心架構決策` 末端（Task 4 新增的兩則 bullet 之後）加入：

```markdown
- `core.Notifier` 的介面方法集與 `gosdk/notify.Notifier` 完全相同（結構性相容），
  gosdk 的 Multi / Stdout / Slack 可直接傳入，不需 adapter。
- Presets, not walls：設定挑 preset 而非組合細節（middleware 鏈的順序是正確性，
  不是偏好）；`WithCustomize` 在全部 stage 之後拿到組好的 `*runtime.Engine`，
  設定詞彙沒覆蓋的都還做得到。
```

- [ ] **Step 4: 刪除 README 的 設計原則 / 慣例衝突 / 慣例 三節**

刪除 `## 設計原則`、`## 慣例衝突 (Naming Collision)`、`## 慣例` 三個標題與其內容。

- `設計原則` 其餘 5 條全部重複於 `CLAUDE.md` 的 `核心架構決策`。
- `慣例衝突` 已存在於 `CLAUDE.md` 的 `慣例與注意事項`（`sample/logdoctor-agent/core` 與 `agentsdk/core` 撞名，import 時用 `sdkcore` / `domain` alias）。
- `慣例` 重複於 `CLAUDE.md:470-478`，且其最後一條「遵循 `playground/CLAUDE.md` 慣例」指向本 repo 讀者無從取得的跨專案檔案——一併刪除，不搬移。

- [ ] **Step 5: 修正執行範例的行數宣稱**

原文：

```
`sample/code-agent` — 全能力組合，composition 只有 `101` 行（宣告 `agent.Config` 而非手工接線）：
```

改為：

```
`sample/code-agent` — 全能力組合，以宣告 `agent.Config` 取代手工接線：
```

- [ ] **Step 6: 驗證去重完成**

Run: `grep -n "設計原則\|慣例衝突\|101 行\|playground/CLAUDE.md" README.md`
Expected: 無輸出

Run: `grep -c "" README.md`
Expected: 約 `130`–`160`（原 `312`）

Run: `grep -n "gosdk/notify.Notifier\|presets, not walls\|Presets, not walls" CLAUDE.md`
Expected: 兩個命中（Step 3 併入的內容）

- [ ] **Step 7: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: deduplicate README against CLAUDE.md

README 的模組樹已與 CLAUDE.md 分岔（缺 utils/agentconfig、utils/configfile、
cmd/agent/wizard/），設計原則與慣例兩節則整段重複。改由 CLAUDE.md 單一擁有
結構與決策，README 專注於支柱、tier 階梯、Getting Started 與執行範例。

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01XqVnAZGod2equMrMT3oN2p
EOF
)"
```

---

## Task 8: 連結完整性與最終驗收

**Files:**
- Modify: `README.md` / `CLAUDE.md`（僅修復本 task 發現的斷鏈）

**Interfaces:**
- Consumes: Task 1–7 全部
- Produces: 兩檔內所有相對連結均可解析

- [ ] **Step 1: 列出兩檔所有相對連結目標**

Run:
```bash
grep -oE '\]\([^)h][^)]*\)' README.md CLAUDE.md | sed 's/.*](//; s/)$//' | sort -u
```
Expected: 一份相對路徑清單（`CLAUDE.md`、`README.todo`、`docs/CHANGELOG.md`、`docs/specs/*.md`、`sample/*/README.md` 等）。

- [ ] **Step 2: 驗證每個目標存在**

Run:
```bash
grep -oE '\]\([^)h][^)]*\)' README.md CLAUDE.md \
  | sed 's/.*](//; s/)$//; s/#.*//' | sort -u \
  | while read -r f; do [ -z "$f" ] || [ -e "$f" ] || echo "BROKEN: $f"; done
```
Expected: 無 `BROKEN:` 輸出

若有斷鏈：修掉或移除該連結。特別注意 `docs/specs/` 只有 4 個檔案。

- [ ] **Step 3: 驗證外部 repo 細節確實不再出現**

Run: `grep -n "8317\|0700\|ROUTES\|/v1/messages\|openai-codex-oauth\|xai-chat" README.md CLAUDE.md`
Expected: 無輸出

- [ ] **Step 4: 驗證機器專屬路徑不再出現**

Run: `grep -n "/Users/" README.md CLAUDE.md README.todo`
Expected: 無輸出

- [ ] **Step 5: 最終行數確認**

Run: `wc -l README.md CLAUDE.md README.todo layering_test.go scripts/verify-workspace.sh`
Expected（量級，非精確值）：`README.md` ~`145`、`CLAUDE.md` ~`250`、`README.todo` ~`62`。若 `CLAUDE.md` 仍 > `320` 行，回頭檢查 Task 4/5 是否有段落漏刪。

- [ ] **Step 6: 完整測試與建置**

Run: `go test ./... 2>&1 | grep -E "FAIL|ok.*agentsdk\s" | tail -5`
Expected: 無 `FAIL`

Run: `bash scripts/verify-workspace.sh 2>&1 | tail -3`
Expected: `verify-workspace: 全部 module 通過`

- [ ] **Step 7: Commit（若 Step 2 有修復）**

```bash
git add README.md CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: fix dangling relative links after scope cleanup

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01XqVnAZGod2equMrMT3oN2p
EOF
)"
```

若 Step 2 無斷鏈，略過本步驟。

---

## 完成後的 ownership

| 檔案 | 行數 | 擁有 |
| --- | --- | --- |
| `README.md` | `312` → ~`145` | 五大支柱、tier 階梯、Getting Started、兩層 opt-in、Option 表、執行範例、規格索引 |
| `CLAUDE.md` | `478` → ~`250` | 技術基準、結構樹、技術棧、架構決策與 ownership、CLI/持久化、模組對應、開發驗證入口、慣例 |
| `layering_test.go` | 新增 ~`120` | 依賴紀律的唯一執行點 |
| `scripts/verify-workspace.sh` | 新增 ~`35` | 跨 module build/test |
| `docs/CHANGELOG.md` | `291` 不變 | 歷史（已完整，本計畫不新增） |
| `README.todo` | `58` → ~`62` | 未完成工作 + 跨 repo 約束 |

## 不在本計畫範圍

- `plans/` 目錄有 6 個 system-generated slug 檔名（`crystalline-brewing-sedgewick.md`、`modular-frolicking-key.md`、`peaceful-finding-owl.md`、`quizzical-wishing-garden.md`、`smooth-zooming-lecun.md`、`validated-meandering-rabbit.md`）與 12 個正確的 `YYYY-MM-DD-<topic>.md` 並存。不屬於本計畫的兩個目標檔，但 `CLAUDE.md` 的專案追蹤章節會指向該目錄，值得另開一個 task 處理。
- `AGENTS.md` 是否為指向 `CLAUDE.md` 的軟連結，未驗證。
- 任何 sample module 若在 Task 2 Step 3 被發現建置失敗，回報而不在本計畫修復。
