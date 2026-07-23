# agentSDK Tutorial — Build an Agent with the Skeleton

> 對象：想用 `agentSDK` 寫一個新應用、但還沒從 `sample/code-agent/cmd/compose.go` 333 行手寫組裝的舊路活過來的人。
> 完成本教學後你會寫出來：一份宣告式設定、一個六行的 `main.go`、一個 `wizard` 驅動的 onboarding 流程。

本教學假設你已讀過 [`docs/tutorials/01-getting-started.md`](01-getting-started.md)（或同等熟悉度）。術語以 [`docs/terminology.md`](../terminology.md) 為準。

## 前置需求

- Go `1.26+`
- `go work sync` 完成；`go test ./... -count=1` 全綠
- 你正在 `~/projects/ai/agentSDK/`

## 為什麼需要 skeleton

`agentSDK` 是一個目標導向控制迴圈 (`Goal-directed Control Loop`) SDK。它分成兩半：底層是 `core/` + `runtime/`（純狀態機與 shell），上層是 `action/` + `permission/` + `session/` + `skill/` + `subagent/` + `hook/` + `wire/`（harness 能力）。

上層能力很多，接線順序很多；應用層只想說「我要什麼」。

### 沒有 skeleton 的日子

骨架落地之前，每個新應用都靠 `compose.go` 手寫一份組裝。`sample/code-agent/cmd/compose.go` 2026-07-22 之前的版本是 `333` 行——這個數字本身就是問題的證明。把它壓成摘要（完整版仍可從 git 撈回 `5d3a8ba` 之前）：

```text
buildProvider(o)                  // switch 五個 adapter,各自的 envLookup + WithXxxOption
buildProvider → buildTools → buildContextFile → buildSkills →
  buildSubagents → buildPermission → buildHookRunner → buildEngine

每個 build* 函式獨立存在。順序約束散落在 buildEngine 內的註解裡。
buildTools 內建 6 個 + subagent.Spawner（自建 sub-engine）
buildSkills 兩層 dir 探索（user + project）
buildSubagents 需要 subagentRunner closure，closure 自己又拼一套 Engine
buildPermission 用 config.SecureMiddleware(sandbox, perm) — 同一個 *Engine
    要餵兩個地方
buildHookRunner — 範例：HOOK_PRE_TOOL_USE / bash / 含 'rm -rf /' 拒絕

contextfile.Loader.Load(cwd) → 拼進 system message
skill.SystemPrompt()            → 拼進 system message
mergeSystem(parts)               → join 成單一 ROLE_SYSTEM
newState(MaxTurns=20, Autonomy=L2, Budget, system prompt)

新行為的擴充點全不明顯：Hook 規則 → 改 buildHookRunner；新工具 →
改 buildTools；新策略 → 改 buildEngine。每一個都要碰順序約束。
```

### 為什麼這是問題

三個症狀，同時也是骨架設計的目標：

1. **順序是隱性合約**。`buildTools` 必須在 `buildSubagents` 之前，因為後者要前者產出的 Provider。但程式碼沒有強制這點——換順序還是能 compile，只是執行到 `subagent.NewSpawner` 時 provider 是 nil。沒有 type-system、沒有 runtime check。
2. **重複的 scaffold**。每個新應用都重寫這 333 行的不同切片。`sample/file-agent` 和 `sample/logdoctor` 的 `cmd/*.go` 各有自己的 60 行 `compose`，結構一模一樣，名稱微調。`sample/code-agent` 擴充最多，變成 333 行。
3. **擴充點不明顯**。要新增一個 hook 規則——改 `buildHookRunner`。要新增工具——改 `buildTools`。但這些函式既長又夾著順序約束，新手只能複製貼上再改。

### skeleton 解掉什麼

`agent/` 把三件事收回組裝層：

