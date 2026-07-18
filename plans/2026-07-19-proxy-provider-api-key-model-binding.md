# Proxy Provider、API Key 與 Model List 綁定 Implementation Plan

> `For agentic workers:` 執行時必須使用 `executing-plans` skill，逐個 task 完成 `RED → GREEN → review`。本檔只定義計畫，不授權目前 session 實作、啟動服務、commit 或 push。

`目標 (Goal)：` Proxy 以 downstream client API key 唯一選定一個 upstream provider，再以該 provider 的 model list 對 request model 做精確授權；不再使用 model 名稱選 provider，也不實作 fallback。

`架構 (Architecture)：` 在獨立 `proxy` Go module 內新增 `proxy/providers/`，由 ordered provider catalog 編譯 `client API key → provider binding → allowed models`。Gin middleware 驗證 API key 並把 binding 放入 request context；handler 只從 binding 取得 provider、target protocol 與 models，沿用既有 `transform.Registry`、credential resolver 及安全 HTTP transport。

`技術棧 (Tech Stack)：` Go `1.26`、Gin、Viper/gosdk config、既有 `proxy/protocol`、`proxy/transform`、`proxy/upstream`、`auth/svc.Resolver`、testify。

## 全域限制 (Global Constraints)

- 範圍只限 `proxy` layer 與其 canonical docs；不得修改 root runtime 的 `core.ModelProvider`、`provider/*` Agent runtime adapters、收據腳本或 `pm2`。
- `providers` 是 ordered list；同一個 client API key 重複出現時，第一個 provider 生效，後續 binding 忽略並輸出不含 raw secret 的 warning。
- 一個 request 只允許一次 provider selection 與一次 upstream attempt；不得加入 fallback、retry、load balancing、model alias 或由 model 名稱推導 provider。
- model authorization 使用完整字串、大小寫敏感的 exact match；不得移除 prefix，也必須接受包含 `/`、`:` 的 Ollama model name。
- `client-api-keys` 是呼叫 proxy 的 downstream keys；OpenAI、Anthropic、MiniMax、xAI 的 upstream secrets 仍由既有 auth store 與 active credential resolver 管理，兩者不得混用。
- Ollama 直接使用 OpenAI-compatible `POST /v1/chat/completions`，不得 import `agentSDK/provider/openaicompat`，不得要求 upstream credential，只允許 loopback HTTP 或 HTTPS base URL。
- 保留既有 Anthropic、MiniMax、OpenAI API key、OpenAI Codex OAuth 與 xAI protocol behavior，包含 header allowlist、Codex normalization、SSE bridge、xAI tool 限制與 native count-tokens 規則。
- raw downstream/upstream API key、Authorization header、prompt、tool output 與 upstream error body 不得寫入一般 log。
- 目前 worktree 有既存 module-split 變更；執行本計畫前必須先確認 baseline 狀態。不得用 `git reset --hard`、`git checkout --` 或整檔覆蓋清除既有變更。
- 本計畫不包含 commit；若後續需要 commit，必須由使用者另行授權並只 stage 本功能檔案。

## 設定契約 (Configuration Contract)

Canonical 設定放在 gosdk 載入的 `~/.config/agentSDK/settings.local.json`：

```json
{
  "providers": [
    {
      "provider": "openai",
      "protocol": "openai-responses",
      "auth": "active",
      "client-api-keys": ["proxy-openai-1", "proxy-openai-2"],
      "models": ["gpt-5.4", "gpt-5.4-mini"]
    },
    {
      "provider": "anthropic",
      "protocol": "anthropic-messages",
      "auth": "active",
      "client-api-keys": ["proxy-anthropic-1"],
      "models": ["claude-sonnet-4-5", "claude-opus-4-1"]
    },
    {
      "provider": "ollama",
      "protocol": "openai-chat",
      "auth": "none",
      "base-url": "http://127.0.0.1:11434",
      "client-api-keys": ["proxy-ollama-1"],
      "models": ["qwen2.5:7b", "z-uo/qwen2.5vl_tools:7b"]
    }
  ]
}
```

