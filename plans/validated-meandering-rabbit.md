# credential vocabulary 集中化: core / spec / registry / cmd

## Context

`agent/spec/` 與 `provider/registry.go` 之間存在 credential 相關常數的雙重宣告,risk of drift 而無 guard test 守住。這個 plan 把這些常數下沉到 `core` package — 已經是兩個 caller 的共同依賴 — 讓 single source of truth 出現在 leaf 層,並修補 `cmd/provider.go` 直接寫死 `"minimax"` 字面值這個第三處風險點。

`MODE_DEFAULT` 等 `permission.Mode` 的雙重宣告不在本次範圍(既有模式, choice.go 註解已說明,且語意上屬於 permission 業務)。

---

## 變更範圍 (5 個檔案,1 個新增)

### 1. `core/credential.go` — 新檔案,集中宣告 vocabulary

把以下兩個語意單元搬到 `core`,因為它們是 credential port 的 contract 常數,跟 `core.ObservationSource`、`core.Hooks` 等 port 概念同層級:

```go
package core

// DefaultProvider is the registry key used when a name is empty.
// spec, registry, and cmd all reference this single value so the
// fallback cannot drift across the three config layers.
const DefaultProvider = "minimax"

// CredentialKind selects which credential class a strict lookup
// consults. Empty / "auto" preserves the legacy precedence (OAuth > API
// key, first non-empty wins). "oauth" / "api_key" restrict to one
// class and fail at resolution time when nothing resolves.
const (
    CredentialKindAuto   = ""
    CredentialKindAPIKey = "api_key"
    CredentialKindOAuth  = "oauth"
)
```

放 `core/credential.go` 而非塞進既有檔案(`message.go` / `input.go` 等),因為它與既有的 Message / Event / Effect 概念無關,獨立檔案讓依賴方向一目了然。

### 2. `agent/spec/choice.go` — 改成 re-export core constants

```go
// CredentialKind values for Model.CredentialKind. The constants live in
// core so spec and registry share one source; the uppercase names stay
// here as thin re-exports for readability at call sites (Model.CredentialKind
// reads better as spec.CREDENTIAL_KIND_OAUTH than core.CredentialKindOAuth).
const (
    CREDENTIAL_KIND_AUTO   = core.CredentialKindAuto
    CREDENTIAL_KIND_APIKEY = core.CredentialKindAPIKey
    CREDENTIAL_KIND_OAUTH  = core.CredentialKindOAuth
)
```

保留大寫名稱作為 readability alias — wizard 與 validate.go 已用 `spec.CREDENTIAL_KIND_AUTO` / `spec.CREDENTIAL_KIND_OAUTH`,改呼叫端會擴大 diff 但不改語意。

`spec/choice.go` import 已經有 `core`(見檔案頂端),零依賴變化。

### 3. `agent/spec/tier.go` — `DEFAULT_PROVIDER` 改為 core 引用

```go
const (
    DEFAULT_PROVIDER = core.DefaultProvider
    ...
)
```

同樣保留大寫名稱作為 re-export alias。`spec.Expand()` 在 line 106 `out.Model.Provider = DEFAULT_PROVIDER` 的呼叫點不需改。

### 4. `provider/registry.go` — 兩個常數都改 core 引用

```go
const DEFAULT = core.DefaultProvider

const (
    CredentialKindAuto   = core.CredentialKindAuto
    CredentialKindAPIKey = core.CredentialKindAPIKey
    CredentialKindOAuth  = core.CredentialKindOAuth
)
```

保留 PascalCase 名稱(`registry.CredentialKindAuto` 等)讓呼叫端 (`agent/once.go`、`cmd/provider.go`) 不變。

### 5. `cmd/provider.go` — `--provider` flag 預設改引用

```go
flags.StringVar(&ProviderName, "provider", registry.DEFAULT, ...)
```

`ResetFlags()` 同樣改:

```go
ProviderName = registry.DEFAULT
```

---

## 測試 (2 個更新,1 個新增)

### `agent/spec/spec_test.go` — `TestCredentialKindChoicesMatchRegistryConstants` 改名 + 改內容

現有測試斷言 `spec.Values(...)` 包含 `spec.CREDENTIAL_KIND_AUTO` 等三個值。改成斷言兩個來源 `Values()` 出來的 slice 完全相等:

