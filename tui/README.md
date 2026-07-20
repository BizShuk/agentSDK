# tui — terminal UI library（root sub-package, zero-dep）

`github.com/bizshuk/agentsdk/tui` 是 pi-tui 式的 terminal presentation library：differential rendering、不用 alternate screen（保留 scrollback）、CSI 2026 synchronized output。zero dependency（純 stdlib），且不 import agentsdk 任何 package —— caller 把 engine 的 `core.StreamEvent` 轉成 component 更新。第一個真 caller 是 [`sample/code-agent`](../sample/code-agent/)（interactive 模式的 transcript 區域即本 package 渲染）。

## 現有能力（skeleton）

- `VisibleWidth` / `TruncateToWidth` / `WrapText`：ANSI-aware 文字量測（CSI 與 OSC sequence 不計寬）。
- `Terminal` 抽象：`ProcessTerminal`（stdout）與 `VirtualTerminal`（headless 測試）。
- `Renderer`：region 差分重繪 —— 首繪全量、frame 未變零輸出、變更時 cursor-up 重繪並清掉縮短的行；全程包在 `?2026` synchronized bracket。
- Components：`Text`（word wrap）、`Loader`（spinner）、`Container`、`Spacer`、`Rule`；`Component` 契約為 `Render(width) []string`，focusable 走 optional `InputHandler`。

## 已知限制（後續補）

- rune 一律算寬 1：CJK double-width 與 grapheme cluster 待補。
- `ProcessTerminal.Size()` 目前讀 `COLUMNS`，ioctl 尺寸與 raw-mode 輸入（Editor、autocomplete、bracketed paste）隨 interactive editor 一起落地。
- 行級 diff（略過未變行）與 `Markdown`、`SelectList`、overlay 系統為下一階段。

## 測試

```bash
# 從 root module 跑（tui 已是 root sub-package）
go test ./tui/... -count=1
```