設定語意固定如下：

| 欄位 | 規則 |
| --- | --- |
| `provider` | 小寫 provider family；v1 支援 `anthropic`、`minimax`、`openai`、`xai`、`ollama`；同一 provider 不得重複宣告。 |
| `protocol` | 該 binding 的唯一 upstream target protocol：`anthropic-messages`、`openai-chat` 或 `openai-responses`。 |
| `auth` | credential provider 使用 `active`；Ollama 使用 `none`。省略時依 driver 預設，不提供任意自訂 auth mode。 |
| `base-url` | credential provider 可省略並使用官方 default；Ollama default 為 `http://127.0.0.1:11434`。 |
| `client-api-keys` | 一個 provider 可綁多把 downstream keys；空白或空 list 是 startup error。 |
| `models` | 該 provider 允許的完整 model IDs；空白、空 list 或同 provider 內重複 model 是 startup error。 |

舊 top-level `api-keys` 不再有 provider/model binding，因此 startup 必須明確拒絕並回報 migration message，不再自動產生 global key。

## 檔案結構 (File Map)

```text
proxy/
├── providers/
│   ├── provider.go                 # Config、Driver、Binding contracts
│   ├── catalog.go                  # ordered key index、model authorization、shadow warnings
│   ├── catalog_test.go
│   ├── internal/shared/profile.go  # 共用 header lists、preserve normalizer、error helpers
│   ├── anthropic/anthropic.go      # Anthropic Messages driver
│   ├── minimax/minimax.go          # MiniMax Anthropic-compatible driver
│   ├── openai/openai.go            # OpenAI API key / Codex OAuth driver
│   ├── ollama/ollama.go            # loopback OpenAI Chat driver、no auth
│   ├── xai/xai.go                  # xAI Responses / Chat driver
│   └── registry/registry.go        # production driver set + catalog constructor
├── proxy/
│   ├── config.go                   # providers config、legacy api-keys rejection
│   ├── middleware.go               # client key → request binding
│   ├── handler.go                  # binding → model check → transform → upstream
│   └── server.go                   # registry/catalog composition root
└── upstream/
    ├── profile.go                  # provider-neutral transport profile only
    └── client.go                   # explicit no-auth transport support
```

`proxy/route/` 可暫時保留作 historical/test compatibility，但 production `server.go` 與 `handler.go` 完成後不得 import 或呼叫 `route.Router`。`upstream.DefaultCatalog()` 若仍需保留給 migration tests，必須標示 legacy 且不得出現在 production wiring；較佳落點是把 concrete construction 與 normalizer tests完整移至各 driver 後刪除它。

---

### Task 1：建立 ordered provider config 與 access catalog

`Files：`

- Create: `proxy/providers/provider.go`
- Create: `proxy/providers/catalog.go`
- Create: `proxy/providers/catalog_test.go`
- Modify: `proxy/proxy/config.go`
- Create: `proxy/proxy/config_test.go`

`Interfaces：`

```go
package providers

type Config struct {
	Provider      string          `json:"provider" mapstructure:"provider"`
	Protocol      protocol.Format `json:"protocol" mapstructure:"protocol"`
	Auth          string          `json:"auth,omitempty" mapstructure:"auth"`
	BaseURL       string          `json:"base-url,omitempty" mapstructure:"base-url"`
	ClientAPIKeys []string        `json:"client-api-keys" mapstructure:"client-api-keys"`
	Models        []string        `json:"models" mapstructure:"models"`
}

type Driver interface {
	ID() string
	Validate(Config) error
	RequiresCredential() bool
	Profile(Config, *model.Credential) (upstream.Profile, error)
}

type Binding struct {
	provider string
	protocol protocol.Format
	config   Config // ClientAPIKeys is cleared before storage.
	driver   Driver
	models   map[string]struct{}
	ordered  []string
}

func (b Binding) Provider() string
func (b Binding) Protocol() protocol.Format
func (b Binding) RequiresCredential() bool
func (b Binding) AllowsModel(model string) bool
func (b Binding) Models() []string
func (b Binding) Profile(*model.Credential) (upstream.Profile, error)

type ShadowedKey struct {
	Fingerprint      string
	WinnerProvider   string
	ShadowedProvider string
}

func NewCatalog(configs []Config, drivers []Driver) (*Catalog, []ShadowedKey, error)
func (c *Catalog) ResolveAPIKey(rawKey string) (Binding, bool)
```

