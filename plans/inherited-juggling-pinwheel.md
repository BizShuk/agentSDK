# Plan: SDK-level `WithAppName` fail-fast + sample wiring helper

## Context

目前 `sample/greet-agent/cmd/root.go` 每次都手抄同一段 boilerplate:

1. `config.Default(config.WithAppName(appName))` 初始化
2. 讀 `config.GetAppDataDir()` / `GetAppLogDir()`
3. nil-check (而且是壞的 — `GetAppDataDir()` 在未初始化時回 `"data"` 不是 `""`)
4. `os.MkdirAll(states)` + `os.MkdirAll(wal)` + 開 log 檔
5. `filestore.NewFileStateStore(dataDir)` + `NewFileWAL(dataDir)` 注入 `loop`
6. 啟動 `slog` file handler

這六步每次都要重複,新 sample (`sample/logdoctor/run.go` 之後) 一定會再貼一次。
而且 sample 端的 nil-check `dataDir == ""` 實際是 no-op — 真正該檢查的 sentinel
是 `config.GetAppName() == ""`(見 `/Users/bytedance/projects/tmp/gosdk/config/config.go:97-110`,
未初始化時 `GetAppDataDir()` 經 `filepath.Join("", "data") == "data"`)。

`gosdk` 是框架層 (`~/projects/gosdk`),被 `agentsdk/sample/*` 引用,
CLAUDE.md:76「`core/` 純 stdlib」是 `core/` 的規則不是 `runtime/` 的;
`runtime/` 已經 import `middleware`,再 import `gosdk/config` 是同一層級的依賴,
合規。

## 目標

把這六步下沉到 `agentsdk/runtime/`,給 sample 一個
`runtime.MustOpenForCLI(appName, logger)` API,內部完成所有 wiring 並 fail-fast。

## 設計

### 新 API: `runtime.MustOpenForCLI`

```go
// runtime/app.go (新檔)
package runtime

// AppDirs 是一次性 wiring 的結果;sample 把它的欄位塞進 loop。
type AppDirs struct {
    DataDir  string  // ~/.config/<appName>/data
    LogDir   string  // ~/.config/<appName>/log
    RunID    string  // UnixNano
    LogFile  string  // <LogDir>/<RunID>.log
}

// MustOpenForCLI 為 CLI 樣本一次做完:config 初始化、mkdir、log 開檔、
// filestore 建好。把結果回傳,讓 caller 把它塞進 loop。
// 若 WithAppName 未呼叫或 appName 為空,直接 panic — 這是 programmer error
// 不是 user input error,呼叫端必為開發者。
//
// 範例:
//
//     dirs := runtime.MustOpenForCLI("greet-agent", slog.LevelInfo)
//     defer os.Remove(dirs.LogFile) // optional cleanup
//     loop := runtime.NewLoop(step, model, tools)
//     loop.Store, _ = filestore.NewFileStateStore(dirs.DataDir)
//     loop.WAL,   _ = filestore.NewFileWAL(dirs.DataDir)
func MustOpenForCLI(appName string, level slog.Level) *AppDirs
```

### 內部行為

1. **`config.Default(config.WithAppName(appName))`** — 確保 `GetAppName()` 之後非空。
2. **sentinel 檢查** — `config.GetAppName() != ""`(`config.GetConfigDir()` 同義,
   都依賴 `appName` 全域變數)。空字串代表 caller 沒呼叫 `Default(WithAppName(...))`,
   此時 panic with `runtime: gosdk/config 未初始化 (請先呼叫 config.Default(config.WithAppName(appName)))`。
3. **`config.GetAppDataDir()` + `GetAppLogDir()`** — 這兩個函式 `WithAppName` 後
   一定回非空,毋需 nil-check。
4. **`os.MkdirAll(<dataDir>/states)` + `os.MkdirAll(<dataDir>/wal)`** —
   對應 `filestore.NewFileStateStore/NewFileWAL` 期待的目錄。
5. **`os.MkdirAll(<logDir>)`** + `os.OpenFile(<logDir>/<runID>.log, O_APPEND|O_CREATE, 0o640)`。
6. **`slog.SetDefault(slog.NewJSONHandler(f, ...).With(slog.String("runID", runID)))`**。

### Sample 端改動

`sample/greet-agent/cmd/root.go` 從 24 行 boilerplate 縮成 4 行:

```go
dirs := runtime.MustOpenForCLI(appName, slog.LevelInfo)
loop := runtime.NewLoop(step, model, tools)
loop.Store, _ = filestore.NewFileStateStore(dirs.DataDir)
loop.WAL,   _ = filestore.NewFileWAL(dirs.DataDir)
```