| 職責 | 原本在 | 現在在 |
| --- | --- | --- |
| 順序約束 | `compose.go` 的 8 個 `build*` 函式 | `agent/build.go::Bootstrap` 的 8 stage pipeline，doc comment 列每一條約束 |
| 重複 scaffold | 每個 sample 自己寫 | 應用層寫 `agent.Config{}`，skeleton 展開 + 組裝 |
| 擴充點 | 在哪個 `build*` 加哪幾行 | block pointer / 具名字串 variant，wizard `--list` 看得到 |

應用層的 `agent.Config{}` + 幾個 `agent.Option` 比 333 行 `compose.go` 的程式碼量小一個量級，可讀性卻高一階——前者說「我要什麼」，後者說「我要怎麼接成這個東西」。前者是宣告，後者是實作。

實作落地的範例是 `sample/code-agent/cmd/compose.go`：`333` → `101` 行；實際的 101 行在後面的「進階」段會再看到一次。

## 一條規則要記住：兩層 opt-in

整個 skeleton 的設定只有一條規則：

```text
層 1 feature — block 是 pointer：缺 key = nil = 關；{} = 開且用預設
層 2 variant — block 內的具名字串：空字串 = 該 feature 的預設實作
```

`planning` 再多一層正交軸：`reasoning.enable`（註冊哪些策略進 `core.NewDecide`）vs `reasoning.style`（這次跑哪個）。

把這條規則記住，後面所有的 block 都不用單獨說明。

### 三個狀態的對照

以 `Safety` block 為例，三種寫法各自代表什麼：

```yaml
# 1. 完全沒有 safety 區塊 → nil → 沒有 approval gate，沒有 sandbox
#    適合 trust the model 的場景（oneshot 分類器、唯讀工具集）

# 2. 空物件 → &Safety{} → 開啟且用預設值
#    mode = "default"（先試規則再 fallback grid）
#    fallback = "autonomy"（用 L0-L4 risk grid）
#    sandbox = false（沒勾 Sandbox=true，所以不檢查路徑/指令）
safety: {}

# 3. 顯式列舉 → 知道自己在做什麼
safety:
  mode: acceptEdits       # 低風險自動放行，高風險才問
  sandbox: true           # 啟動路徑/指令檢查
  deny: ["bash(sudo:*)"]  # 永遠拒絕 sudo
```

第 `1` 種寫法對 `core.NewDecide` 而言 `eng.Approval == nil`——runtime 與 middleware chain 都視為「no gate」，模型呼叫的工具全放行。第 `2` 種寫法建一個有預設規則的 `*permission.Engine`，配上 `config.SecureMiddleware(sandbox, perm)`——`Sandbox=false` 這條是 `Expand` 不會覆寫的特例（`ApplyBlockDefaults` 只填 `Mode`/`Fallback`），所以 `&Safety{}` 不會自動 sandbox。第 `3` 種寫法等於自填一份檢查清單。

判定時機：`Expand` 跑完才判定，所以 `MustNew` 與 `LoadFile` 會在檔案讀入後自動套用展開。手寫 `Config{}` literal 也可以先呼叫 `cfg.Expand()` 自己看結果。

### `Reasoning` 的第三層正交軸

兩個欄位各管一件事：

```yaml
reasoning:
  style: choose_agent          # 這次跑哪個 → seed core.State.ReasoningStyle
  enable:                     # 註冊哪些到 core.NewDecide 的 map
    - choose_agent            # 一定要含 style，否則 NewDecide 報 NOTIFY error
    - think_then_act          # router 委派的對象
    - plan_then_run
```

`Style` 沒在 `Enable[]` 裡是常見拼寫錯誤——`core.NewDecide` 未註冊的 style 不會 reason，會 emit `INSTRUCTION_NOTIFY` 說 `unknown reasoning style`。`Validate` 在讀檔階段就擋下，所以這是設定期錯誤不是執行期錯誤。`TestReasoningStyleNotRegistered` 鎖住這條。

## 步驟 1 (Step 1)：寫最小的設定

