# M4 — 架構解耦 + HITL + 三 Provider (Todo)

> Source-of-truth: `plans/plan-only-and-plan-breezy-pike.md` 第 361-372 行
> 規格完成後轉入 `docs/specs/YYYY-MM-DD-m4-hitl-providers.md`
> 建立日期: 2026-07-07
> 前置: M3 收完才進;M3 細節見 `plans/2026-07-07-m3-tooling-security.md`

## Context

M3 (工具生態 + 執行期安全) ✅ 後,M4 是終章:把 SDK 從「單機 demo」推到「生產多 provider + 人工審批」。

三大目標:
1. **架構解耦** — 證明 `runtime/core` 對 provider 型別零洩漏
2. **完整 HITL** — mid-run approval 透過 `StateStore` 跨 process 恢復
3. **三 provider 實測** — anthropic / openaicompat / google 對同一 prompt 等價

## Pre-flight (M4 開工前)

- [ ] M3 全部 DoD 過(見 `plans/2026-07-07-m3-tooling-security.md` Definition of Done)
- [ ] 確認 `anthropic-sdk-go` 在 module cache(M4 開工才用,不在 M3 Pre-flight 收)
- [ ] `google.golang.org/genai@v1.62.0` 已驗證可用
- [ ] 本地 Ollama 已安裝且在跑(openaicompat 預設檢查用)
- [ ] 有 `ANTHROPIC_API_KEY` 與 `GOOGLE_API_KEY` 環境變數(CI 用 `if [ -n "$KEY" ]` 條件跑)

---

## 建立檔案

- [ ] `action/approval_policy.go` — `DefaultApprovalPolicy`,L1/L2 企業預設(high risk 一律 ASK 直到 L3+;low risk L1+ 自動)
- [ ] `cli/envelope.go` + `cli/codec.go` — JSONL 協定
- [ ] `provider/anthropic/{go.mod,provider.go}` — `anthropic-sdk-go`
- [ ] `provider/openaicompat/{go.mod,provider.go}` — Ollama / LM Studio,token 計數用 chars/4 啟發式
- [ ] `provider/google/{go.mod,provider.go}` — `google.golang.org/genai`
- [ ] `go.work` 增列 `./provider/*`
- [ ] `sample/logdoctor`:
  - [ ] `cmd/watch.go` — `--interactive` 模式,接收 log append
  - [ ] `cmd/approve.go` — `logdoctor approve <approval-id>`,out-of-band resume
  - [ ] `tool/propose_fix.go` — `RiskLevel: high`,觸發 `approval_request`
  - [ ] `cmd/root.go` 接 provider 選擇(`--provider=anthropic|openaicompat|google`)

---

## 驗收 (Verification)

**單元**:
- [ ] 含 mid-run `PendingApproval` 的 State JSON round-trip(脫水/復水)— 對應 M2 既有 `Checkpointer.Recover`
- [ ] `ApprovalGate` 使 run 進入 `RUN_STATUS_PAUSED_APPROVAL`
- [ ] approve / reject 分歧正確
- [ ] `cli.Codec` 對每個 `MessageType` round-trip
- [ ] DI 抽換測試:同一 `runtime.Loop` 換兩個 `FakeProvider`,斷言 `runtime/core` 無 provider 型別洩漏
- [ ] `Chunk{Kind: IMAGE}` 全程穿透 Message 不損毀(證明多模態抽象成立)
- [ ] 三 provider 各自對應的 mock-driven 測試(不需要真 key)

**完整 e2e (目標使用者故事)**:
- [ ] `logdoctor watch <path> --interactive` → append ERROR 行
- [ ] 預期 JSONL 序列:`percept → ReAct tool calls → add_todo x3 → propose_fix → approval_request`
- [ ] **另一個 process** 執行 `logdoctor approve <approval-id>`,證明透過 `StateStore` 非同步 out-of-band resume
- [ ] 修復套用 → `complete_todo` → `StdoutNotifier` 摘要

**Provider 實測**(用真憑證):
- [ ] `openaicompat` 指向本地 Ollama(免憑證,作為預設檢查)
- [ ] 有 `ANTHROPIC_API_KEY` 時跑一次 `anthropic` provider
- [ ] 有 `GOOGLE_API_KEY` 時跑一次 `google` provider
- [ ] 三 provider 對同一 prompt 結果的 `core/effect.go` 序列化等價性測試

---

## 風險 (M4)

| # | 風險 | 處理 |
|---|------|------|
| R1 | `anthropic-sdk-go` / `google.golang.org/genai` API 形狀 | Pre-flight + 參考 `provider/*/testdata/` fixture |
| R2 | Provider 實測需真憑證 | 預設 Ollama;Anthropic/Google 在 CI 用 `if [ -n "$KEY" ]` 條件跑 |
| R3 | mid-run approval + StateStore 非同步 resume 的 race | 需 idempotency key(同 `Checkpointer.Recover` 的 `Input.Seq`) |
| R4 | `Chunk{Kind: IMAGE}` 三 provider 表達差異 | provider 內部 translate,`core` 維持純 abstract |

## Definition of Done

- [ ] `go test ./...` 全綠(root + sample/logdoctor + provider/* + mcp)
- [ ] `golangci-lint run` 全綠(`ST1003` 例外已在 `.golangci.yml`)
- [ ] `docs/specs/YYYY-MM-DD-m4-hitl-providers.md` 從本檔轉出
- [ ] `CLAUDE.md` Milestone 進度表 M4 改為 ✅
- [ ] `README.todo` M4 條目更新為 ✅
- [ ] `sample/logdoctor/README.md` 補上 `--provider` 與 `--interactive` 章節

## 進度

```
M4 開始: _____(M3 收後)
M4 收:    _____
```
