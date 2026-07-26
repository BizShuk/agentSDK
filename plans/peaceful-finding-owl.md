# Move `agent/load.go` to `utils/agentconfig/`

## Context

`agent/load.go` (55 lines, `package agent`) is pure file-IO glue: it
re-exports `utils/configfile`'s `Format` / `FORMAT_*` / `FormatOf` and
combines `configfile.ReadJSON` / `Marshal` / `Write` with
`spec.DecodeBytes` / `EncodeBytes` for `agent.Config`. The composition-
layer `agent` package shouldn't own file-IO details — that concern sits
with the other production helpers (`utils/configfile/`,
`utils/frontmatter/`, `utils/testutil/`). The user wants this lifted
out of `agent/` and placed under `utils/`.

After the move, callers read/write agent config through
`utils/agentconfig/LoadFile` / `Marshal` / `SaveFile`; the composition
package keeps only what composition owns.

## Target

New sub-package `utils/agentconfig` (`package agentconfig`),
file `utils/agentconfig/load.go`, mirror of today's `agent/load.go`
with two adjustments:

- **Return type goes from `agent.Config` to `spec.Config`.** Returning
  `spec.Config` keeps `utils/agentconfig` free of any dependency on
  `agent/` and breaks no contract — `agent.Config = spec.Config` is a
  type alias (`agent/choices.go:23`), so callers can assign without
  conversion. Imports become exactly:
    - `github.com/bizshuk/agentsdk/agent/spec`
    - `github.com/bizshuk/agentsdk/utils/configfile`
- **Re-exported surface stays the same.** `Format`, `FORMAT_YAML`,
  `FORMAT_JSON`, `FormatOf` are forwarded from `utils/configfile` for
  caller convenience (matches today's `agent/load.go` lines 9-19).

### Layering check

`utils/agentconfig` deps:
- `agent/spec` — declarative layer, imports `core` only; OK
- `utils/configfile` — stdlib-only generic file IO; OK

Downstream callers:
- `cmd/agent/wizard/{command,wizard_test}.go`, samples
  (`greet-agent`, `file-agent`, `skeleton-agent`) all import
  `agent` for `agent.Config` etc.; they can additionally import
  `utils/agentconfig` without a cycle.

No cycle, no new external dependency, no behaviour change.

## Files

**Write (new):**
- `utils/agentconfig/load.go` — port of `agent/load.go`. Preserve the
  existing comments that explain the YAML→JSON pivot and the
  refused-clobber semantics (`configfile.Write`'s `force` flag).

**Modify — call-site swaps (6 files, 10 sites):**

| File                              | Sites                    | Change                                           |
| --------------------------------- | ------------------------ | ------------------------------------------------ |
| `cmd/agent/wizard/command.go`     | lines 81, 114, 120       | `agent.LoadFile` / `agent.Marshal` / `agent.FORMAT_YAML` / `agent.SaveFile` → `agentconfig.*` |
| `cmd/agent/wizard/wizard_test.go` | lines 41, 107, 167       | `agent.LoadFile` → `agentconfig.LoadFile`        |
| `sample/file-agent/main.go`       | line 94                  | same swap, one site                              |
| `sample/greet-agent/main.go`      | line 89                  | same swap, one site                              |
| `sample/skeleton-agent/main.go`   | line 179 (comment), 182  | same swap, one comment one call                  |

Add `utils/agentconfig` to the import block in each file.

**Delete:**
- `agent/load.go`

**Modify — project docs:**
- `CLAUDE.md` — two touch points:
    1. The `agent/` package description currently lists
       `LoadFile/SaveFile` among `agent`'s public functions
       (CLAUDE.md, "Module Mapping" row for `agent/config` /
       `agentsdk/agent`). Drop those two names from the `agent`
       description.
    2. The `Module Mapping` table has no `utils/agentconfig` row
       yet. Add a row under the `宣告式設定` (declarative config)
       group pointing at the new home, with one-line note: "format-
       chosen file IO for `spec.Config` (load/marshal/save)."

## Out of scope (intentionally)

- **No new tests.** Today's `agent/load.go` has no dedicated
  `load_test.go`; the wizard tests already exercise the path
  end-to-end, and `agent/spec/spec_test.go` covers `Decode` /
  `Encode`. Adding `utils/agentconfig/load_test.go` is a separate
  decision.
- **No backward-compat aliases** in `agent/` for `LoadFile` /
  `Marshal` / `SaveFile` / `FORMAT_*` / `FormatOf`. Matches the
  precedent set by the `pkg registry → pkg provider` rename
  (2026-07-26): rename the surface, update all callers, no shims.

## Verification

Run from `/Users/shuk/projects/ai/agentSDK`:

```bash
go work sync
go build ./...
go test ./... -count=1 -timeout=120s

for mod in . sample/code-agent sample/file-agent sample/greet-agent \
  sample/logdoctor-agent sample/skeleton-agent sample/demo-memory \
  sample/demo-middleware sample/demo-strategy; do
  (cd "$mod" && go build ./... && go test ./... -count=1 -timeout=120s)
done
```

Then sanity-check no stale caller remains:

```bash
grep -rn "agent\.LoadFile\|agent\.Marshal\|agent\.SaveFile\|agent\.Format\|agent\.FORMAT_" \
  --include="*.go" .
# must return 0 lines
```

End-to-end smoke:

```bash
go run . w -y --tier basic -o -                       # wizard writes YAML via agentconfig.Marshal
go run . w --edit /tmp/agent.yaml                     # wizard reads via agentconfig.LoadFile
cd sample/file-agent && go run . -c ./agent.yaml     # sample loads via agentconfig.LoadFile
```

Wizard output (`go run . w -y --tier basic -o -`) must be byte-for-byte
identical to today's output, since `agentconfig.Marshal` is a 1-to-1
re-export of the old `agent.Marshal`.