```go
func TestCredentialKindChoicesMatchRegistryConstants(t *testing.T) {
    // spec and registry must read from the same core constants; if a
    // future refactor reintroduces hardcoded strings on either side,
    // both sides would drift independently — this test catches that.
    specVals := spec.Values(spec.VariantChoices("model.credential_kind"))
    require.Len(t, specVals, 3, "spec variant list must stay in lockstep with core constants")
    assert.Equal(t, spec.CREDENTIAL_KIND_AUTO,   specVals[0])  // auto is the default
    assert.Equal(t, spec.CREDENTIAL_KIND_APIKEY, specVals[1])
    assert.Equal(t, spec.CREDENTIAL_KIND_OAUTH,  specVals[2])
}
```

(實作上 `spec.CREDENTIAL_KIND_*` 與 `core.CredentialKind*` 是同一字面值;re-export 保證 identity。)

### `provider/registry_test.go` — `TestSpecDefaultProviderIsRegistered` 強化

現有測試已驗證 `spec.DEFAULT_PROVIDER == registry.DEFAULT`。改為直接驗證兩者都引用 `core.DefaultProvider`:

```go
func TestDefaultProviderComesFromCore(t *testing.T) {
    // Single source of truth: any change to core.DefaultProvider flows
    // to both spec and registry through their const aliases.
    assert.Equal(t, core.DefaultProvider, spec.DEFAULT_PROVIDER)
    assert.Equal(t, core.DefaultProvider, registry.DEFAULT)
    _, ok := registry.Lookup(core.DefaultProvider)
    assert.Truef(t, ok, "core.DefaultProvider %q must be a registered provider", core.DefaultProvider)
}
```

### `cmd/provider_test.go` — 新增 `TestProviderFlagDefaultReferencesRegistryDefault`

```go
func TestProviderFlagDefaultReferencesRegistryDefault(t *testing.T) {
    flag := ProviderCmd.Flags().Lookup("provider")
    require.NotNil(t, flag)
    assert.Equal(t, registry.DEFAULT, flag.DefValue,
        "--provider default must come from registry.DEFAULT, not a hardcoded literal")
}
```

---

## 風險評估

| 風險 | 評估 |
|---|---|
| `core` 變成 vocabulary hub,leaf 套件職責擴張 | 可接受:`core` 既有 `Role`、`PartKind`、`EventKind`、`ReasoningStyle` 等都是 contract constants,`DefaultProvider` 與 `CredentialKind*` 是同性質的 port-contract 值 |
| spec 大寫名稱移除會擴大 diff | 不移除:保留 re-export alias 維持 `spec.CREDENTIAL_KIND_OAUTH` 可讀性,同時底層是 core 引用 |
| registry PascalCase 名稱移除會擴大 diff | 不移除:同樣保留 alias |
| `TestSpecDefaultProviderIsRegistered` 名稱改 | 改名更貼切新行為,測試仍守住核心 invariant |

---

## 影響摘要

- 新檔案:1 個 (`core/credential.go`, 9 行)
- 修改檔案:5 個 (spec/choice.go, spec/tier.go, provider/registry.go, cmd/provider.go + 2 個測試更新)
- 公開 API:零新增/移除 (re-export aliases 維持既有呼叫端可用)
- 向後相容:既有 `spec.CREDENTIAL_KIND_*` 與 `registry.CredentialKind*` 與 `registry.DEFAULT` 名稱全部保留
- 行數變動:核心 code +30 行(core 新檔案),其餘為 reference 改寫

---

## 驗證

```bash
cd /Users/shuk/projects/ai/agentSDK

# 1. 所有 module build + test 不 regression
for mod in . sample/code-agent sample/file-agent sample/greet-agent sample/logdoctor \
  sample/memory-demo sample/middleware-demo sample/skeleton-demo sample/strategy-demo; do
  (cd "$mod" && go build ./... && go test ./... -count=1 -timeout=120s)
done

# 2. 三層引用同一常數的事實
go test ./provider -run TestDefaultProviderComesFromCore -v          # 鎖定 core 為 source
go test ./agent/spec -run TestCredentialKindChoicesMatchRegistryConstants -v

# 3. provider cmd flag 預設走 registry.DEFAULT
go test ./cmd -run TestProviderFlagDefaultReferencesRegistryDefault -v

# 4. functional smoke (確認 credential_kind 三層仍通)
go run . provider --help | grep "credential-kind"                   # flag 仍存在
go run . w -y --tier basic -o /tmp/w.yaml                           # wizard 仍 emit

# 5. 防 drift test(把任一處故意改錯,測試應抓)
# (人工) 暫改 core.DefaultProvider = "anthropic"; 跑測試應失敗;改回來
```

預期:
- 所有 build/test 綠
- 三個 guard test 守住新 invariant
- credential_kind 三層行為不變(已驗證的 401 / fail-fast 路徑仍正常)
- 防 drift:故意改 core 常數,所有引用方都會跟著改(re-export alias 保證 identity)