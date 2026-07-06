# M3 Spec — 工具生態 + 執行期安全

## 目標

加入:
1. **Schema 反射** — `action.TypedTool` 從 struct tags 自動產生 JSON Schema,並在 `Call` 前做基本驗證
2. **Sandbox** — 路徑 / 指令 allow/deny 政策
3. **Spotlight + Sanitizer** — 標記 + 過濾 TOOL_RESULT 中的 prompt injection
4. **Tracing** — OpenTelemetry span 包每個 effect
5. **MCP adapter** — `mcp.Client` 實作 `action.ToolSource`,可從本地 MCP server 拿工具
6. **Sample todo tools** — `add_todo` / `list_todos` / `complete_todo`,加上 `logdoctor list` 命令

## 設計原則

- **Schema 自動反射**:TypedTool 用 `invopop/jsonschema` 從 struct tags 產生 schema,免去手寫;`ValidateArgs` 做基本驗證(僅 required + 型別 JSON 合法性,不替換完整 JSON Schema validator)
- **Schema 驗證在 TypedTool.Call 內執行**:失敗 → `ToolResult{OK:false, Error: "schema validation failed: ..."}`,fn 不會被叫
- **Sandbox 政策可注入**:default 是 `action.DefaultPolicy()`,允許 `/tmp` 路徑,阻擋危險指令 (`rm -rf /`, fork bomb, `shutdown` 等)
- **Sandbox DENY 改寫成 NOTIFY + DONE**:不丟 phantom tool result,直接告訴 LLM 拒絕了
- **Spotlight + Sanitizer 是 middleware**:chain 中 `Spotlight` 是最外層,`Sanitizer` 在它內側 — sanitizer 處理原始文字,spotlight 包 sanitized output 加上 `<UNTRUSTED_TOOL_OUTPUT>` 標記
- **Tracing 用 OTel SDK 內建 tracetest**:測試用 `tracetest.NewSpanRecorder` 驗證 span 數與 attribute
- **MCP 是獨立 module**:`go.work` 加 `./mcp`,避免把 MCP SDK 拖進 root SDK
- **Sample 的 todo 是 in-memory thread-safe**:用 sync.Mutex,M4 再決定是否落盤

## 套件結構

| 套件 | 角色 | 關鍵型別 |
|------|------|---------|
| `action/` | Schema + Sandbox | `SchemaFor[T]`, `SchemaForTool[T]`, `ValidateArgs[T]`, `Policy`, `Verdict`, `Sandbox` |
| `middleware/security/` | Sandbox mw + Spotlight + Sanitizer | `Sandbox(policy)`, `Spotlight()`, `Sanitizer{}.Middleware()`, `DefaultSanitizer()` |
| `middleware/observability/` | OTel tracing | `Tracing(cfg)`, `TracingConfig{TracerProvider}` |
| `mcp/` | MCP adapter | `Client`, `NewClient(session)`, `Discover`, `Call` |
| `sample/logdoctor/core/` | Todo store | `Todo`, `TodoStore`, `TodoStatus` |
| `sample/logdoctor/tool/` | Todo tools | `AddTodo`, `ListTodos`, `CompleteTodo` |
| `sample/logdoctor/cmd/list.go` | `list` 子命令 | `RegisterList` |

## 關鍵介面

### Schema 反射

```go
// SchemaFor 反射 T 的 struct tag 產生 JSON Schema。
func SchemaFor[T any]() *jsonschema.Schema
func SchemaJSON[T any]() (json.RawMessage, error)
func SchemaForTool[T any](name, desc string, risk core.RiskLevel) (core.ToolSchema, error)

// ValidateArgs 檢查 raw 是否符合 SchemaFor[T]()。
// 檢查:JSON 合法性 + 型別 map[string]any + required 欄位存在。
func ValidateArgs[T any](toolName string, raw json.RawMessage) (valid bool, err error)
```

jsonschema 反射器對 named struct 會產生 `$ref: #/$defs/<TypeName>`,所以 required 列表在 `$defs.<TypeName>.required`,`ValidateArgs` 自動 resolve ref。

### Sandbox

```go
type Sandbox interface {
    Check(toolName string, args map[string]any) Verdict
}
type Verdict int  // ALLOW | DENY

type Policy struct {
    AllowedPathPrefixes     []string  // 預設 ["/tmp"]
    PathKeys                []string  // 預設 ["path"]
    DeniedCommandSubstrings []string  // 預設 rm -rf /, fork bomb, shutdown, ...
    CommandKeys             []string  // 預設 ["command", "cmd"]
}
```

### Security Middlewares

```go
// Spotlight wraps CALL_TOOL return-path ToolResult.Output with
// <UNTRUSTED_TOOL_OUTPUT>...</UNTRUSTED_TOOL_OUTPUT>.
func Spotlight() middleware.Middleware

// Sanitizer 預設 8 條 regex ("ignore previous instructions",
// "system:", "forget everything", "<|...|>" 等)。
func DefaultSanitizer() *Sanitizer
func (s *Sanitizer) Middleware() middleware.Middleware
```

### OTel Tracing

