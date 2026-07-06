# M4 Spec — 架構解耦 + HITL 完整 + 三個 LLM Provider

## 目標

加入:
1. **Approval policy + gate middleware** — `action.DefaultApprovalPolicy` (L0-L4 vs RiskLevel grid) + `security.ApprovalGate` middleware 在 ASK 時 emit REQUEST_APPROVAL
2. **CLI Envelope/Codec** — `cli.Envelope` 9 種 message type + JSONL `cli.Codec` 序列化
3. **三個獨立 provider module** — anthropic-sdk-go / openaicompat (stdlib HTTP) / google.golang.org/genai
4. **Sample watch + approve + propose_fix** — long-running watch 模式 + 離線 approve 決策 + high-risk 修復提案

## 設計原則

- **Provider 三模組獨立**:`go.work` 列 `provider/{anthropic,openaicompat,google}` 各有獨立 `go.mod`;每個 provider 只 import `github.com/bizshuk/agentsdk/core`,不污染 root SDK
- **stdlib openaicompat**:`net/http` 直接打 `/chat/completions`,免依賴 — 支援 Ollama / LM Studio / vLLM / OpenAI 全部相容端點
- **ApprovalGate 在 CALL_TOOL 出口**:middleware 看到 `EFFECT_CALL_TOOL` → 查 `policy.Decide` → ALLOW 透過 / ASK 改寫成 REQUEST_APPROVAL / DENY 改寫成 NOTIFY error。runtime loop 不需感知 policy 細節
- **Approval Policy 是純資料表**:`DefaultApprovalPolicy` 是 stateless grid lookup;M4 不引入動態 override (M5+ 可加 env flag)
- **CLI Envelope 是 pure DTO**:`Codec` 不變動 `core.Effect` 形狀;純粹 wire-format
- **IMAGE chunk 不在 transport 層被破壞**:JSON round-trip 後 byte 完全相同

## 套件結構

| 套件 / module | 角色 | 關鍵型別 |
|--------------|------|---------|
| `action/` | Approval policy | `DefaultApprovalPolicy`, `gridLookup(autonomy, risk)` |
| `middleware/security/` | ApprovalGate mw | `ApprovalGate(autonomy, policy)` |
| `cli/` | Wire format | `Envelope` (9 種 Type), `Codec`, `MessageType` (9 consts) |
| `provider/anthropic/` | Claude SDK adapter | `Provider`, `New(opts)`, `WithAPIKey`, `WithModel` |
| `provider/openaicompat/` | Ollama/OpenAI HTTP | `Provider`, `New`, `WithBaseURL`, `WithModel` |
| `provider/google/` | Gemini SDK adapter | `Provider`, `New(ctx, opts)`, `WithAPIKey`, `WithModel` |
| `sample/logdoctor/tool/propose_fix.go` | High-risk tool | `ProposeFix` (RISK_LEVEL_HIGH) |
| `sample/logdoctor/cmd/watch.go` | Watch loop | `RegisterWatch` |
| `sample/logdoctor/cmd/approve.go` | Out-of-band approve | `RegisterApprove` |

## Approval Grid

| Autonomy \ Risk | LOW  | HIGH |
|-----------------|------|------|
| L0              | ASK  | ASK  |
| L1              | ALLOW | ASK |
| L2              | ALLOW | ASK |
| L3              | ALLOW | ALLOW |
| L4              | ALLOW | ALLOW |

L0 = 完全人工,L1/L2 = 企業預設 (low 自動 / high ASK),L3 = 高自主性,L4 = 全自動。

## CLI MessageType 對照

| Type | Payload | 用途 |
|------|---------|------|
| `percept` | `PerceptPayload` | Source 發出 percept |
| `assistant` | `AssistantPayload` | LLM 回應 |
| `tool_call` | `ToolCallPayload` | CALL_TOOL effect |
| `tool_result` | `ToolResultPayload` | CALL_TOOL 完成 |
| `approval_request` | `ApprovalPayload` | policy ASK → 暫停 |
| `approval_decision` | `DecisionPayload` | operator approve/reject |
| `checkpoint` | `CheckpointPayload` | StateStore 持久化標記 |
| `result` | `ResultPayload` | 終端 status |
| `error` | `ErrorPayload` | 不可恢復錯誤 |

