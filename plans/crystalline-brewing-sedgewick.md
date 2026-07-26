# Plan: `prompt/source/` sub-package — assemble all built-in Sources in one place

Date: 2026-07-26
Status: planning (not executed)

## Context

`prompt/sources.go` (124 lines) holds four built-in Sources: `PersonaSource`, `ContextFileSource`, `EnvSource`, `ReminderSource`. They are content — they need only `prompt` and the standard library. A fifth Source, `agent.SkillSource`, is the only cross-package adapter: it adapts `*skill.Registry.SystemPrompt()` into a `prompt.Source`. Today it lives in `agent/sources.go` because `prompt` and `skill` must not see each other (CLAUDE.md:114).

We are reorganising so all five Sources live together, while keeping the no-direct-import rule. The seam is a tiny `SkillProvider` interface declared in the new sub-package. `*skill.Registry` satisfies it implicitly, so neither `prompt` nor `prompt/source` needs to import `skill`.

User-confirmed design decisions:

- Directory: `prompt/source/`
- Package name: `source`
- `SkillSource` takes a `SkillProvider` interface, not `*skill.Registry`. No `skill` import in `prompt/source`.

Outcome:

- `prompt/source` depends only on `prompt` + stdlib.
- `agent/sources.go` keeps only `BuildSources` dispatch + the discovery helpers (`promptUserDir`, `discoveryRoots`, `discoverSkills`).
- No sample module is broken (none import these symbols today).
- Dependency rules verifiable: `go list -deps ./prompt/source | grep agentsdk` returns only `prompt/source` itself; `go list -deps ./prompt | grep agentsdk` returns `prompt` + `prompt/source` (no `skill`).

## File layout

New directory `/Users/shuk/projects/ai/agentSDK/prompt/source/`, package `source`:

| File | Responsibility |
| --- | --- |
| `doc.go` | Package doc only (declares the rule: imports only `prompt` + stdlib). |
| `skillprovider.go` | `SkillProvider` interface (`SystemPrompt() string`). |
| `persona.go` | `PersonaSource(string) prompt.Source`. |
| `contextfile.go` | `ContextFileSource(string) prompt.Source` (calls `prompt.LoadContextFiles`). |
| `env.go` | `EnvSource() prompt.Source` + unexported `gitBranch(ctx, dir)`. |
| `reminder.go` | `ReminderSource() prompt.Source`. |
| `skill.go` | `SkillSource(p SkillProvider) prompt.Source` — the cross-package adapter. |

`prompt/source/doc.go` (top of package):

```go
// Package source assembles the built-in content sources that turn a
// prompt.Req into []prompt.Section. Persona, context files, environment,
// and budget reminder are content concerns and need only prompt + stdlib;
// the fifth source, SkillSource, is the one Source whose two halves live
// in packages that must not see each other (skill does not know prompt
// exists), so it takes a SkillProvider interface here rather than
// importing skill.
//
// This package imports only prompt and the standard library; it does
// not import skill or agent.
package source
```

Files deleted:

- `prompt/sources.go` (split into the 5 source files above + `gitBranch` colocated with `env.go`).
- `prompt/sources_test.go` (split into per-source test files; one helper test stays in `prompt/prompt_test.go`).

`agent/sources.go` is **not deleted**; it keeps `BuildSources` dispatch and the three discovery helpers. The `SkillSource` function body is removed from it.

## `SkillProvider` interface — exact definition

`prompt/source/skillprovider.go`:

```go
package source

// SkillProvider is the seam between prompt/source and the skill registry.
// *skill.Registry satisfies this interface implicitly; consumers may pass
// any type whose SystemPrompt() renders the progressive-disclosure skill
// listing for SLOT_SYSTEM / ORDER_SKILLS.
type SkillProvider interface {
    SystemPrompt() string
}
```

Confirmation that `*skill.Registry` satisfies it: `skill/registry.go:117` declares `func (r *Registry) SystemPrompt() string` — exact match.

Compile-time assertion lives in `prompt/source/skill_test.go` (a `_test.go` file is allowed to import `skill`; production `prompt/source/*.go` stays clean):