新建檔案 `agent.yaml`：

```yaml
name: greeter
tier: basic
model:
  provider: minimax
```

把它丟給 `agent.LoadFile`：

```go
package main

import (
    "fmt"
    "github.com/bizshuk/agentsdk/agent"
)

func main() {
    cfg, err := agent.LoadFile("agent.yaml")
    if err != nil { panic(err) }
    fmt.Println("tier:", cfg.Tier, "provider:", cfg.Model.Provider)
}
```

預期輸出（不是真的執行——下面會帶你實際跑）：

```text
tier: basic provider: minimax
```

**預期結果**：寫一份設定就讀回來，不碰 `core`、`runtime` 或任何 adapter。

如果這步不通過，先確認：

```bash
cd /Users/shuk/projects/ai/agentSDK
go build ./...
go list -deps ./agent/spec | grep agentsdk   # 應該只有 core + agent/spec 兩行
```

## 步驟 2 (Step 2)：理解 tier 階梯

`tier` 是 engagement 的階梯——不是一堆獨立開關，而是單調包含的四階。每一階都是下一階的子集。

| tier | 內容 | 何時用 |
| --- | --- | --- |
| `oneshot` | 只有 provider，單次 `Generate` | 嵌在別人服務裡跑一次分類 / 摘要 |
| `basic` | `+` reasoning loop、middleware、state/WAL | 有記憶的對話 agent |
| `standard` | `+` 內建工具、permission、session、context files | 會動檔案的工作 agent |
| `full` | `+` skills、subagents、stream 輸出 | 完整 coding agent |

```bash
go run . w -y --tier oneshot  -o -     # 5 個 key，無 name，無 memory
go run . w -y --tier standard -o -     # 多 safety / tools / sessions / prompt
```

四階是單調包含（測試見 `agent/spec/spec_test.go::TestExpandTierLadderIsMonotonic`）：高階不能丟掉低階已有的 block——`oneshot` 與 `full` 的 `Middleware` 都得有，差別在 `full` 多加 `Skills` 與 `Subagents`。

**這條很關鍵**：設定的「開關」不是 10 個 `bool`，是一個 `tier` 字串。展開後仍有顯式覆寫—— `tier: basic` 加上 `memory: {store: none}` 表示「basic 的 middleware 預設我不要，持久化我也關掉」。

### 兩階展開的對照

把兩個 tier 跑 `wizard -y -o -` 的結果並排，看差異落在哪：

```yaml
# tier: oneshot（展開後）
model: {provider: minimax}
reasoning: {style: think_then_act, enable: [think_then_act]}
limits: {autonomy: L2, max_turns: 2}        # ← oneshot 給 2 turn
# —— 沒有 middleware / memory / tools / safety / prompt / sessions
# —— 沒有 name（persist 是 nil）

# tier: basic（展開後，差異以 ⬇ 標出）
model: {provider: minimax}                  # ⬇ 多了 middleware
reasoning: {style: think_then_act, enable: [think_then_act]}
limits: {autonomy: L2, max_turns: 20}       # ⬇ basic 給 20
middleware: {preset: default}               # ⬇ 預設 chain（retry→timeout→budget→loopguard）
memory: {store: file, compaction: none}     # ⬇ 開持久化 → name 現在必填
# ⬇ 仍無 tools / safety —— 沒有要管的東西

# tier: standard（再 +）
tools: {}                                   # ⬇ 6 個內建工具全開
safety: {fallback: autonomy, mode: default, sandbox: true}  # ⬇ 加權限
prompt: {project_dir: .agentsdk, sources: [files]}         # ⬇ 加 context files
sessions: {}                                # ⬇ 加 session lineage
middleware: {preset: secure}                 # ⬇ standard 起自動換 secure（含 sandbox/approval）
limits: {autonomy: L2, max_turns: 40}       # ⬇ standard 給 40
# ⬇ 仍無 skills / subagents

# tier: full（再 +）
skills: {}                                  # ⬇ 加 progressive-disclosure
subagents: {max_depth: 1, max_turns: 10}    # ⬇ 加 task tool
output: {format: text}                      # ⬇ 預設 text sink（由前端接）
prompt: {sources: [files, skills, env, reminder]}  # ⬇ 開 env 與 reminder
```

