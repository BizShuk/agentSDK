# Extract Go Dependency Analyzer to Standalone Repository

## Context

`tools/dependency-analyzer` was an independent, standard-library-only CLI with no AgentSDK package dependency, but initially lived inside the AgentSDK repository with module path `github.com/bizshuk/agentsdk/tools/dependency-analyzer`. The user requested moving it to `~/projects/go-dependency-analysis`. The extraction preserves the existing CLI contract and analyzer behavior, removes workspace coupling from AgentSDK, and makes the destination conform to the global unified repository interface.

The recommended defaults are: module path `github.com/bizshuk/go-dependency-analysis`, new local Git repository without a commit, AgentSDK policy retained as `examples/agentsdk.json`, and external CLI invocation rather than a Go import or `go.work` linkage.

## Extraction Plan

### 1. Guard and stage the move

- Record both repositories’ pre-change state and preserve all unrelated uncommitted AgentSDK changes.
- Refuse to overwrite the destination if `/Users/shuk/projects/go-dependency-analysis` appears and is non-empty.
- Copy the source into a temporary sibling staging directory first; do not delete the AgentSDK source until destination tests pass.
- Rewrite the module path and all internal imports from `github.com/bizshuk/agentsdk/tools/dependency-analyzer` to `github.com/bizshuk/go-dependency-analysis`.

### 2. Make the analyzer standalone

Create the destination with:

- `go.mod`, `main.go`, `cmd/`, `internal/` from the source analyzer;
- `README.md` for installation, generic workspace usage, formats, policy, flags, exit codes, and limitations;
- `CLAUDE.md` for technical boundaries and verification commands;
- `AGENTS.md -> CLAUDE.md` relative symlink;
- `README.todo` using the required TODO/Archive format;
- `docs/memory/2026-07-20-agentSDK-extraction.md` recording the extraction decision;
- `examples/agentsdk.json` containing the existing AgentSDK-specific policy.

Initialize `/Users/shuk/projects/go-dependency-analysis` as a new Git repository after the staged content is validated, but do not commit.

### 3. Remove repo-layout coupling

Update `internal/discovery` so self-exclusion is based on the standalone module identity/canonical module directory rather than the suffix `tools/dependency-analyzer`. Preserve `--include-tool-module` as the override and keep explicit `--exclude` support.

Add regression tests for:

- arbitrary checkout directory names;
- absolute and relative `--workspace` paths;
- installed/external analyzer not present in the target workspace;
- analyzer module present in the workspace and excluded by default;
- `--include-tool-module` restoring it;
- symlink-normalized workspace/module paths.

The analyzer must remain read-only and stdlib-only.

### 4. Validate destination before removing source

Run in the staged/destination repository:

```bash
go test ./... -count=1
go vet ./...
go build ./...
go run . --version
```

Run deterministic text, JSON, Mermaid, and policy smoke tests against `/Users/shuk/projects/agentSDK/go.work`. Verify JSON outputs are byte-identical across two runs and Mermaid IDs remain unique.

### 5. Detach AgentSDK

After destination validation:

- remove `./tools/dependency-analyzer` from `go.work`;
- delete `tools/dependency-analyzer` from AgentSDK;
- update `README.md` and `CLAUDE.md` to remove the in-tree module/tree entry and recalculate workspace counts;
- replace `go run ./tools/dependency-analyzer` examples with `go-dependency-analysis --workspace /Users/shuk/projects/agentSDK/go.work` and the external example policy path;
- annotate active analyzer plans/specs with the new standalone location while preserving historical context;
- do not add the standalone repository back to AgentSDK’s `go.work` and do not import it from AgentSDK code.

No production AgentSDK wrapper is needed in v1 because there is no code caller; documentation should use the installed binary or an explicit absolute checkout command.

### 6. Final verification

Destination:

```bash
cd /Users/shuk/projects/go-dependency-analysis
go test ./... -count=1
go vet ./...
go build ./...
go run . --workspace /Users/shuk/projects/agentSDK/go.work --format text
go run . --workspace /Users/shuk/projects/agentSDK/go.work --format json --json-indent=’  ‘
go run . --workspace /Users/shuk/projects/agentSDK/go.work --format mermaid
go run . --workspace /Users/shuk/projects/agentSDK/go.work --policy examples/agentsdk.json --fail-on error
```

AgentSDK:

```bash
cd /Users/shuk/projects/agentSDK
go work edit -json
go test ./... -count=1 -timeout=120s
git diff --check
git status --short
```

Search both repositories for the old module/import path and old in-tree invocation. Remaining occurrences are allowed only when clearly marked historical.

## Critical Files

Destination:

- `/Users/shuk/projects/go-dependency-analysis/go.mod`
- `/Users/shuk/projects/go-dependency-analysis/internal/discovery/discovery.go`
- `/Users/shuk/projects/go-dependency-analysis/README.md`
- `/Users/shuk/projects/go-dependency-analysis/CLAUDE.md`
- `/Users/shuk/projects/go-dependency-analysis/README.todo`
- `/Users/shuk/projects/go-dependency-analysis/examples/agentsdk.json`
- `/Users/shuk/projects/go-dependency-analysis/docs/memory/2026-07-20-agentsdk-extraction.md`

AgentSDK:

- `/Users/shuk/projects/agentSDK/go.work`
- `/Users/shuk/projects/agentSDK/README.md`
- `/Users/shuk/projects/agentSDK/CLAUDE.md`
- `/Users/shuk/projects/agentSDK/docs/superpowers/plans/2026-07-20-go-dependency-analyzer-v1.md`