- [ ] `Step 1.1` 在 `catalog_test.go` 先建立 fake driver，寫下列 failing tests：

```go
func TestNewCatalogUsesFirstProviderForDuplicateAPIKey(t *testing.T)
func TestCatalogModelAuthorizationIsExactAndCaseSensitive(t *testing.T)
func TestCatalogAllowsSlashAndColonInModelID(t *testing.T)
func TestCatalogNeverExposesRawKeyInShadowWarning(t *testing.T)
func TestNewCatalogRejectsEmptyKeysModelsAndDuplicateProvider(t *testing.T)
func TestNewCatalogRejectsUnsupportedProtocol(t *testing.T)
```

測試資料必須包含相同 key 同時出現在 OpenAI 與 Ollama，assert winner 是設定順序第一個 provider；fingerprint 只能是 SHA-256 digest 的固定前綴，不能含 raw key。

- [ ] `Step 1.2` 執行 RED：

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./providers -run 'Test(NewCatalog|Catalog)' -count=1
```

Expected: compile failure，因 `providers.Config`、`Catalog` 與 `Binding` 尚未存在。

- [ ] `Step 1.3` 實作 catalog：trim provider/key/configured model 外圍空白；provider 正規化成小寫；model map 保留大小寫；key 只以 `sha256.Sum256` digest 作 map key；遇到 duplicate digest 時保留既有 binding 並追加一筆 `ShadowedKey`。建立 binding 前必須把 internal config copy 的 `ClientAPIKeys` 設為 `nil`，讓 catalog 完成建索引後不再持有 raw downstream keys。

核心 first-wins loop 必須等價於：

```go
digest := sha256.Sum256([]byte(key))
if winner, exists := catalog.bindings[digest]; exists {
	shadows = append(shadows, ShadowedKey{
		Fingerprint:      hex.EncodeToString(digest[:6]),
		WinnerProvider:   winner.Provider(),
		ShadowedProvider: config.Provider,
	})
	continue
}
catalog.bindings[digest] = binding
```

- [ ] `Step 1.4` 修改 `proxy/proxy/config.go`：以 `Providers []providers.Config` 取代 runtime `APIKeys`；移除 `ensureAPIKey()` 與 `APIKeySet()`；`LoadConfig()` 在 unmarshal 前後同時檢查 `viper.IsSet("api-keys")` 與 `len(Providers) > 0`，讓舊 key 即使設定成空 list 仍得到 migration error。

錯誤文字固定包含：

```text
proxy config: legacy api-keys is not supported; use providers[].client-api-keys
proxy config: providers is required
```

- [ ] `Step 1.5` 在 `proxy/proxy/config_test.go` 寫並跑：

```go
func TestLoadConfigRejectsLegacyAPIKeys(t *testing.T)
func TestProviderConfigRequiresProviders(t *testing.T)
func TestProviderConfigPreservesProviderOrder(t *testing.T)
```

測試必須在每個 case 呼叫 `viper.Reset()`，避免 global viper state 污染。

- [ ] `Step 1.6` 執行 GREEN：

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./providers ./proxy -run 'Test(NewCatalog|Catalog|LoadConfig|ProviderConfig)' -count=1
```

Expected: PASS；review catalog clone 行為，確認 caller 無法修改 binding 內部 models/config slices。

---

### Task 2：建立 protocol-based provider drivers 與 Ollama no-auth transport

`Files：`