規律：`tier=oneshot` 是「極簡呼叫」，其後每上一階新增一組 block，沒有覆寫。

### 為什麼不直接叫 `engagement`

`tier` 借自 `core.NewDecide` 與 `core.ReasoningStyle` 的對應層——它本來就表示「這個 plan 跑哪一階」。skeleton 沿用這個詞，是要讓設定層與核心層共享同一份心智模型。如果哪天 core 改名，整個設定詞彙也跟著改——這是刻意的耦合，避免「『one-shot』與『REASON_ONE_SHOT』是兩件事」這種漂移。

## 步驟 3 (Step 3)：用 `MustNew` 建 Agent

```go
package main

import (
    "github.com/bizshuk/agentsdk/app"
    "github.com/bizshuk/agentsdk/agent"
)

func main() {
    app.Main(agent.MustNew(agent.Config{
        Name:  "greeter",
        Tier:  "basic",
        Model: agent.Model{Provider: "minimax"},
    }))
}
```

`agent.MustNew` 做兩件事：

1. `Prepare()`：tier 展開 + 驗證，**不**碰 `core` 與 `runtime`
2. 把注入 (`agent.Option`) 收集起來，等 `app.Run` 給 `AppConfig` 後再走 `Bootstrap` 真正組裝

所以應用層呼叫 `MustNew` 不會建目錄、不打 provider、不驗憑證。失敗只有 schema 錯誤（不會動到磁碟的幾種）。

`Bootstrap` 會做：

- 建 `core.NewDecide`，註冊 `Reasoning.Enable` 列出的策略
- 走 8 stage pipeline：provider → tools → reasoning → prompt → safety → memory → output → assemble
- 回傳 `*runtime.Engine` + opening `core.State`

### 為什麼拆 New 跟 Bootstrap

時序：

```text
main()                          app.Run()
─────────                       ─────────
agent.MustNew(cfg)              config.OpenForCLI(name)
  ├ cfg.Prepare()                  ├ 開 ~/.config/<name>
  ├ option.apply()                 ├ FileStore + WAL
  └ return *Agent                  └ return *AppConfig
                                  ┌──────────────────────────────┐
                                  │ a.Bootstrap(ctx, AppConfig)  │
                                  ├ stage 1: provider           │
                                  ├ stage 2: tools              │
                                  ├ ...                          │
                                  ├ stage 8: assemble *Engine   │
                                  ├ run Engine.Run(ctx, state)  │
                                  └ return state                │
```

New 早於任何 I/O——這讓它在測試裡建構 `Agent` 不會留 side effect；壞設定讓 `main` 立刻 panic，沒有建出半套目錄。Bootstrap 晚於 `OpenForCLI`，因為它需要 data dir 與 state store。預期的副作用（建目錄、打 provider、開 log）都在 `Bootstrap` 與 `Engine.Run` 之間發生。

`Agent` 還有 `Preflight(ctx, ac)`：在 `Bootstrap` 之前先 build provider，credential 錯誤會在打 store 之前爆。實作見 `agent/build.go`，就是一個 `resolveProvider` 呼叫。

### 8 stage pipeline 的順序約束

`agent/build.go::Bootstrap` 的 doc comment 把每一條約束寫出來。摘錄：