## Provider 切換契約

```go
// 同一 Loop,不同 provider — 不需重新編譯
loop := runtime.NewLoop(step, prov, reg)  // prov 任意實作 core.ModelProvider

// 三個 provider 都實作同樣的四個方法
type ModelProvider interface {
    Name() string
    Generate(ctx, req) (ModelResult, error)
    Stream(ctx, req) (<-chan ModelChunk, error)
    CountTokens(ctx, msgs) (int, error)
}
```

`TestDIProviderSwap` 斷言兩個 FakeProvider (A/B) 都能驅動同一 Loop。

## 範例

### CLI Envelope 範例

```go
codec := cli.NewJSONLCodec(os.Stdin, os.Stdout)
codec.Write(cli.Envelope{
    Type: cli.MSG_TYPE_PERCEPT,
    Percept: &cli.PerceptPayload{Source: "logfile", Payload: "ERROR"},
})
```

### Provider 切換

```go
// Ollama 本地
p, _ := openaicompat.New(openaicompat.WithBaseURL("http://localhost:11434/v1"))

// Anthropic Claude
p, _ := anthropic.New(anthropic.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")))

// Gemini
p, _ := google.New(ctx, google.WithAPIKey(os.Getenv("GOOGLE_API_KEY")))

loop.Model = p  // 任一 provider 直接喂 runtime.Loop
```

### Approval Gate

```go
policy := action.DefaultApprovalPolicy{}
loop.Middleware = middleware.Chain(
    security.ApprovalGate(core.AUTONOMY_L2, policy),
    // ... 其他鏈
)

// 高風險 propose_fix 在 L2 下會被改成 REQUEST_APPROVAL
```

## 測試驗證

| 計畫驗收項 | 測試位置 |
|------------|---------|
| State 含 mid-run PendingApproval JSON round-trip | `cli/codec_test.go::TestStateRoundTripPreservesMidRunApproval` |
| ApprovalGate ASK → REQUEST_APPROVAL + 進 PAUSED_APPROVAL | `middleware/security/approval_gate_test.go` |
| approve/reject 分歧 | `middleware/security/approval_gate_test.go::TestApprovalGate*` |
| `cli.Codec` 對 9 種 MessageType round-trip | `cli/codec_test.go::TestCodecRoundTripAllMessageTypes` |
| DI 抽換 — 同一 Loop 換兩 provider 無型別洩漏 | `runtime/di_integration_test.go::TestDIProviderSwap` |
| `Chunk{Kind:IMAGE}` 全程穿透 | `runtime/di_integration_test.go::TestImageChunkSurvivesRunLoop`<br>`cli/codec_test.go::TestImageChunkSurvivesJSONRoundTrip` |
| openaicompat 指向本地 Ollama | `provider/openaicompat/provider_test.go` (4 tests, httptest fake Ollama) |
| Anthropic provider 結構驗證 (有 key 才跑) | `provider/anthropic/provider_test.go` |
| Google provider 結構驗證 (有 key 才跑) | `provider/google/provider_test.go` |
| propose_fix 高風險 | `sample/logdoctor/tool/propose_fix_test.go` |

## Provider 測試策略

- **openaicompat**:用 `httptest.NewServer` 模擬 Ollama,所有路徑本地跑 (有 API key / bearer / error / 無 key)
- **anthropic / google**:在沒有對應 env var 時 `t.Skip`;CI 也可跑,只驗證 `New()` 結構 + `CountTokens` heuristic
- M5+ 在 dev machine 有 key 時可手動跑實際 round-trip

## 對應原始 plan

本 spec 對應 `plans/plan-only-and-plan-breezy-pike.md` 的 M4 區段。