- Create: `proxy/providers/internal/shared/profile.go`
- Create/Test: `proxy/providers/anthropic/anthropic.go`、`anthropic_test.go`
- Create/Test: `proxy/providers/minimax/minimax.go`、`minimax_test.go`
- Create/Test: `proxy/providers/openai/openai.go`、`openai_test.go`
- Create/Test: `proxy/providers/ollama/ollama.go`、`ollama_test.go`
- Create/Test: `proxy/providers/xai/xai.go`、`xai_test.go`
- Create/Test: `proxy/providers/registry/registry.go`、`registry_test.go`
- Modify/Test: `proxy/upstream/profile.go`、`profile_test.go`
- Modify/Test: `proxy/upstream/client.go`、`client_test.go`

`Driver matrix：`

| Driver | Allowed protocol | Default base URL | Endpoint | Credential |
| --- | --- | --- | --- | --- |
| `anthropic` | `anthropic-messages` | `https://api.anthropic.com` | `/v1/messages` | active API key/OAuth |
| `minimax` | `anthropic-messages` | `https://api.minimax.io/anthropic` | `/v1/messages` | active API key |
| `openai` | `openai-responses`、`openai-chat` | `https://api.openai.com` | `/v1/responses` 或 `/v1/chat/completions` | active API key；OAuth 只允許 Codex Responses |
| `xai` | `openai-responses`、`openai-chat` | `https://api.x.ai` | `/v1/responses` 或 `/v1/chat/completions` | active API key/OAuth |
| `ollama` | `openai-chat` | `http://127.0.0.1:11434` | `/v1/chat/completions` | none |

- [ ] `Step 2.1` 先寫 driver failing tests，名稱至少包含：

```go
func TestDriverRejectsUnsupportedProtocol(t *testing.T)
func TestDriverBuildsConfiguredProfile(t *testing.T)
func TestOpenAIDriverSelectsAPIKeyOrCodexOAuthProfile(t *testing.T)
func TestOpenAICodexNormalizerPreservesRequiredContract(t *testing.T)
func TestXAIDriverRejectsNonFunctionTools(t *testing.T)
func TestOllamaDriverRejectsNonLoopbackHTTP(t *testing.T)
func TestOllamaDriverAcceptsLoopbackIPv4IPv6AndLocalhost(t *testing.T)
func TestRegistryInstallsEverySupportedDriver(t *testing.T)
```

Codex test必須 assert `stream=true`、`store=false`、system/developer messages 被移入 `instructions`，以及 non-stream client 會設定 `BridgeToNonStream=true`。

- [ ] `Step 2.2` 在 `upstream/client_test.go` 先增加：

```go
func TestClientDoWithoutCredentialForNoAuthProfile(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "/v1/chat/completions", req.URL.Path)
		assert.Empty(t, req.Header.Get("Authorization"))
		assert.Empty(t, req.Header.Get("x-api-key"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"choices":[]}`)),
			Request:    req,
		}, nil
	})
	client, err := NewClient(&http.Client{Transport: transport}, timeoutConfig())
	require.NoError(t, err)
	profile := Profile{
		ID: "ollama", BaseURL: "http://127.0.0.1:11434",
		Endpoints: map[protocol.Format]string{
			protocol.FORMAT_OPENAI_CHAT: "/v1/chat/completions",
		},
		Preferred: protocol.FORMAT_OPENAI_CHAT,
		AuthScheme: AUTH_NONE,
		NormalizeRequest: preserveRequest,
	}
	response, err := client.Do(context.Background(), profile, nil, protocol.RequestEnvelope{
		TargetFormat: protocol.FORMAT_OPENAI_CHAT,
		Body: []byte(`{"model":"qwen2.5:7b","messages":[]}`),
	})
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
}
```

- [ ] `Step 2.3` 執行 RED：

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./providers/... ./upstream -run 'Test(Driver|OpenAI|XAI|Ollama|Registry|ClientDoWithoutCredential)' -count=1
```

Expected: compile/test failure，因 driver packages 與 `AUTH_NONE` 尚未存在。

- [ ] `Step 2.4` 將 `upstream.Profile` 維持 provider-neutral，新增 `AUTH_NONE AuthScheme = "none"`。`Client.do()` 只有在 auth scheme 非 `AUTH_NONE` 時呼叫 `validateCredentialForProfile`，只有 `cred != nil` 時才套用 credential `BaseURL`，`applyProviderHeaders()` 對 `AUTH_NONE` 直接 return。

