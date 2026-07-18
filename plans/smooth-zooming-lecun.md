# Plan: Codex OAuth semantic loss warning cleanup

## Context

`proxy` currently logs every transform `SemanticLoss` at `WARN` level. In the Anthropic Messages → OpenAI Responses path used by `openai-codex-oauth`, common Claude-style request fields produce noisy warnings:

- `messages.content.tool_result.is_error`
- `messages.content.cache_control`
- `system.cache_control`
- `max_tokens`
- `metadata`

Two of these are not inherently semantic losses for OpenAI Responses: `max_tokens` can be represented as `max_output_tokens`, and `metadata` can be preserved as Responses `metadata`. The remaining cache-control and tool-result error fields are real format losses, but for the Codex OAuth bridge they are expected compatibility noise and should not page the logs as warnings. The intended outcome is to keep diagnostics honest while reducing noisy `WARN proxy transform semantic loss` output for the known Codex path.

## Recommended approach

Implement a two-part fix:

1. Represent portable fields in the OpenAI Responses DTO and transform output.
2. Demote only the known Codex-only expected losses from `WARN` to a lower slog level, without removing them from transform results or metrics.

This keeps transform semantics correct and avoids hiding real losses for other providers or future profiles.

## Implementation steps

### 1. Extend OpenAI Responses request DTO

Modify `proxy/model/responses/types.go`:

- Add `MaxOutputTokens int `json:"max_output_tokens,omitempty"`` to `responses.Request`.
- Add `Metadata json.RawMessage `json:"metadata,omitempty"`` to `responses.Request`.
- Reuse the existing `encoding/json` import already present in this file.

Rationale: `responses.Request` currently exposes model/input/instructions/stream/store/tools/tool choice/reasoning/parallel tool calls, but not fields that can carry Anthropic `max_tokens` and request metadata.

### 2. Preserve `max_tokens` and `metadata` in Anthropic → Responses transform

Modify `proxy/svc/transform/anthropic_responses_request.go`:

- In `AnthropicToResponsesRequest`, populate:
  - `MaxOutputTokens: src.MaxTokens`
  - `Metadata: src.Metadata`
- In `task5AnthropicRequestLosses`, remove loss generation for:
  - `max_tokens`
  - `metadata`
- Keep existing losses for fields still not faithfully represented:
  - `thinking.budget_tokens`
  - `temperature`
  - `top_p`
  - `stop_sequences`
  - message/system `cache_control`
  - `messages.content.tool_result.is_error`

Rationale: once `max_tokens` and `metadata` are encoded into the Responses request body, warning about them as losses is incorrect.

### 3. Add scoped Codex loss log-level classification

Modify `proxy/handlers/observability.go`:

- Keep `TransformObserver` unchanged.
- Replace `WarnContext` inside `RecordLoss` with `LogAttrs` or equivalent slog API so level can vary.
- Add a helper such as `lossLogLevel(provider string, source, target model.Format, field string) slog.Level`.
- Return lower severity (`slog.LevelInfo`, or `slog.LevelDebug` if existing log policy prefers quieter output) only when all conditions match:
  - `provider == "openai-codex-oauth"`
  - `source == model.FORMAT_ANTHROPIC_MESSAGES`
  - `target == model.FORMAT_OPENAI_RESPONSES`
  - `field` is one of:
    - `messages.content.tool_result.is_error`
    - `messages.content.cache_control`
    - `system.cache_control`
- Return `slog.LevelWarn` for every other loss.
- Keep `o.losses.Add(...)` unchanged so metrics still count all semantic losses.

Rationale: the transform layer still reports true losses; observability decides that these exact Codex bridge losses are expected and should not be warning-level noise.

### 4. Add transform regression tests

Modify `proxy/svc/transform/anthropic_responses_request_test.go`:

- Add a targeted test for Anthropic Messages → OpenAI Responses with:
  - `max_tokens`
  - `metadata`
  - system `cache_control`
  - message content `cache_control`
  - `tool_result` with `is_error: true`
- Assert output JSON includes:
  - `max_output_tokens`
  - `metadata`
- Assert `result.Losses` does not contain:
  - `max_tokens`
  - `metadata`
- Assert `result.Losses` still contains:
  - `messages.content.tool_result.is_error`
  - `messages.content.cache_control`
  - `system.cache_control`

### 5. Add DTO round-trip coverage

Modify or add tests under `proxy/model/responses/`:

- Verify `responses.DecodeRequest` and `responses.Encode` preserve:
  - `max_output_tokens`
  - `metadata`

Representative file: `proxy/model/responses/types_test.go` if present; otherwise add a small package test in the same directory.

### 6. Add observability classification tests

Modify or add `proxy/handlers/observability_test.go`:

- Prefer direct unit tests for the helper to avoid brittle log parsing.
- Assert Codex + Anthropic Messages → OpenAI Responses + known expected field returns the lower level.
- Assert another provider or another field returns `slog.LevelWarn`.

Representative assertions:

- `openai-codex-oauth` + `messages.content.cache_control` → lower level
- `openai-api` + `messages.content.cache_control` → `WARN`
- `openai-codex-oauth` + `temperature` → `WARN`

## Critical files

- `proxy/model/responses/types.go`
- `proxy/svc/transform/anthropic_responses_request.go`
- `proxy/handlers/observability.go`
- `proxy/svc/transform/anthropic_responses_request_test.go`
- `proxy/model/responses/*_test.go`
- `proxy/handlers/observability_test.go`

## Verification

Run targeted proxy tests:

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./model/responses ./svc/transform ./handlers -count=1 -timeout=120s
```

Run full proxy tests:

```bash
cd /Users/shuk/projects/agentSDK/proxy
go test ./... -count=1 -timeout=120s
```

If the implementation touches exported protocol behavior in a way that may affect the workspace, also run root tests:

```bash
cd /Users/shuk/projects/agentSDK
go test ./... -count=1 -timeout=120s
```

Optional full workspace module verification from `CLAUDE.md`:

```bash
cd /Users/shuk/projects/agentSDK
for mod in auth proxy utils/video mcp provider/anthropic provider/google provider/openaicompat \
  sample/file-agent sample/greet-agent sample/logdoctor \
  sample/memory-demo sample/middleware-demo sample/strategy-demo; do
  (cd "$mod" && go test ./... -count=1 -timeout=120s)
done
```

## Non-goals

- Do not suppress all semantic losses globally.
- Do not remove loss records from `TransformResult` for actual unrepresented fields.
- Do not put provider-specific diagnostic suppression in `svc/upstream/profile.go`; the normalizer runs after transform diagnostics are produced and should stay focused on provider request normalization.
- Do not document `/admin/*` or other unrelated proxy surfaces as part of this change.