```go
import sdkSkill "github.com/bizshuk/agentsdk/skill"

var _ SkillProvider = (*sdkSkill.Registry)(nil)
```

## `BuildSources` dispatch change

`agent/sources.go:54-79` switches its call sites. The new `BuildSources` body (private helpers `promptUserDir` / `discoveryRoots` / `discoverSkills` stay put):

```go
import (
    "fmt"
    "path/filepath"
    "strings"

    "github.com/bizshuk/agentsdk/agent/spec"
    "github.com/bizshuk/agentsdk/prompt"
    "github.com/bizshuk/agentsdk/prompt/source"
    "github.com/bizshuk/agentsdk/skill"
)

func BuildSources(cfg Config, reg *skill.Registry, userDir string) ([]prompt.Source, error) {
    var out []prompt.Source
    if strings.TrimSpace(cfg.Persona) != "" {
        out = append(out, source.PersonaSource(cfg.Persona))
    }
    if cfg.Prompt == nil {
        return out, nil
    }
    for _, name := range cfg.Prompt.Sources {
        switch name {
        case spec.SOURCE_FILES:
            out = append(out, source.ContextFileSource(promptUserDir(cfg, userDir)))
        case spec.SOURCE_SKILLS:
            out = append(out, source.SkillSource(reg))
        case spec.SOURCE_ENV:
            out = append(out, source.EnvSource())
        case spec.SOURCE_REMINDER:
            out = append(out, source.ReminderSource())
        default:
            return nil, fmt.Errorf("agent: unknown prompt source %q", name)
        }
    }
    return out, nil
}
```

Notes:

- The `context` import in `agent/sources.go` is no longer needed (the only `context.Context` use was in the moved `SkillSource` body). Drop it.
- `BuildSources` keeps the concrete `reg *skill.Registry` parameter. `*skill.Registry` satisfies `source.SkillProvider` implicitly — no widening at the call site.
- Top-of-file doc comment in `agent/sources.go` is updated: the old text "prompt owns every Source it can build from its own vocabulary … SkillSource is the one Source that must live here" no longer reflects reality (SkillSource now lives in `prompt/source`). New text: `agent/sources.go` is "the dispatch site — turning the config's source names into live `prompt.Source` values".

## Import path / alias

No alias. The path is `github.com/bizshuk/agentsdk/prompt/source` and the package name is `source`. Callers read `source.PersonaSource(...)`, `source.SkillSource(reg)`. No collision with the parent `prompt` package.

## Test file split

`prompt/sources_test.go` (118 lines, `prompt_test` package) becomes:

| New file | Tests it owns | Notes |
| --- | --- | --- |
| `prompt/source/persona_test.go` | `TestPersonaSourceUsesTheStableOrder`, `TestPersonaSourceEmptyContributesNothing` | duplicates 6-line `sectionsOf` helper |
| `prompt/source/contextfile_test.go` | `TestContextFileSourceReadsTheHierarchy`, `TestContextFileSourceMissingFilesIsNotAnError` | duplicates `sectionsOf` |
| `prompt/source/env_test.go` | `TestEnvSourceIsLastAmongSystemSections` | duplicates `sectionsOf` |
| `prompt/source/reminder_test.go` | `TestReminderSourceOnlySpeaksNearTheBudget` (5-case table) | duplicates `sectionsOf` |
| `prompt/source/skill_test.go` | `TestSkillSourceHandlesNilRegistry` (moved from `agent/sources_test.go:80-84`), `TestSkillSourceNilProviderContributesNothing` (new), `TestSkillSourceDelegatesSystemPrompt` (new, uses a fake `SkillProvider`), and `var _ SkillProvider = (*sdkSkill.Registry)(nil)` compile-time assertion | uses `package source_test` |

The 6-line `sectionsOf` helper is duplicated across the four per-source test files rather than living in a `helpers_test.go` — each per-file helper keeps the test files independently readable. (Test files in Go are a normal place to accept minor duplication for locality.)