必須保留兩層 URL 防線：driver startup validation 拒絕 `http://` 非 loopback URL，`upstream.buildEndpointURL()` 仍在每次 request 拒絕 userinfo、query、fragment、redirect 與不安全 scheme。

- [ ] `Step 2.5` 把 concrete provider construction 移到 driver packages：

```go
type Driver struct{}

func (Driver) ID() string
func (Driver) Validate(providers.Config) error
func (Driver) RequiresCredential() bool
func (Driver) Profile(providers.Config, *model.Credential) (upstream.Profile, error)
```

`internal/shared/profile.go` 只放跨 provider 的 default request/response header lists、slice clone 與 preserve-request normalizer；Codex normalizer 必須留在 `providers/openai`，xAI tool validation 必須留在 `providers/xai`，不得把 concrete rules 留在 generic transport。

- [ ] `Step 2.6` 實作 `providers/registry.New`：

```go
func New(configs []providers.Config) (*providers.Catalog, []providers.ShadowedKey, error) {
	return providers.NewCatalog(configs, []providers.Driver{
		anthropic.Driver{},
		minimax.Driver{},
		openai.Driver{},
		xai.Driver{},
		ollama.Driver{},
	})
}
```

- [ ] `Step 2.7` 執行 GREEN：

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./providers/... ./upstream -count=1
```

Expected: PASS；review import graph，確認 `proxy/providers/ollama` 沒有 import root `provider/openaicompat` 或任何 Agent runtime package。

---

### Task 3：讓 middleware 以 API key 選定 binding

`Files：`

- Modify: `proxy/proxy/middleware.go`
- Modify: `proxy/proxy/middleware_test.go`

`Interfaces：`

```go
func requireAPIKey(catalog *providers.Catalog) gin.HandlerFunc
func providerBinding(c *gin.Context) (providers.Binding, bool)
```

- [ ] `Step 3.1` 先改寫 middleware tests，使用 fake driver 建 catalog，覆蓋：

```go
func TestAPIKeyMiddlewareStoresSelectedProviderBinding(t *testing.T)
func TestAPIKeyMiddlewareUsesXAPIKeyBeforeBearer(t *testing.T)
func TestAPIKeyMiddlewareReturns401ForMissingKey(t *testing.T)
func TestAPIKeyMiddlewareReturns403ForUnknownKey(t *testing.T)
func TestAPIKeyMiddlewareNeverLogsRawKey(t *testing.T)
```

`x-api-key` precedence test 必須提供 invalid `x-api-key` 與 valid Bearer，預期 `403`，避免第二個 header 繞過第一個 header 的明確選擇。

- [ ] `Step 3.2` 執行 RED：

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./proxy -run 'TestAPIKeyMiddleware' -count=1
```

Expected: compile/test failure，因 middleware 仍接收 global key set，且 context 沒有 binding。

- [ ] `Step 3.3` 移除 `validKey(map[string]struct{}, key)`；保留既有 `extractAPIKey` header precedence；呼叫 `catalog.ResolveAPIKey` 後用 private context key 保存 immutable binding。missing key 維持 `401`，unknown key 維持 `403`；log 只能包含 reason、IP、path 與可選 fingerprint，不得包含輸入 key。