| 順序 | 約束 | 理由 |
| --- | --- | --- |
| provider 在 tools 之前 | `subagent.Spawner.RunFunc` 需要已建好的 provider 才能開 sub-engine | tools 階段已經在產生 task tool |
| prompt 在 tools/skills 之後 | `skill.Registry.SystemPrompt()` 是 system message 的一部分 | prompt 階段要合併 skill index |
| safety 同一個 `*permission.Engine` 餵 `Engine.Approval` 與 `config.SecureMiddleware` | 兩個獨立實例對同一次呼叫的判斷會分歧 | safety 階段只建一次 |
| assemble 在最後 | `Customize` 必須看得到組好但未跑過的 engine | 逃生艙 |

任一條違反，型別不會擋，runtime 會在第一個使用點 panic。`TestBootstrapWiresBlocksToEngineFields` 鎖住每個 block 對應的 Engine 欄位。

## 步驟 4 (Step 4)：把設定檔帶進 main

把步驟 3 的寫法替換成 `LoadFile`：

```go
package main

import (
    "github.com/bizshuk/agentsdk/agent"
    "github.com/bizshuk/agentsdk/app"
)

func main() {
    cfg, err := agent.LoadFile("agent.yaml")
    if err != nil { panic(err) }
    app.Main(agent.MustNew(cfg))
}
```

讀寫兩端是同一個型別。`wizard` 產的 YAML 可直接被 `LoadFile` 讀回來——round-trip 測試在 `agent/spec/spec_test.go::TestEncodeDecodeRoundTripIsFixedPoint`。

擴充格式：

```yaml
name: greeter
tier: basic
model: {provider: minimax, name: MiniMax-M3}
persona: "you are terse; reply in one sentence"
reasoning: {style: plan_then_run}
safety:
  mode: acceptEdits
  deny: ["bash(sudo:*)"]
  ask:  ["bash(git push:*)"]
prompt:
  sources: [files, env]
memory: {compaction: headline}
```

YAML 與 JSON 共用同一份 `json` tag（轉譯在中間發生，`agent/load.go`），所以新增欄位時只需要在 `agent/spec/spec.go` 改一處。

## 步驟 5 (Step 5)：處理無法寫進設定的東西

有些東西寫不進 YAML（也不該寫）：

| `agent.Option` | 為什麼不能寫進設定 |
| --- | --- |
| `WithProvider` | 活物件（測試 fake、已建好的 client）；API key 是密鑰，不該進設定檔 |
| `WithTools` | 應用自有工具的實作 |
| `WithHooks` | closure 安全閘：要看實際參數內容，任何 specifier pattern 都做不到 |
| `WithSources` | 自訂 prompt 內容來源 |
| `WithRules` | 超出內建六個的推理策略 |
| `WithSink` / `WithNotifier` | 呈現與通知的實作 |
| `WithCustomize` | 最終逃生艙：拿到組好的 `*runtime.Engine` 再改 |

### 每個 Option 的真實用法

從 `sample/code-agent` 與測試取出來的實際 closure 形狀：

| Option | 用途 | 例子 |
| --- | --- | --- |
| `WithProvider` | 注入 fake provider 跑測試 | `agent.WithProvider(testutil.NewScriptedProvider())` |
| `WithTools` | 加應用自有工具（部署、回滾、custom API call） | `agent.WithTools(myDeployTool)` —— 實作 `core.Tool` 介面的 5 個方法即可 |
| `WithHooks` | 阻擋高風險呼叫 | 見下面完整範例 |
| `WithSources` | 自訂 prompt 來源 | `agent.WithSources(prompt.Static(prompt.SLOT_SYSTEM, "brand", "語氣：簡潔", prompt.ORDER_PERSONA))` |
| `WithRules` | 註冊自訂推理策略 | 實作 `core.DecisionRule`（`Kind() ReasoningStyle` + `NextStep(state)`），`Kind` 會贏過同名的內建 rule |
| `WithSink` | 接自訂呈現 | `agent.WithSink(agent.SinkFunc(func(ev core.StreamEvent) { ... }))` |
| `WithNotifier` | 接通知系統 | 配合 `auth/notify` 或外部服務 |
| `WithCustomize` | 拿到組好的 Engine 再改 | `agent.WithCustomize(func(e *runtime.Engine) error { e.Store = myCustomStore; return nil })` |