End-to-end test `TestSourcesAssembleInTheDocumentedOrder` (`prompt/sources_test.go:94-118`) tests `prompt.Builder`, not any individual Source. Move it into `prompt/prompt_test.go` next to the other `Builder` tests. The Builder still works because `prompt/source/*.go` exports the same `Source` constructors — call sites change from `prompt.PersonaSource(...)` to `source.PersonaSource(...)`.

`agent/sources_test.go` (84 lines) becomes a 55-line file. `TestSkillSourceHandlesNilRegistry` (lines 80-84) and the doc comment block above it (lines 76-79) are removed because the test now lives in `prompt/source/skill_test.go`. The other two tests stay as-is.

## Critical files

- `prompt/source/skill.go` — `SkillSource` with `SkillProvider` interface parameter. If a future contributor types `*skill.Registry` here, `prompt/source` re-imports `skill` and the no-cross-import rule is broken.
- `prompt/source/skillprovider.go` — declares `SkillProvider`; its doc comment is the canonical explanation of the no-cross-import seam.
- `agent/sources.go` — `BuildSources` switch arms + the now-removed `SkillSource` definition. Top-of-file doc comment must be rewritten to reflect that `prompt/source` owns the Sources.
- `prompt/prompt.go` — provides `Source` / `SourceFunc` / `Static` / `Slot` / `Order_*` consumed by every `prompt/source/*.go` file.
- `CLAUDE.md` — three sections need wording updates: the prompt description (line 28), the dependency rule (line 114), and the 2026-07-26 decision entry (line 124). The deps check (line 281) needs an additional sub-package line.

## Doc updates

### CLAUDE.md line 28 — project structure

Replace the `prompt/` line and add a `prompt/source/` line under it. New text:

```text
├── prompt/                # content management：Slot(system/user/reminder)、Source、Builder、LoadContextFiles（AGENTS.md/CLAUDE.md 階層 + @import；2026-07-24 自 contextfile 併入）
├── prompt/source/         # 內建 Source 實作：PersonaSource/ContextFileSource/EnvSource/ReminderSource + SkillSource（透過 SkillProvider interface 收 *skill.Registry，prompt/source 仍只 import prompt + stdlib）
```

### CLAUDE.md line 114 — the prompt/skill mutual-ignorance rule

Old:

> `skill` 不知道 `prompt` 存在，adapter 住在 `agent`；context-file 載入已併入 `prompt.LoadContextFiles`（固定行為、無客製化縫）。

New:

> `skill` 不知道 `prompt` 存在，adapter 住在 `prompt/source`（sub-package，透過 `SkillProvider` interface 而非型別耦合）；context-file 載入已併入 `prompt.LoadContextFiles`（固定行為、無客製化縫）。

### CLAUDE.md line 124 — 2026-07-26 decision entry (extend)

Append a second paragraph:

> 接著把 `prompt/sources.go` 五個 Source（含 `agent.SkillSource` 與新增的 `SkillProvider` interface）一同移入 `prompt/source/` sub-package，每個 Source 一檔；`prompt/source` 不 import `skill` 也不 import `agent`，只透過 `SkillProvider` interface 收 `*skill.Registry`。`agent/sources.go` 因此只剩 `BuildSources` 的 name→Source dispatch、`promptUserDir`、`discoveryRoots`、`discoverSkills`。`prompt/source` 仍只 import `prompt` 與 stdlib。

### CLAUDE.md line 281 — `go list -deps` rule

Add a line under the existing two:

```bash
go list -deps ./agent/spec | grep agentsdk   # 只該有 core 與 agent/spec
go list -deps ./prompt     | grep agentsdk   # 只該有 core 與 prompt 與 prompt/source
go list -deps ./prompt/source | grep agentsdk # 只該有 prompt 與 prompt/source（不該出現 skill）
```

## Order of execution

Sequenced to keep each green-commit small. After each step, run `go build ./...` and `go test ./...` from the repo root.