```go
type TracingConfig struct {
    TracerName      string
    TracerProvider  trace.TracerProvider  // 預設 otel.GetTracerProvider()
}
func Tracing(cfg TracingConfig) middleware.Middleware

// Span name 依 effect: tool.<name> / model.<id> / approval.request /
// notify / loop.done / checkpoint / emit / effect.<kind>
// Attributes: agentsdk.effect.kind + tool/model/approval/notify 子鍵
```

### MCP

```go
type Client struct{ session *mcppkg.ClientSession }
func NewClient(session *mcppkg.ClientSession) *Client

// Discover → []core.ToolSchema (from MCP ListTools)
// Call(name, json.RawMessage) → core.ToolResult
// risk: 由 inferRisk() 啟發式分類 (get_/read_=low, delete_/exec_/shell_=high)
```

### Sample Todo Store

```go
type Todo struct {
    ID, Title string; Status TodoStatus; CreatedAt, UpdatedAt time.Time
}
type TodoStore struct { /* sync.Mutex + items []Todo */ }
func (s *TodoStore) Add(title) Todo
func (s *TodoStore) List() []Todo
func (s *TodoStore) Complete(id) (Todo, bool)
func (s *TodoStore) Open() []Todo  // helper for agent
```

## 行為保證

### Schema 反射
- `ValidateArgs` 失敗 → ToolResult{OK:false},fn **不** 被呼叫
- required 欄位依 `json:"name"` (無 omitempty) 自動推斷
- 任意型別 (string/int/bool/map/[]) 都能正確反射

### Sandbox
- ALLOW 通過;DENY 改寫成 `EFFECT_NOTIFY{level=error, "sandbox denied tool X with args ..."}` + `EFFECT_DONE`,run 結束
- 路徑必須是絕對路徑且落在 `AllowedPathPrefixes` 內
- 指令比對大小寫不敏感的子字串

### Spotlight + Sanitizer
- Spotlight chain 位置:最外層 (後處理)
- Sanitizer chain 位置:Spotlight 內側
- 觸發 sanitizer → output 換成 `[SANITIZED_BY_AGENTSDK] reason="..." original_len=N`,再被 spotlight 包成 `<UNTRUSTED_TOOL_OUTPUT>...</UNTRUSTED_TOOL_OUTPUT>`

### Tracing
- 每個 effect 1 span
- `agentsdk.tool.name` / `agentsdk.tool.call_id` / `agentsdk.tool.risk` attribute 在 CALL_TOOL span 上

### MCP
- `Discover` 透過 stdio / in-memory / http transport 都可;測試用 `NewInMemoryTransports`
- `Call` 把 `json.RawMessage` 解碼成 `map[string]any` 餵給 MCP `CallToolParams.Arguments`
- 結果編碼成 `[{type:"text", text:"..."}, ...]` JSON

## M2→M3 升級注意事項

- `TypedTool.Call` 在 M3 變嚴格:缺 required 欄位會被擋下來。已修正 `action_test.go` 與 sample tools(`Message` / `Path` / `Level` 等加上 `omitempty` 或確保測試填入)。
- 移除 `TypedTool.SchemaV` 欄位 — 由反射自動產生。Sample tool wrappers 改用 `Tool.Inner.Schema()` 鏈。
- `MCP` 是獨立 module(`go.work` 加 `./mcp`),`agentsdk/core` 與 `agentsdk/action` 不會被 MCP SDK 污染。

## 範例

### Schema 反射範例

```go
type ReadFileArgs struct {
    Path string `json:"path"`           // required
    N    int    `json:"n,omitempty"`    // optional
}

ts, _ := action.SchemaForTool[ReadFileArgs]("read_file", "...", core.RISK_LEVEL_LOW)
// ts.Parameters 是 reflection 結果,required = ["path"]

ok, err := action.ValidateArgs[ReadFileArgs]("read_file", json.RawMessage(`{}`))
// ok=false, err: "read_file: schema validation failed: [missing required field: path]"
```

### Sample CLI Todo 流程

```
$ logdoctor --fake run --once --fixture error.log
... add_todo ×3 → notify → done
$ logdoctor list --data-dir ./data
3 run(s) in ./data/states:
  run-1783... turns=4 status=completed
```

## 測試驗證

| 計畫驗收項 | 測試位置 |
|------------|---------|
| Args struct schema 含 required 欄位 | `action/schema_test.go` (4 tests) |
| sandbox allow/deny table | `action/sandbox_test.go` (8 tests) |
| sandbox mw DENY 改寫成 NOTIFY + DONE | `middleware/security/sandbox_mw_test.go` (4 tests) |
| spotlight 以分隔符標記 untrusted | `middleware/security/security_test.go::TestSpotlightWrapsToolOutput` |
| sanitizer 命中 fixture 注入字串 | `middleware/security/security_test.go::TestSanitizerDetectsIgnorePreviousInstructions` 等 |
| tracing span 數/屬性 | `middleware/observability/tracing_test.go` (3 tests) |
| mcp.Client.Discover 回傳宣告工具 | `mcp/client_test.go::TestDiscoverReturnsDeclaredTools` |
| e2e: sanitizer + spotlight chain | `middleware/security/security_integration_test.go::TestM3ChainDirect` |
| sample todo 工具 | `sample/logdoctor/tool/todo_test.go` (4 tests) |

## 對應原始 plan

本 spec 對應 `plans/plan-only-and-plan-breezy-pike.md` 的 M3 區段。