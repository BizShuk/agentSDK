# 將 `agent/spec/` 移入 `config/agent/`

## Context

`agentsdk` 目前有兩個語意命名都叫 `agent` 的 package 候選人：

- `github.com/bizshuk/agentsdk/agent` — 組裝層（assembly），把 declarative config 變成 `*runtime.Engine`
- `github.com/bizshuk/agentsdk/agent/spec` — 宣告層（declaration），`Config` schema、`Choice`、tier 展開、`Decode`/`Encode`，**只 import `core`**

宣告層目前的命名（`agent/spec`）暗示它是「組裝層的子模組」，但實際上它是個完全獨立的資料層：任何讀 / 寫設定檔的工具（wizard、schema generator、validation、web form）都不該被迫把整個組裝層拖進來。把 `spec` 移到 `config/agent/` 之後：

- `config/` namespace 變成「configuration-related」完整封面：宣告（`config/agent`）+ 組裝時的 runtime wiring（`config` 本身）
- 宣告層在依賴圖上更接近 `core`（與 wiring 同層），語意不再被「`agent` 的子目錄」誤導
- 新位置可以承接未來其他宣告層（例如 `config/session`、`config/skill`），而不需要再展開 `agent/*`

`agent/spec` 這個名字是 2026-07-22 落地 agent skeleton 時定的；本 plan 是後續的 namespace 收斂，不是 API 變更：對外型別與方法簽名不變，純粹改 import path 與 package name。

## 決策摘要 (Decisions)

| 項目                | 決策                                                                                                                                                                                                                                                                              |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| New path            | `github.com/bizshuk/agentsdk/config/agent`                                                                                                                                                                                                                                       |
| New package name    | `agent`（使用者拍板，路徑的 parent 已 disambiguate）                                                                                                                                                                                                                              |
| File names          | 沿用 `spec.go` / `choice.go` / `tier.go` / `validate.go` / `load.go` / `spec_test.go`（不為了對齊 `config/{app,default,provider}.go` 的「單 export 命名」風格而改名——會擴張 scope，且 plan 已說明這是目錄型而非檔案型的命名習慣） |
| Test package        | `agent_test`（黑盒，與現有 `spec_test` 風格一致）                                                                                                                                                                                                                                |
| Import alias        | **統一使用 `agentcfg`** —— 所有 14 個 caller 都用 `agentcfg "github.com/bizshuk/agentsdk/config/agent"`，不只 7 個衝突檔。理由：caller 看到 `agentcfg.X` 就知道是宣告層；同個 wizard package 內部 import 規則也要一致，不能「這個檔用 `agent`、那個檔用 `agentcfg`」讓 reviewer 困惑 |
| Alias 慣例鏡像      | `agentcfg` 對應既有的 `appconfig "…/config"`（samples 已在用）                                                                                                                                                                                                                   |
| Spec type aliases   | `agent/choices.go` 內 `Config = spec.Config`、`Choice`、`Model`、`Reasoning`、`Limits`、`Middleware` 等 type aliases **保留**——這是 top-level `agent` 對外 API（讓 caller 寫 `agent.Config{...}`），只是 source 從 `agent/spec` 改指 `config/agent` |
| Error prefix        | `spec:` 字串（出現在 `load.go` `Encode`/`Decode` 與 `validate.go` 錯誤訊息中）保留——這是對外錯誤契約的一部分，移動不該順便改                                                                                                                                                          |
| Breaking change     | 接受。任何外部直接 `import "github.com/bizshuk/agentsdk/agent/spec"` 的程式會編不過（internal repo only，目前無外部 caller）                                                                                                                                                            |

## 關鍵保留面 (Preserved Surfaces)

這些不變，只改 source path：

- `agent/choices.go` 的 type alias block（`Config`、`Choice`、`Model`、`Reasoning`、`Limits`、`Middleware` 等所有 `= spec.X`）
- `agent/ProviderChoices()`、`agent.LoadFile` / `SaveFile`、`agent.Main` / `agent.Run` / `agent.Once` 等組裝 API（內部使用 `spec.X`，呼叫端不變）
- `agent/spec/*.go` 內所有的 exported 型別、常數、函式簽名
- `core` 是唯一被 `config/agent` import 的內部 package（已驗證：spec 內所有檔案只 import stdlib + `core`）

## 變動清單 (Change List)

### Phase 1 — Atomic Go relocation（單一 commit）

**Move（git mv）**：

| 來源                                  | 目標                                  | package 改      |
| ------------------------------------- | ------------------------------------- | --------------- |
| `agent/spec/spec.go`                  | `config/agent/spec.go`                | `spec` → `agent` |
| `agent/spec/choice.go`               | `config/agent/choice.go`              | `spec` → `agent` |
| `agent/spec/tier.go`                 | `config/agent/tier.go`                | `spec` → `agent` |
| `agent/spec/validate.go`             | `config/agent/validate.go`           | `spec` → `agent` |
| `agent/spec/load.go`                 | `config/agent/load.go`                | `spec` → `agent` |
| `agent/spec/spec_test.go`            | `config/agent/spec_test.go`           | `spec_test` → `agent_test`；self-import 改為 `agentcfg "…/config/agent"` |

