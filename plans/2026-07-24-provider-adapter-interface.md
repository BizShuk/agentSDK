# Provider Adapter Interface

Date: 2026-07-24
Status: completed on 2026-07-24

## Context

`provider/` 目前以 `Entry` struct literal 形式收集 adapter 註冊資訊。`Entry` 同時持有兩種資料：runtime-facing 欄位（`New`、`Catalog`）與註冊 metadata（`Name`、`Label`、`Note`、`APIKeyEnv`、`BaseURLEnv`）。後 5 個欄位其實是「描述 adapter 本身」的事，卻散落在 registry 端；7 個 adapter 各自實作了相同的 6 個 `core.Provider` 方法加上 `Name()`，但 `Name()` 並不在 `core.Provider` 內。

目標：

1. 在 `provider/` 內定義新 interface `Adapter`，把 `core.Provider` + `Name()` + 一個新的 `Metadata()` 方法收進來，作為所有 adapter 統一的編譯期契約。
2. 定義 `Metadata` struct 承接 `Entry` 原本的 `Label/Note/APIKeyEnv/BaseURLEnv` 四欄位。
3. `Entry` 改為薄殼（只剩 `Name/Metadata/New/Catalog`），`Factory` 回傳型別由 `core.Provider` 改為 `registry.Adapter`，型別系統強制每個 adapter 同時滿足兩者。
4. 每個 adapter 以 package-level `adapterMetadata()` 函式作為 single source of truth，直接被 `Entry.Metadata` 與 `Provider.Metadata()` 引用，無 struct 欄位、無 withMeta option。
5. `Options.Resolve` 簽章由 `(e Entry)` 收緊為 `(m Metadata)`，credential resolution 只讀 metadata。

## 設計

### 新檔 `provider/adapter.go`

```go
package registry

import "github.com/bizshuk/agentsdk/core"

// Adapter 是 adapter 對 registry 與 runtime 的完整契約:runtime-facing
// core.Provider + Name() + 註冊 metadata。每個 provider/<name>/provider.go
// 都用 `var _ registry.Adapter = (*Provider)(nil)` 編譯期掛保證。
type Adapter interface {
    core.Provider

    // Name 回傳 "<family>:<model>"——給 log 與 wire-format metadata 用。
    // family 本身由 ID() 提供。
    Name() string

    // Metadata 是該 adapter 的註冊描述:Label 顯示於 CLI 選單,
    // Note 解釋 credential 模型,APIKeyEnv 是 credential env 變數
    // (高優先序在前),BaseURLEnv 是 endpoint 覆寫 env 變數
    // (空字串代表走 adapter 內建預設)。
    Metadata() Metadata
}

type Metadata struct {
    Label      string
    Note       string
    APIKeyEnv  []string
    BaseURLEnv string
}
```

### `provider/registry.go` 變更

- `Entry` 結構精簡為 `Name / Metadata / New / Catalog`，刪除 `Label / Note / APIKeyEnv / BaseURLEnv`。
- `Factory` 簽名由 `func(Options) (core.Provider, error)` 改為 `func(Options) (Adapter, error)`。
- `New()` 回傳型別同步改為 `Adapter`。Callers 若需要 `core.Provider` 的 sub-set，`Adapter` 內嵌 `core.Provider` 可直接傳遞；需要 `core.ModelLister` 的動態型別斷言仍可運作（runtime concrete type 仍是 `*anthropic.Provider` 等具體型別）。
- `Options.Resolve(e Entry)` 改為 `Options.Resolve(m Metadata)`,內部讀 `m.APIKeyEnv` 與 `m.BaseURLEnv`。
- `Lookup` / `Entries` / `Catalog` 行為不變,只是 `Entry` 欄位換位置。

### 每個 adapter 的 pattern (7 個一致)

`provider/<name>/register.go` (範例: anthropic):

```go
package anthropic

import (
    "github.com/bizshuk/agentsdk/provider"
)

// adapterMetadata 是 anthropic adapter 對外的 single source of truth。
// 同時被 Entry.Metadata 與 Provider.Metadata() 引用,保證兩處不 drift。
// 函式 (非變數) 是為了每次回傳新的 slice,避免外部突改 APIKeyEnv。
func adapterMetadata() registry.Metadata {
    return registry.Metadata{
        Label:     "Anthropic",
        Note:      "OAuth token outranks API key",
        APIKeyEnv: []string{"ANTHROPIC_OAUTH_TOKEN", "ANTHROPIC_API_KEY"},
    }
}

// Compile-time: ensure *Provider satisfies registry.Adapter.
var _ registry.Adapter = (*Provider)(nil)

func init() {
    meta := adapterMetadata()
    registry.Register(registry.Entry{
        Name:     "anthropic",
        Metadata: meta,
        New: func(o registry.Options) (registry.Adapter, error) {
            var opts []Option
            if o.Model != "" {
                opts = append(opts, WithModel(o.Model))
            }
            if o.APIKey != "" {
                opts = append(opts, WithAPIKey(o.APIKey))
            }
            if o.BaseURL != "" {
                opts = append(opts, WithBaseURL(o.BaseURL))
            }
            return New(opts...)
        },
        Catalog: DefaultCatalog,
    })
}
```

`provider/<name>/provider.go` 新增方法:

```go
// Metadata 實作 registry.Adapter:回傳 package-level adapterMetadata()
// 的當下值,讓 direct constructors (New/NewWithOAuth) 與 registry.Factory
// 構造出的 adapter 都帶著一致的 metadata。
func (p *Provider) Metadata() registry.Metadata { return adapterMetadata() }
```

並把現有的 `var _ core.Provider = (*Provider)(nil)` 移除(被 `register.go` 裡的 `var _ registry.Adapter = (*Provider)(nil)` 涵蓋)。

OAuth adapter (`codex` / `antigravity` / `grok`) 因為有 `NewWithOAuth`,`New` 的 closure 仍建出 API-key 路徑 Provider,`Metadata()` 不論走哪條路徑都回傳相同靜態值。OAuth 認證細節保持 orthogonal,本介面不擴張。

## 修改檔案清單

### 新增
- `provider/adapter.go` — `Adapter` interface + `Metadata` struct

### 修改 — registry 層
- `provider/registry.go` — `Entry` 精簡、`Factory`/`New` 回傳 `Adapter`、`Options.Resolve` 簽章改吃 `Metadata`
- `provider/registry_test.go` — `TestEveryEntryIsSelfDescribing` 改讀 `e.Metadata.*`、`TestCredentialResolutionPrecedence` 改呼叫 `o.Resolve(e.Metadata)`

### 修改 — 7 個 adapter (pattern 一致,逐個套用)
- `provider/anthropic/{provider,register}.go`
- `provider/antigravity/{provider,register}.go`
- `provider/codex/{provider,register}.go`
- `provider/google/{provider,register}.go`
- `provider/grok/{provider,register}.go`
- `provider/minimax/{provider,register}.go`
- `provider/ollama/{provider,register}.go`

### 修改 — call sites
- `agent/choices.go` — `providerNote(e)` 改讀 `e.Metadata.Note / e.Metadata.APIKeyEnv`;`ProviderChoices()` 改讀 `e.Metadata.Label`
- `agent/once_test.go` — 不動 (觀察 `Choice.Label`/`Choice.Note`,經 `choices.go` 轉發)
- `cmd/agent/wizard/providers_test.go` — 不動 (只有 blank import)
- `cmd/provider.go` — `effectiveModel` 不動 (local `named` 斷言繼續可用)

## 風險與取捨

1. **Factory signature 是 source-breaking**:Go function literal 回傳型別不可變,所有 7 個 register.go 的 closure 都要更新;`registry_test.go` 兩個測試 (`TestRegisterPanicsOnDuplicate`、`TestRegisterRejectsIncompleteEntry`) 的 inline factory 也必須改寫。
2. **OAuth resolution lossy bug** (`ANTHROPIC_OAUTH_TOKEN` 走 API-key `New`):屬本次範圍外,保留 registry 既有行為;follow-up 應讓 `Entry.New` 根據 credential 種類路由到對應 constructor (`New` 或 `NewWithOAuth`)。
3. **`registry.Catalog(name)` 仍保留為 pre-construction factory**:有些 adapter (`minimax`) 構造時需要 API key,無法靠 `Adapter` 拿到 catalog。
4. **slice 防禦性複製**: `adapterMetadata()` 是函式而非變數,每次回傳新 slice,避免 caller mutate `Metadata.APIKeyEnv` 影響後續註冊。
5. **`Entry.Name` vs `Adapter.Name()` 的語意差異**: `Entry.Name` 是 family (如 `"anthropic"`);`Adapter.Name()` 是 instance-qualified (如 `"anthropic:claude-sonnet-5"`);`Adapter.ID()` 與 `Entry.Name` 必須相等(契約)。

## 驗證

```bash
cd "$(git rev-parse --show-toplevel)"

# 1. 全 workspace build 與 vet
go work sync
go build ./... && go vet ./...

# 2. provider 套件測試
go test ./provider/... -count=1 -timeout=120s

# 3. agent / cmd 測試 (ProviderChoices 路徑)
go test ./agent/... ./cmd/... -count=1 -timeout=120s

# 4. 7 個 sample module 都需重 build + test
for mod in . sample/code-agent sample/file-agent sample/greet-agent \
  sample/log-agent-v2 sample/logdoctor-agent sample/demo-memory sample/demo-middleware \
  sample/skeleton-agent sample/demo-strategy; do
  (cd "$mod" && go build ./... && go test ./... -count=1 -timeout=120s)
done

# 5. CLI 煙霧測試:registry 列出 + Adapter 構造 + Metadata 回傳
go run . provider --list-providers
go run . provider "ping" --provider minimax
go run . provider --list-models --provider google

# 6. wizard 路徑:ProviderChoices 從 e.Metadata 讀 Label/Note/APIKeyEnv
go run . w -y --tier oneshot -o /tmp/agent.yaml
grep -A2 'model:' /tmp/agent.yaml    # 確認 Note 仍出現
```

預期:

- `go build ./...` 全部通過;7 個 register.go 與 registry_test.go 沒有 stale field access。
- `go test ./provider/...` 通過:`TestEveryEntryIsSelfDescribing` 改讀 `e.Metadata.*` 後斷言仍命中。
- `go test ./agent/...` 通過:`ProviderChoices` 仍能給 wizard 提供含 Label + Note 的 Choice。
- `provider --list-providers` 印出 7 個名稱;`provider --list-models --provider google` 印出 google catalog。
- `wizard --tier oneshot` 產出的 YAML 內 `model.provider.note` 仍含 credential 提示。