1. **Create `prompt/source/` skeleton** — `doc.go` + `skillprovider.go` + 5 source files. The package compiles standalone (nothing references it yet). `go build ./prompt/source` green.
2. **Move tests into `prompt/source/`** — 5 per-source test files + the compile-time `var _ SkillProvider` assertion. `go test ./prompt/source -count=1 -v` green.
3. **Switch `agent/sources.go` and `agent/sources_test.go`** — replace `prompt.PersonaSource(...)` with `source.PersonaSource(...)` etc.; remove the inline `SkillSource` definition; remove the `context` import; drop the `TestSkillSourceHandlesNilRegistry` test from `agent/sources_test.go`. `go test ./agent` green.
4. **Move `TestSourcesAssembleInTheDocumentedOrder`** into `prompt/prompt_test.go` (with `source.X` call sites). `go test ./prompt` green.
5. **Delete the old files** — `rm prompt/sources.go` and `rm prompt/sources_test.go`. `go build ./...` green.
6. **Verify dependency rules** — three `go list -deps` checks (see below).
7. **Workspace sweep** — build + test all 9 modules; `go-dependency-analysis` pass.
8. **Update CLAUDE.md** — apply the four doc edits.

## Verification

### Build + test all 9 modules

```bash
go work sync
go mod download

go build ./...
go test ./... -count=1 -timeout=120s

for mod in . sample/code-agent sample/file-agent sample/greet-agent \
            sample/logdoctor-agent sample/skeleton-agent \
            sample/demo-memory sample/demo-middleware sample/demo-strategy; do
  (cd "$mod" && go build ./... && go test ./... -count=1 -timeout=120s)
done
```

### Dependency rules (must hold)

```bash
go list -deps ./prompt/source | grep agentsdk
# EXPECTED: prompt/source
# NOT: skill, agent, core (none of these should appear)

go list -deps ./prompt | grep agentsdk
# EXPECTED: prompt, prompt/source
# NOT: skill (this would mean skill leaked into prompt)

go list -deps ./agent | grep agentsdk
# EXPECTED: agent, core, prompt, prompt/source, skill, agent/spec
# NOT: provider, runtime, planning, action, tool (composition-layer leakage)
```

### Targeted unit tests

```bash
go test ./prompt/source -count=1 -v
go test ./prompt        -count=1 -v
go test ./agent         -count=1 -v
```

### CI guard — prevent the regression

If `SkillProvider` is forgotten and someone writes `SkillSource(reg *skill.Registry)`, the package re-imports `skill`. Add this check to a `scripts/check-deps.sh` (create if missing):

```bash
# prompt/source must not import skill in production code.
files=$(go list -f '{{ range .GoFiles }}{{ . }}\n{{ end }}' ./prompt/source)
echo "$files" | while read f; do
  [ -z "$f" ] && continue
  case "$f" in *_test.go) continue;; esac
  if grep -q 'agentsdk/skill' "$f"; then
    echo "FAIL: $f imports skill — use SkillProvider interface" >&2
    exit 1
  fi
done
```

### Analyzer pass

```bash
go-dependency-analysis --workspace /Users/shuk/projects/ai/agentSDK/go.work --format text
go-dependency-analysis --workspace /Users/shuk/projects/ai/agentSDK/go.work \
  --policy /Users/shuk/projects/go-dependency-analysis/examples/agentsdk.json
```

## Risks

1. **Forgetting `SkillProvider`** — re-introduces `prompt/source → skill` import. Caught by the CI guard above and by `go list -deps ./prompt/source | grep agentsdk` returning only `prompt/source`.
2. **Test helper duplication** — `sectionsOf` is duplicated across 4 test files. Acceptable; alternative (a 6th `helpers_test.go` file) trades one risk for another.
3. **`gitBranch` leak** — unexported helper colocated with `EnvSource`. If a future Source needs similar logic, copy it; do not promote to `GitBranch`.
4. **Cross-sub-package cycle** — `prompt/source` imports `prompt`; `prompt` must never import `prompt/source`. The third `go list -deps` check covers this.
5. **`BuildSources` parameter type** — keep concrete `*skill.Registry`, do not widen to `source.SkillProvider`. Widening would force every caller (currently just `agent/build.go:174` and `agent/sources_test.go`) to know about the interface; concrete types keep `agent` the composition seam.