> 後面兩個範例（closure 阻擋 `rm -rf` 與「失敗/正確」對照）刻意只給部分程式碼——讀懂「Option 的 closure body 長怎樣」就夠了；完整可執行的版本見 `sample/code-agent/cmd/safety.go::blockDestructiveBash`。

範例——`code-agent` 用 closure 阻擋 `rm -rf /`：

```go
import (
    "context"
    "strings"

    "github.com/bizshuk/agentsdk/agent"
    "github.com/bizshuk/agentsdk/app"
    "github.com/bizshuk/agentsdk/core"
    "github.com/bizshuk/agentsdk/hook"
)

func main() {
    cfg, _ := agent.LoadFile("agent.yaml")

    app.Main(agent.MustNew(cfg,
        agent.WithHooks(hook.Rule{
            Event: core.HOOK_PRE_TOOL_USE, Match: "bash",
            Handlers: []hook.Handler{hook.Func(func(_ context.Context, ev core.HookEvent) (core.HookDecision, error) {
                cmd, _ := ev.ToolCall.Args["command"].(string)
                if strings.Contains(cmd, "rm -rf /") {
                    return core.HookDecision{Block: true, Reason: "refusing rm -rf on root-ish path"}, nil
                }
                return core.HookDecision{}, nil
            })},
        }),
    ))
}
```

`WithHooks` 是 functional option（`func(*builder) error`），不是資料。`spec.Choice` 才是資料，寫進 YAML，跨 process 讀回。決定性的一條是`可列舉`：wizard 要先枚舉候選才能讓人選，closure 無法自我描述。

## 步驟 6 (Step 6)：用 wizard 產設定檔

`agentsdk w`（alias `w`）逐階段產生 `Config`，階段順序刻意對齊 build pipeline：

```bash
cd /Users/shuk/projects/ai/agentSDK
go run . w                              # 互動，預設寫 ./agent.yaml
go run . w -y --tier full -o -          # 非互動，輸出 stdout
go run . w --edit existing.yaml         # 以既有設定當預設值升級
go run . w --list model.provider        # 列 provider 選項
go run . w -y -o - --print-go           # 同時印 Go literal
```

Wizard 的選項來源是 `spec.TierChoices` / `spec.StyleChoices` / `spec.VariantChoices` / `agent.ProviderChoices`——wizard 本身不持有詞彙。新增 strategy 或 provider 不需要改 wizard。

## 步驟 7 (Step 7)：驗證骨架行為

骨架的所有不變式都有測試鎖住。下面是最常被違反的四條：

```bash
cd /Users/shuk/projects/ai/agentSDK

# 1. 兩層宣告層只該 import core
go list -deps ./agent/spec | grep agentsdk   # 應該只有 core 與 agent/spec
go list -deps ./prompt     | grep agentsdk   # 應該只有 core 與 prompt

# 2. tier 單調包含
go test ./agent/spec/... -run TestExpandTierLadderIsMonotonic -count=1 -v

# 3. 一路 Enter 預設必產出合法設定
for t in oneshot basic standard full; do
    go run . w -y --tier $t -o - >/dev/null && echo "✅ $t"
done

# 4. 加了未註冊 strategy 會被擋下（structural 防呆）
go run . w -y --tier basic -o /tmp/x.yaml
go run . w --edit /tmp/x.yaml -y -o - >/dev/null   # round-trip 不失真
```

如果第 `1` 條多出別的依賴——恭喜，有人加了個 leak，順手把它解決。第 `3` 條跑不過代表有個 block 的預設值不合法，呼叫端會被坑。

## 進階：直接打 Engine

有些應用需要 driver loop（例如互動 CLI 自己管 steer / follow-up queue）——它們要 Engine 不要 lifecycle：