**Caller updates（14 個檔案，import path + `spec.` → `agentcfg.`）**：

| 檔案                                            | 同時 import top-level `agent`？ |
| ----------------------------------------------- | ------------------------------- |
| `agent/build.go`                                | 是（自身就是 `package agent`）   |
| `agent/build_test.go`                           | 是                                |
| `agent/load.go`                                 | 否                                |
| `agent/once.go`                                 | 否                                |
| `agent/once_test.go`                            | 是                                |
| `agent/sources.go`                              | 否                                |
| `agent/sources_test.go`                         | 是                                |
| `agent/choices.go`                              | 否（但要更新 file-level doc comment：「the definitions live in spec」改為「live in `config/agent`」）|
| `cmd/agent/wizard/wizard.go`                    | 是                                |
| `cmd/agent/wizard/helper.go`                    | 是                                |
| `cmd/agent/wizard/tui.go`                      | 否                                |
| `cmd/agent/wizard/prompt.go`                   | 否                                |
| `cmd/agent/wizard/wizard_test.go`              | 是                                |
| `sample/code-agent/cmd/compose.go`             | 是（也 import `config`、`core`、`provider`）|

**Doc string in Go source**：

- `cmd/provider.go` line 160：`"Matches agent/spec.Model.CredentialKind ..."` → `"Matches config/agent.Model.CredentialKind ..."`

### Phase 2 — Documentation sync（單一 commit，與 Phase 1 分開）

| 檔案                                                 | 改什麼                                                                                                |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `README.md`（line 15, 159, 205, 222）                | 目錄樹、layer 描述、`go list -deps ./agent/spec` 範例指令                                            |
| `CLAUDE.md`（line 7-8, 83, 121, 218, 274, 351）       | 技術脈絡、模組對應表、verification 範例                                                                |
| `docs/terminology.md`（line 5, 9-18, 27, 79）        | 術語表新增 `agentcfg` row；`agent/spec/*` 改 `config/agent/*`                                          |
| `docs/tutorials/02-agent-skeleton.md`（多處）         | `go list -deps ./agent/spec` 與 `go test ./agent/spec/...` 範例指令、`agent/spec/spec_test.go` 路徑引用 |
| `docs/architecture.svg`（line 525）                   | `agent/spec/tier.go::Expand()` 改 `config/agent/tier.go::Expand()`                                     |
| `README.todo`（line 11）                             | `M1 \`agent/spec\`：...` 改 `M1 \`config/agent\`：...`                                                |
| `docs/superpowers/plans/2026-07-23-agent-approval-resolver.md`（line 590, 901） | import path、source path 引用                                  |
| `plans/2026-07-22-agent-skeleton-config-opt-in.md`（多處） | 歷史 plan 內 path 引用                                                                     |
| `plans/validated-meandering-rabbit.md`（多處）       | 設計 plan 內 path 引用                                                                                |
| `plans/2026-07-24-round-batch-and-interactive-seam.md`（多處） | 設計 plan 內 path 引用                                                                |
| `config.example.yaml`（line 3，**untracked**）       | header comment 改 `agent/spec.Config` → `config/agent.Config`                                         |

`.claude/settings.local.json`（gitignored）內若有 `gofmt -d` 權限字串含舊 path，本機自行更新；不進 commit。`sample/*/` 內已編出的 binary（`skeleton-demo`、`file-agent/file-agent`）含舊 path 字串，跳過不動；如需清乾淨需 `go build` 重生。

## Commit split（兩 commit）

1. **`refactor: move agent spec package under config/agent`**
   - 6 個檔案 `git mv` + package name 改
   - 14 個 caller import path / qualifier 改
   - `cmd/provider.go` doc string 改
   - commit 必須 `go build ./...` + `go test ./...` 全綠

2. **`docs: update agent config package references`**
   - README、CLAUDE.md、terminology、tutorial、architecture.svg、README.todo、四份 plan、config.example.yaml
   - commit 不引入新功能，純文件同步

兩 commit 順序嚴格：Phase 2 在 Phase 1 通過驗證後再做，確保 git bisect 永遠指向可建置狀態。

## 工作流（特別注意 dirty tree）

目前 working tree 已經有未提交變更散落在 6 個將被移動的檔案內（5 個 spec 檔 + 部分 caller）。Plan agent 已 flag 這是主要操作風險：

- `git mv` 會把 dirty 變更連同檔案帶到新位置——這是預期行為，不算事故
- 確認這些 dirty 變更是否屬於先前 credential vocabulary centralization（S1636-S1637）的尾巴；若是，建議**先**把那批改動獨立 commit（commit 0），再執行本 plan 的兩個 commit，避免「move commit」夾帶其他 concern
- 不要用 `git add agent config cmd docs` 等 broad pattern；用 `git add` 明確指定檔案 + `git diff --cached --stat` 二次確認