- [ ] `Step 3.4` 執行 GREEN：

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./proxy -run 'TestAPIKeyMiddleware' -count=1
```

Expected: PASS。

---

### Task 4：handler 與 server 改為 binding-only provider flow

`Files：`

- Modify: `proxy/proxy/handler.go`
- Modify: `proxy/proxy/handler_test.go`
- Modify: `proxy/proxy/server.go`
- Modify: `proxy/proxy/server_test.go`

`Handler dependency contract：`

```go
type HandlerDeps struct {
	Registry     *transform.Registry
	Providers    *providers.Catalog
	Credentials *upstream.CredentialResolver
	Client       *upstream.Client
	Observer     TransformObserver
	MaxBodyBytes int64
}
```

`route.Router` 與 `upstream.Catalog` 必須從 production `Handler`/`HandlerDeps` 移除。

- [ ] `Step 4.1` 先增加 handler failing tests：

```go
func TestHandlerUsesAPIKeyBindingInsteadOfModelRouting(t *testing.T)
func TestHandlerRejectsModelOutsideBindingWith403(t *testing.T)
func TestHandlerPassesSlashModelUpstreamUnchanged(t *testing.T)
func TestHandlerTransformsEachClientFormatToConfiguredProtocol(t *testing.T)
func TestHandlerModelsReturnsOnlySelectedBindingModels(t *testing.T)
func TestHandlerCountTokensUsesSelectedBinding(t *testing.T)
func TestHandlerDoesNotFallbackAfter429Or5xx(t *testing.T)
func TestHandlerDoesNotFallbackAfterTransportTimeout(t *testing.T)
```

protocol matrix 測試至少覆蓋三種 source endpoint 對三種 configured target protocol，沿用 `transform.NewDefaultRegistry()`；upstream fake 必須記錄 call count 並 assert 每個 request 恆為 `1`。

- [ ] `Step 4.2` 執行 RED：

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./proxy -run 'TestHandler' -count=1
```

Expected: compile/test failure或 assertion failure，因 handler 仍以 model router 決定 provider。

- [ ] `Step 4.3` 建立單一路徑 helper，`Handle` 與 `HandleCountTokens` 共用相同 binding/model authorization：

```go
func (h *Handler) resolveRequest(
	c *gin.Context,
	modelID string,
) (providers.Binding, *model.Credential, upstream.Profile, error) {
	binding, ok := providerBinding(c)
	if !ok {
		return providers.Binding{}, nil, upstream.Profile{}, missingProviderBindingError()
	}
	if !binding.AllowsModel(modelID) {
		return providers.Binding{}, nil, upstream.Profile{}, modelNotAllowedError()
	}
	var credential *model.Credential
	var err error
	if binding.RequiresCredential() {
		credential, err = h.credentials.Resolve(c.Request.Context(), binding.Provider())
		if err != nil {
			return providers.Binding{}, nil, upstream.Profile{}, err
		}
	}
	profile, err := binding.Profile(credential)
	return binding, credential, profile, err
}

func missingProviderBindingError() error {
	return &protocol.ProxyError{
		Kind: protocol.ERROR_UNAVAILABLE,
		Status: http.StatusInternalServerError,
		Code: "provider_binding_missing",
		Message: "provider authorization context is unavailable",
	}
}

func modelNotAllowedError() error {
	return &protocol.ProxyError{
		Kind: protocol.ERROR_AUTH,
		Status: http.StatusForbidden,
		Code: "model_not_allowed",
		Message: "model is not allowed for this API key",
	}
}
```

不得把拒絕的 model 或 API key 寫入公開 error/log。request body 中的 model 不得加/減 provider prefix；target format 直接使用 `binding.Protocol()`。

- [ ] `Step 4.4` 改寫 `HandleModels()`：從 request context 取 binding，依設定順序回傳 `Binding.Models()`；不得回傳其他 provider models、prefix patterns 或 upstream catalog union。

- [ ] `Step 4.5` 改寫 `HandleCountTokens()`：先做相同 exact model authorization，再只呼叫 selected profile 的 native endpoint；沒有 native capability 時回既有 `501 token_count_unsupported`，不得改呼叫其他 provider 或使用估算常數。

- [ ] `Step 4.6` 修改 `server.New()`：

```go
catalog, shadows, err := registry.New(cfg.Providers)
if err != nil {
	return nil, fmt.Errorf("new proxy server provider catalog: %w", err)
}
for _, shadow := range shadows {
	slog.Warn("duplicate client API key ignored",
		"fingerprint", shadow.Fingerprint,
		"winner_provider", shadow.WinnerProvider,
		"shadowed_provider", shadow.ShadowedProvider,
	)
}
```