```go
// 示意：完整 driver loop，缺少 *runtime.Engine 與真實 state；
//       請把 `parts.Engine` 與 `state` 換成 Bootstrap 回傳的實例
parts, err := agent.New(cfg).Bootstrap(ctx, appCfg)
if err != nil { return err }
engine := parts.Engine
state := /* parts.Config + seeded messages */

for {
    final, err := engine.Run(ctx, state)
    // 自己的迴圈邏輯
    state = final
}
```

`agent.Parts` 暴露：

| 欄位 | 用途 |
| --- | --- |
| `Engine` | 給 driver loop 直接跑 |
| `Sessions` | session lineage（`-c` / `-r` / `--fork` 續跑） |
| `Skills` | 互動命令介面（`/help` 之類） |
| `Prompt` | `prompt.Builder`，可呼叫 `Turn()` 拿 reminder slot |
| `Config` | 展開後的 `Config`，debug 用 |
| `AppConfig` | `config.AppConfig`，給自訂工具需要 `~/.config/<name>` 時 |

`sample/code-agent/cmd/compose.go` 的 101 行就是這套：`MustNew` → `Bootstrap` → 用 `Parts.Sessions`、`Parts.Engine`、`Parts.AppConfig` 拼 headless / 互動 / `--json` 三種模式。

## 設計原則速記

- **presets, not walls**：設定挑 preset 而非組合細節（middleware 鏈順序是正確性）；`WithCustomize` 是逃生艙
- **宣告與組裝分離**：宣告層只看 core，組裝層才知道有那些實作
- **`prompt` vs `memory`**：`prompt` 決定`放什麼進 context window`（policy），`memory` 決定`放不下時砍什麼`（mechanism）；合在一起會遞迴
- **`T0` 走 Engine**：不要為 `oneshot` 寫 no-op Decide——`planning.OneShotReasoning` 已經是這個東西，且不違反 `core.Decide` 的純函式不變式
- **`tier × reasoning` 正交**：`oneshot` 同時給 `reasoning` 是合法的；無工具 → model 不發 tool call → engine short-circuit 後，無論哪個 strategy 都退化成一次呼叫

## 常見錯誤 (Common Mistakes)

每條都給「失敗示範 → 正確示範」對照——只看症狀看不出該怎麼改。

> 下面對照的程式碼片段是說明用的，不是 paste-and-run 的模板：`go` 區塊可能省略 import 或省略非必要的型別宣告，焦點放在「哪個字打錯、哪個欄位寫在哪裡」。要跑就從 `sample/code-agent/cmd/safety.go` 或對應的 `*_test.go` 抄。

### 1. 把密鑰寫進設定檔

```yaml
# 失敗：API key 進 git,進 shell history,進 build log
model:
  provider: anthropic
  api_key: "sk-ant-xxxxxxxxxxxxxxxx"      # ❌ 違反 secret 不落盤的原則
```

```go
// 正確：API key 走程式注入（env 或密鑰管理），spec 只指名變數名
// yaml：model: {provider: anthropic, api_key_env: ANTHROPIC_API_KEY}
// 程式碼：
app.Main(agent.MustNew(cfg,
    agent.WithProvider(anthropicprovider.New(anthropicprovider.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")))),
))
// 或更乾淨：寫 env 到 process env，registry.LookupEnv 直接撈
```

`registry.Options.APIKey` 是 `Options` 欄位，刻意不暴露成 `spec.Model` 欄位——這條邊界守住了才能讓設定檔安全丟 git。

### 2. 把冗餘 block 一起打開

```yaml
# 失敗：tier=oneshot 已經沒有工具了，safety 也沒東西可管
tier: oneshot
model: {provider: minimax}
safety: {mode: default, deny: ["bash(sudo:*)"]}    # ❌ 沒東西可管
```

```yaml
# 正確：oneshot 不開 safety（沒東西可 gate）；需要 gate 時升到 standard
tier: oneshot
model: {provider: minimax}
# 或：tier: standard + tools: {} + safety: {...}
```

