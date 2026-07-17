# Wire Format Catalog Implementation Plan

> `執行方式`：本 session 直接依序完成，不委派 sub-agent。

`目標`：從 `auth2api`、`CLIProxyAPI`、`agentSDK master` 與 `agentSDK current` 的程式碼，建立可追溯的 client/provider wire-format catalog。

`架構`：以 `client wire format + provider wire format` 作為 entity。每個 entity 使用獨立目錄，並以不同檔名記錄文字訊息、圖片輸入、工具循環、串流與錯誤的 client send/provider receive/provider send/client receive 格式。

`技術`：Markdown、JSON、SSE、Connect-RPC/Protocol Buffers wire notation、Go/TypeScript source evidence。

## 全域限制

- 只記錄四個已指定來源能證明的格式與組合。
- vendor/profile 若重用相同 wire format，只在索引建立 mapping，不複製 entity。
- JSON 範例只涵蓋 source code 實際讀寫的欄位，不宣稱是完整 vendor API schema。
- `Cursor Connect-RPC` 的圖片與工具格式依來源標示為 unsupported，不虛構 payload。
- 不修改既有使用者變更中的 `README.todo` 與 `docs/specs/2026-07-16-pairwise-agent-provider-transform.md`。

## Task 1：來源與組合索引

`檔案`

- 建立：`docs/specs/format/README.md`

`輸入`

- `auth2api` main：`37c9b864b07939ec8626ce1e3a1554fca5ae01db`
- `CLIProxyAPI` main：`411d7d41eee0ff841b5badba5abacad6c11331ef`
- `agentSDK` master：`e7edfc7cc84918c71abfa78c905519bbb6de793f`
- `agentSDK` current：`39a913cb2e69ec988aa82e1acaf18e84dc951871`

`產出`

- `37` 個 entity 的矩陣。
- client alias、provider format、concrete vendor/profile mapping。
- snapshot、scope、coverage 與限制。

## Task 2：Entity 格式檔

每個 `docs/specs/format/<client>__<provider>/` 建立以下檔案：

- `README.md`：entity identity、來源證據、方向與檔案索引。
- `chat-message.md`：client send → provider receive；provider send → client receive。
- `chat-message-with-image.md`：圖片輸入的 client/provider request；不支援時明確記錄。
- `tool-call.md`：provider tool call → client tool call → client tool result → provider tool result。
- `stream.md`：provider stream → client stream 的代表性 frame sequence。
- `error.md`：provider error → client error。
- `provider-normalization-variants.md`：只在五個 Codex provider entity 建立，記錄四來源 normalization 差異。

Entity 目錄：

```text
anthropic-messages__anthropic-messages
anthropic-messages__openai-chat
anthropic-messages__openai-responses
anthropic-messages__gemini-generate-content
anthropic-messages__google-interactions
anthropic-messages__openai-codex-responses
anthropic-messages__antigravity
anthropic-messages__cursor-connect-rpc
openai-chat__anthropic-messages
openai-chat__openai-chat
openai-chat__openai-responses
openai-chat__gemini-generate-content
openai-chat__google-interactions
openai-chat__openai-codex-responses
openai-chat__antigravity
openai-chat__cursor-connect-rpc
openai-responses__anthropic-messages
openai-responses__openai-chat
openai-responses__openai-responses
openai-responses__gemini-generate-content
openai-responses__google-interactions
openai-responses__openai-codex-responses
openai-responses__antigravity
openai-responses__cursor-connect-rpc
gemini-generate-content__anthropic-messages
gemini-generate-content__openai-chat
gemini-generate-content__gemini-generate-content
gemini-generate-content__google-interactions
gemini-generate-content__openai-codex-responses
gemini-generate-content__antigravity
google-interactions__anthropic-messages
google-interactions__openai-chat
google-interactions__openai-responses
google-interactions__gemini-generate-content
google-interactions__google-interactions
google-interactions__openai-codex-responses
google-interactions__antigravity
```

## Task 3：專案脈絡同步

`檔案`

- 修改：`CLAUDE.md`

`產出`

- 在 project structure 與文件說明加入 `docs/specs/format/` catalog。

## Task 4：驗證

執行：

```bash
find docs/specs/format -mindepth 1 -maxdepth 1 -type d | wc -l
find docs/specs/format -mindepth 2 -maxdepth 2 -type f | wc -l
find docs/specs/format -mindepth 1 -maxdepth 1 -type d -exec test -f '{}/README.md' ';'
find docs/specs/format -mindepth 1 -maxdepth 1 -type d -exec test -f '{}/chat-message.md' ';'
find docs/specs/format -mindepth 1 -maxdepth 1 -type d -exec test -f '{}/chat-message-with-image.md' ';'
find docs/specs/format -mindepth 1 -maxdepth 1 -type d -exec test -f '{}/tool-call.md' ';'
find docs/specs/format -mindepth 1 -maxdepth 1 -type d -exec test -f '{}/stream.md' ';'
find docs/specs/format -mindepth 1 -maxdepth 1 -type d -exec test -f '{}/error.md' ';'
git diff --check
```

預期：`37` 個 entity directory、每個 entity `6` 個基本檔案，加上 `5` 個 Codex normalization variant 檔；沒有 whitespace error，來源矩陣與實際目錄雙向一致。