把同一個 catalog 注入 handler、`/v1` middleware 與 `/admin` middleware；移除 `upstream.DefaultCatalog()`、`catalog.NewRouter()` 與 `Config.APIKeySet()` 的 production usage。

- [ ] `Step 4.7` 更新 server tests 的 config fixture，改用 ordered providers；新增 duplicate-key first-wins server test，確認 server 可啟動且 warning 不含 raw key。

- [ ] `Step 4.8` 執行 GREEN：

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./proxy -count=1
go test ./... -count=1
```

Expected: PASS；production path 的 provider selection 只剩 `ResolveAPIKey → Binding`。

---

### Task 5：同步 canonical docs 與完成 consistency verification

`Files：`

- Modify: `README.md`
- Modify: `CLAUDE.md`
- Create: `docs/specs/2026-07-19-proxy-provider-api-key-model-binding.md`
- Modify: `README.todo`，僅在已有對應 item 時移至 `Archive`；不得自行新增無關 TODO。

- [ ] `Step 5.1` 在 spec 寫入最終 flow：

```text
downstream x-api-key / Bearer
  → ordered providers catalog
  → selected Binding(provider, protocol, models)
  → exact model authorization
  → optional active upstream credential
  → source/target transform
  → exactly one upstream request
```

文件必須明確區分 downstream `client-api-keys` 與 upstream auth credentials，並包含本計畫的完整 settings example、first-wins duplicate 行為、no-fallback 規則與 Ollama loopback 限制。

- [ ] `Step 5.2` 更新 `CLAUDE.md` structure tree：加入 `proxy/providers/`；把 current proxy flow 從 `route.Router` 改為 API-key binding；保留 `route/` 時標示為 legacy/non-production。更新 `README.md` 的 proxy overview 與設定入口。

- [ ] `Step 5.3` 執行 consistency searches：

```bash
cd /Users/shuk/projects/agentSDK
rg -n 'api-keys|qualified model|route\.Router|DefaultCatalog|openai-chat/' \
  README.md CLAUDE.md proxy docs/specs plans
rg -n 'provider/openaicompat' proxy/providers
rg -n 'client-api-keys|model_not_allowed|duplicate client API key' \
  README.md CLAUDE.md proxy docs/specs plans
```

Expected: 第一組剩餘結果只能存在於 historical specs、legacy tests 或明確 migration 說明；第二組無結果；第三組同時命中 code、tests 與 canonical docs。

- [ ] `Step 5.4` 執行最終驗證：

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./... -count=1 -timeout=120s

cd /Users/shuk/projects/agentSDK
go test ./... -count=1 -timeout=120s
git diff --check
git status --short
```

Expected: proxy 與 root tests exit `0`、`git diff --check` 無輸出。若 root baseline 有與本功能無關的既存 failure，必須保留 exact package/test/error evidence，不能宣稱全部通過，也不能順手修改 scope 外檔案。

## Acceptance Checklist

- [ ] 任一 valid client API key 只選到 ordered providers 中第一個 matching provider。
- [ ] duplicate client API key 只產生 secret-free warning，不造成 fallback 或多次 upstream call。
- [ ] request model 必須存在於 selected binding 的 exact model list；不允許時回 `403 model_not_allowed`。
- [ ] `/v1/models` 只顯示 selected key 可用 models，保持設定順序。
- [ ] OpenAI、Anthropic、MiniMax 與 xAI 沿用 active credential resolver；Ollama 不解析 credential。
- [ ] Ollama 只使用 OpenAI-compatible Chat Completions 且不送 Authorization header。
- [ ] 各 client protocol 仍可透過既有 `3×3` registry 轉成 configured provider protocol。
- [ ] 429、5xx、timeout、invalid response 皆不嘗試第二個 provider。
- [ ] production server/handler 不再使用 model-based `route.Router` 或 `upstream.DefaultCatalog()` 選 provider。
- [ ] README、CLAUDE、spec、tests 與 config names 一致，且未修改 proxy scope 以外的 runtime/provider/receipt/pm2 code。