執行順序（建議）：

```text
commit 0 (optional): 撿拾 dirty 的 credential 殘留改動
   ↓
Phase 1: git mv × 6 → 改 package name → 更新 14 caller → gofmt → 驗證 → commit 1
   ↓
Phase 2: 更新 11 個 doc 檔 → grep 二次確認 → commit 2
```

## 驗證矩陣 (Verification)

從 repo root `/Users/shuk/projects/ai/agentSDK` 執行。

### A. Format + 殘留檢查

```bash
gofmt -w \
  config/agent/spec.go config/agent/choice.go config/agent/tier.go \
  config/agent/validate.go config/agent/load.go config/agent/spec_test.go \
  agent/build.go agent/build_test.go agent/load.go agent/once.go \
  agent/once_test.go agent/sources.go agent/sources_test.go agent/choices.go \
  cmd/agent/wizard/wizard.go cmd/agent/wizard/helper.go \
  cmd/agent/wizard/tui.go cmd/agent/wizard/prompt.go \
  cmd/agent/wizard/wizard_test.go \
  sample/code-agent/cmd/compose.go

git grep -n 'github.com/bizshuk/agentsdk/agent/spec' -- '*.go'   # expect: empty
test ! -d agent/spec                                              # expect: silent
go list ./config/agent                                             # expect: github.com/bizshuk/agentsdk/config/agent
```

### B. 套件邊界證明（與 CLAUDE.md 既有的 `core ← spec` 不變式一致）

```bash
go list -deps ./config/agent | grep '^github.com/bizshuk/agentsdk'
# expect exactly:
#   github.com/bizshuk/agentsdk/core
#   github.com/bizshuk/agentsdk/config/agent
```

### C. 焦點測試（commit 前先跑，fail fast）

```bash
go test ./config/agent -count=1 -v
go test ./agent -count=1
go test ./cmd/agent/wizard -count=1
```

### D. 根模組全綠

```bash
go build ./...
go test ./... -count=1 -timeout=120s
go vet ./...
```

### E. 八個 sample module（go.work workspace）

```bash
for d in sample/code-agent sample/file-agent sample/greet-agent sample/logdoctor \
         sample/memory-demo sample/middleware-demo sample/skeleton-demo sample/strategy-demo; do
  (cd "$d" && go build ./... && go test ./... -count=1 -timeout=120s)
done
```

### F. 離線 CLI smoke（不需要 provider credentials）

```bash
go run . wizard -y --tier full -o -        # encode smoke
go run . provider --list-providers        # 純 registry 走查
```

不需要 `go run . provider "ping" --provider minimax`——那是 live integration check，與本次目錄移動無關，留給日後回歸測試。

### G. Phase 2 後 doc 同步驗證

```bash
git grep -n 'agent/spec' -- ':!*.svg' ':!*.output'   # expect: empty
grep -n 'agent/spec/tier' docs/architecture.svg       # expect: empty
```

## 風險 (Risks)

1. **Dirty tree 污染 commit**：commit 0 必須先撿拾，否則 credential 殘留會被一起搬走。驗證流程 commit 前必跑 `git diff --cached --stat`。
2. **Source-breaking import path**：外部直接 `import …/agent/spec` 的程式會壞。目前無外部 caller；若之後要 release，需在 release notes 標示 breaking。
3. **Universal `agentcfg` 與既有慣例張力**：samples 已經用 `appconfig "…/config"`，新規則 `agentcfg "…/config/agent"` 與之對稱；wizard 內若有人想用 bare `agent` 會撞名，需在 `cmd/agent/wizard/CLAUDE.md` 或 wizard.go 開頭註解說明「always use `agentcfg`」避免回歸。
4. **`.claude/settings.local.json` 含舊 path**（gitignored）：本機層級，不影響 build/commit；如本機開發流程依賴該權限字串，自行更新。
5. **sample binary 內含舊 path 字串**：`sample/file-agent/file-agent` 與 `sample/skeleton-demo/skeleton-demo` 是編譯產物，grep 會命中；驗證時排除 `*.output` 或直接 rebuild 後再 grep。
6. **`config.example.yaml` 是 untracked**：本次更新內容時**不要**順手 `git add` 它，除非明確決定要把它加入 tracked artifacts。若要 track，分開一個獨立 commit。

## 不在這次範圍內 (Out of Scope)

- 把 `spec.go` / `choice.go` / `tier.go` / `validate.go` / `load.go` 改名以對齊 `config/{app,default,provider}.go` 風格——會擴張 scope
- `spec:` error prefix 字串改寫——屬於錯誤契約變更
- 任何 `config/agent` 內部重構、新增功能
- 把其他宣告層（例如未來的 `config/session` schema）一起搬——本 plan 只處理 `spec`
- `agent/spec` 的 deprecated forwarding package——使用者接受 breaking