拿掉:
- `import "github.com/bizshuk/gosdk/config"` ← 不再需要
- `resolveDataDir` / `resolveLogDir` / `parseLogLevel` / `initFileLogger` ← 整段刪掉
- `dataDir == "" || logDir == ""` 那段 (用 `GetAppName()` 做 sentinel 的 panic 取代)
- `initFileLogger(logPath, runID)` ← MustOpenForCLI 內部做掉

## 為什麼 panic 而不是 `return error`

`MustOpenForCLI` 是給 CLI 啟動時呼叫,等同 cobra `Execute()` 的入口。
失敗一定是 programmer error(忘了 `WithAppName`),立刻 panic
比 `return error` 然後 sample 端忘記檢查好 — 跟 `regexp.MustCompile` / `template.Must`
是同一個慣例。`OpenForCLI` (非 Must) 版本另開,給 library 使用者。

## 受影響的檔案

| 檔案 | 動作 |
|---|---|
| `runtime/app.go` (新) | `AppDirs` struct + `MustOpenForCLI` + `OpenForCLI` |
| `runtime/app_test.go` (新) | 測 panic 行為、測正常流程產物路徑 |
| `go.mod` (root) | `require github.com/bizshuk/gosdk v0.0.0` + `replace ... => /Users/bytedance/projects/tmp/gosdk` |
| `go.work.sum` | (auto) 重新 sync |
| `sample/greet-agent/go.mod` | 拿掉 `bizshuk/gosdk` 的 require + replace(已上移至 root) |
| `sample/greet-agent/cmd/root.go` | 縮成 4 行 wiring;拿掉所有 helper |
| `sample/greet-agent/data/*` (執行期產物) | 跑 e2e 驗證路徑不變 |

## 不動的部分

- `runtime/loop.go` 的 dispatch / runWithInput — 不改 `Loop.Store` / `Loop.WAL` 介面
- `core/port.go` — `StateStore` / `WAL` 介面契約不動
- `memory/filestore` — 不改;它仍是預設 adapter
- 11 個 `loop_test.go` / `*_integration_test.go` 的 in-memory 測試 — 不動;
  它們建 `*Loop` 時本來就沒 `Store`/`WAL`,也不會呼叫 `MustOpenForCLI`

## 驗證

1. **Unit test** (`runtime/app_test.go`):
   - `TestMustOpenForCLIRequiresAppName`:沒呼叫 `WithAppName` 就呼叫 `MustOpenForCLI`
     → 預期 panic,訊息含 `WithAppName`
   - `TestMustOpenForCLIWiresAllDirs`:呼叫 `MustOpenForCLI("test-app", info)` 後
     檢查 `dirs.DataDir` / `dirs.LogDir` / `dirs.RunID` 形如 `~/.config/test-app/data`、`log`、UnixNano
   - `TestMustOpenForCICreatesFiles`:跑完後檢查
     `<DataDir>/states`、`<DataDir>/wal`、`<LogDir>/<RunID>.log` 都已建立
2. **回歸**:
   - `cd /Users/bytedance/projects/agentSDK && go test ./... -count=1 -timeout 30s`
   - 11 個既有 runtime 測試需全綠
3. **E2E** (greet-agent):
   - `rm -rf ~/.config/greet-agent && go run ./sample/greet-agent --name Shuk --max-turns 5`
   - 預期 envelope 序列不變: `call_model → call_tool(greet) → call_model → done`
   - 預期產物路徑不變:
     - `~/.config/greet-agent/data/states/<UnixNano>.json`
     - `~/.config/greet-agent/data/wal/<UnixNano>.jsonl`
     - `~/.config/greet-agent/log/<UnixNano>.log`
4. **Sanity check**:
   - `grep -rn "bizshuk/gosdk" sample/greet-agent/` 應為空
   - `grep -rn "import" runtime/loop.go` 仍只有 core/middleware

## 設計決策紀錄

- 為什麼不把 `Loop` 本身加 `WithAppName` 欄位:Loop 仍要做純單元測試,
  不能綁定 appName;wiring 屬於 sample 端,helper 抽到 runtime 是好折衷。
- 為什麼用 `MustOpenForCLI` 而非 `OpenForCLI` 必填:CLI 入口 99% 失敗就退出,
  panic 提早暴露;library 嵌入場景另開 `OpenForCLI` (error return 版) 即可。
- 為什麼不把 `gosdk/config` 抽成 `core.AppConfig` 介面注入:
  增加一個只有一個人實作的介面是 over-engineering;先直接 import,
  未來有多個 config backend 再抽介面。
