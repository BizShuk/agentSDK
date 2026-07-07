# M3 — 工具生態 + 執行期安全 (Todo)

> Source-of-truth: `plans/plan-only-and-plan-breezy-pike.md` 第 349-359 行
> 規格完成後轉入 `docs/specs/YYYY-MM-DD-m3-tooling-security.md`
> 建立日期: 2026-07-07
> 前置: 見本檔 Pre-flight 段;M4 見 `plans/2026-07-07-m4-hitl-providers.md`

## Context

M1 (核心範式 + sample 骨架) ✅、`M2` (系統韌性 + 循環防禦) ✅ 已收。
M3 是下一棒:把 SDK 從「能用」推到「能用在 production」 — 自動產生 tool schema、執行期 sandbox、工具輸出信任標記、MCP 動態工具發現。

執行節奏沿用 plan: M3 收完才進 M4,每個 milestone 有獨立驗收 (單元 + e2e)。

## Pre-flight (M3 開工前)

- [ ] 確認 `invopop/jsonschema` 在 module cache,無則 `go get` 後看 API 形狀再寫 adapter (見 Risks #1)
- [ ] 確認 `github.com/modelcontextprotocol/go-sdk` 在 module cache
- [ ] `go.work` 新增 `./mcp` 進入點(目前只有 root + sample/logdoctor)
- [ ] `golangci.yml` 為 `SCREAMING_SNAKE_CASE` 常數加 `ST1003` 例外(Risks #4)
- [ ] **決定 `perception/` 去留**(見下方 M3 carry-over)— 預設:無多 source 場景就走 delete

---

## 建立檔案

- [ ] `action/schema.go` — `invopop/jsonschema` struct 反射 + required-field 最小驗證
- [ ] `action/sandbox.go` — 路徑/指令 allow-deny table
- [ ] `action/approval_policy.go` *(若 M3 順便做,否則延 M4)*
- [ ] `middleware/security/spotlight.go` — 工具輸出 untrusted 區段標記(分隔符包覆)
- [ ] `middleware/security/sanitizer.go` — 命中 fixture 注入字串(`"ignore previous instructions ..."`)
- [ ] `middleware/security/sandbox_mw.go` — 串接 `action.Sandbox` 進 chain
- [ ] `middleware/observability/tracing.go` — OTel span(重用 `gosdk/metric.Tracer`)
- [ ] `mcp/go.mod` + `mcp/client.go` — `modelcontextprotocol/go-sdk`,實作 `action.ToolSource`
- [ ] `go.work` 增列 `./mcp`
- [ ] `sample/logdoctor`:
  - [ ] `tool/{add_todo,list_todos,complete_todo}.go` — 既有 tools 改用 `TypedTool[TArgs]`
  - [ ] `core/todo.go` — todo domain noun
  - [ ] `cmd/list.go` — `logdoctor list`

## Carry-over (從 M2 順手收)

- [ ] **決定 `perception/` 去留**(見 `perception/source.go` 與 `normalize.go` 的 `TODO(M3)`)
  - 選項 A: 接上 — 在 `runtime/loop.go` 加 `FanIn.Observations(ctx)` call site,並讓 `sample/logdoctor` 實作 `FileTailer : ObservationSource`
  - 選項 B: 內聯 — 把 `ObservationSource` / `FanIn` / `ToMessage` 整段搬進 `sample/logdoctor/core/listener.go` 旁,刪除 root `perception/`
  - 選項 C: 刪除 — 連同 `core/input.go` 的 `ObservationSource` shim 一起拔
  - 預設:若 M3 沒有「agent 觀察多 source」的場景就走 C
- [ ] `loopguard` 行為確認(Risks #3):「無新資訊」vs「同名工具再呼叫」判準需有 fixture 測試

---

## 驗收 (Verification)

**單元**:
- [ ] 每個 tool 的 Args struct schema 含 required 欄位(以 fixture struct 反射斷言)
- [ ] `sandbox` allow/deny table:`action/sandbox_test.go`
- [ ] `spotlight` 以分隔符包覆工具輸出,`untrusted` 區段可被 `sanitizer` 命中
- [ ] `sanitizer` 命中 fixture 注入字串(M3 fixture:`"ignore previous instructions"`)
- [ ] `tracing` 用 in-memory OTel exporter 斷言 span 數/屬性
- [ ] `mcp.Client.Discover` 對本地測試 MCP server 回傳宣告工具(需先寫 fixture MCP server)

**e2e**:
- [ ] 注入含 prompt injection payload 的 log 行
- [ ] 斷言 transcript 中工具結果已被 spotlight 標記 untrusted
- [ ] 斷言 `list_todos` 只含合法補救任務(sanitizer 命中後 todo 提案被丟棄)

---

## 風險 (M3)

| # | 風險 | 處理 |
|---|------|------|
| R1 | `invopop/jsonschema` / `modelcontextprotocol/go-sdk` API 形狀未知 | Pre-flight `go get` 後 adapter 寫之前先看型別 |
| R2 | prompt injection fixture 需要維持更新 | e2e 測試加 `prompts/injection_fixtures.txt` |
| R3 | `perception/` 去留(見 M3 carry-over) | M3 開工前決策 |
| R4 | `SCREAMING_SNAKE_CASE` 違反 `ST1003` | Pre-flight 在 `.golangci.yml` 加例外 |

## Definition of Done

- [ ] 所有單元 + e2e 全綠
- [ ] `golangci-lint run` 全綠
- [ ] `docs/specs/YYYY-MM-DD-m3-tooling-security.md` 從本檔轉出
- [ ] `CLAUDE.md` Milestone 進度表 M3 改為 ✅
- [ ] `README.todo` M3 條目更新為 ✅

## 進度

```
M3 開始: 2026-XX-XX
M3 收:    _____(預估)
```