`Validate` 不會擋這個——它是「無害但不必要」的浪費，不是錯誤。把這個當作 code-review smell：看到 `tier: oneshot` 配 `safety` 就該問「為什麼」。

### 3. 自己呼叫 `Validate()` 跳過 `Expand()`

```go
// 失敗：Spec.Validate 是 EXPANDED config 的檢查；直接驗 raw 會誤報
cfg := spec.Config{Name: "x", Tier: "basic"}
err := cfg.Validate()    // ❌ "middleware.preset is empty — call Expand before Validate"
```

```go
// 正確：用 Prepare 串好 Expand + Validate
cfg, err := spec.Config{Name: "x", Tier: "basic"}.Prepare()
// 會得到 fully-expanded Config，且一次報完所有問題
```

錯誤訊息本身有提示——`call Expand before Validate`——但很多人看到會以為自己的 spec 寫錯而開始改設定。實際上是呼叫順序錯了。

### 4. `Customize` 改寫 nil port 變成空 registry

```go
// 失敗：把 nil 改成空 registry = 沒工具 → 但要跑一圈才發現沒工具可呼叫
agent.WithCustomize(func(e *runtime.Engine) error {
    if e.Tools == nil {
        e.Tools = action.NewRegistry()     // ❌ 模型收到 []ToolSpec，白繞一圈
    }
    return nil
})
```

```go
// 正確：nil 就是「沒工具」；Customize 該加東西，不是補空集合
agent.WithCustomize(func(e *runtime.Engine) error {
    e.Store = myCustomStore                  // ✅ 改非 nil 欄位
    return nil
})
```

加工具用 `WithTools`，加 hook 用 `WithHooks`——`Customize` 留給設定詞彙沒覆蓋的零碎接線。

### 5. 加了策略但忘了 wizard 同步

```text
// 失敗：在 core 與 planning 加完，回 spec 加 Choice，忘記更新 wizard
// core/thinking.go: REASON_REFLECT = "reflect"
// planning/reflect.go: NewReflectiveReasoning()
// agent/spec/choice.go: StyleChoices() 不含 "reflect"
```

```bash
# 症狀：wizard --list reasoning.style 看不到新策略，但 NewDecide 不報錯
# 因為 core/thinking.go 已宣告，Validate 通過，但 wizard 給人選的清單少一項
```

```text
// 正確：三處同步（且 TestStyleChoicesMatchCoreConstants 防呆）
// 1. core/thinking.go：新增 ReasoningStyle 常數
// 2. planning/reflect.go：實作 DecisionRule
// 3. agent/spec/choice.go：StyleChoices() 加一筆 Choice
```

骨架的三層（`core` enum → `planning` 實作 → `spec` Choice）必須同時加，缺一就漂移。`TestStyleChoicesMatchCoreConstants` 是這條的編譯期契約。

## 後續閱讀

- [`docs/tutorials/01-getting-started.md`](01-getting-started.md)：core / runtime / planning / action 的底層概觀
- [`plans/2026-07-22-agent-skeleton-config-opt-in.md`](../../plans/2026-07-22-agent-skeleton-config-opt-in.md)：骨架的設計權衡與決策紀錄
- [`docs/terminology.md`](../terminology.md)：本教學出現的所有術語的單一定義
- `agent/spec/spec_test.go`：所有不變式的測試（62 個 subtest）
- `sample/code-agent/cmd/compose.go`：101 行的完整應用案例

## 術語解釋 (Terminology)

本教學出現的所有術語都已收錄在 [`docs/terminology.md`](../terminology.md)，按首次出現順序：

- `tier`、`feature block`、`variant`、`Choice`、`Option`、`Style`、`Enable`、`Build pipeline`、`Persona`、`Source`、`Slot`、`Resolver`、`LookupEnv`、`Once`、`explicit wins`、`monotonic`

首次出現以 `backtick` 高亮，後文沿用，不另立同義詞。
