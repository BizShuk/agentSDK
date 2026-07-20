# Go Dependency Analyzer v1 Implementation Plan

> Status: Implemented in AgentSDK, then extracted on 2026-07-20 to the standalone repository `/Users/shuk/projects/go-dependency-analysis` with module path `github.com/bizshuk/go-dependency-analysis`. Paths below record the original implementation sequence and are historical.

> **For agentic workers:** REQUIRED SUB-SKILL: Use `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立一個獨立、stdlib-only 的 Go CLI，分析所有 `go.work` modules 的 module/package graph，並輸出 dependency minimization 與 layer alignment diagnostics。

**Architecture:** `tools/dependency-analyzer` 是獨立 module。Discovery 透過 Go tooling 取得事實，graph package 正規化與運算拓撲，policy package 評估 heuristics，report package提供 deterministic text/JSON/Mermaid；CLI 只做 orchestration，不修改任何 `go.mod`。

**Tech Stack:** Go 1.26.0、standard library、`go env`、`go work edit -json`、`go list -m/-deps -json`。

## Global Constraints

- Analyzer module 不得 import `github.com/bizshuk/agentsdk` 或任何 third-party package。
- 預設分析所有 `go.work` product modules，包括 `../ai/llm_provider` 與 `../ai/proxy`；排除 analyzer 自身。
- 所有輸出必須 deterministic，診斷必須區分 `go-tool-fact` 與 `policy-heuristic`。
- v1 只讀；不得執行 `go mod tidy`、自動刪除 require、或改寫 dependency versions。
- 保留現有 working tree 的無關變更；不得 commit，除非使用者另行要求。

---

### Task 1: CLI shell and Go-tool discovery

**Files:**
- Create: `tools/dependency-analyzer/go.mod`
- Create: `tools/dependency-analyzer/main.go`
- Create: `tools/dependency-analyzer/cmd/command.go`
- Create: `tools/dependency-analyzer/cmd/command_test.go`
- Create: `tools/dependency-analyzer/internal/discovery/runner.go`
- Create: `tools/dependency-analyzer/internal/discovery/workspace.go`
- Create: `tools/dependency-analyzer/internal/discovery/modules.go`
- Create: `tools/dependency-analyzer/internal/discovery/packages.go`
- Create: `tools/dependency-analyzer/internal/discovery/discovery_test.go`

**Interfaces:**
- Produces: `cmd.Run(ctx context.Context, args []string, stdout, stderr io.Writer) int`.
- Produces: `discovery.Runner.Run(ctx context.Context, dir string, env []string, name string, args ...string) (stdout, stderr []byte, err error)`.
- Produces: `Discover(ctx, Runner, Options) (Snapshot, error)` where `Snapshot` contains workspace modules, selected module records, packages, and direct requirements.

- [ ] Write table-driven tests for CLI defaults/invalid formats and fake-runner tests for streaming `go work`/`go list` JSON, external sibling paths, malformed JSON, stderr failures, and self-exclusion.
- [ ] Run `cd tools/dependency-analyzer && go test ./cmd ./internal/discovery`; expect initial compile/test failure.
- [ ] Implement `flag.FlagSet` parsing for workspace, format, output, policy, include-tests, show-stdlib, include-tool-module, exclusions, fail-on, json-indent, and version.
- [ ] Implement `ExecRunner` with `exec.CommandContext`, separate stdout/stderr, supplied directory/environment, and wrapped command errors.
- [ ] Implement workspace discovery from `go env GOWORK` and `go work edit -json`; resolve every `Use.DiskPath` relative to the `go.work` directory.
- [ ] Implement per-module streaming decoders for `go list -m -json all` and `go list -deps -json ./...`, plus a narrow direct-require parser for single and grouped require clauses.
- [ ] Run `go test ./cmd ./internal/discovery`; expect PASS.

### Task 2: Graph model, policy, and diagnostics

**Files:**
- Create: `tools/dependency-analyzer/internal/graph/model.go`
- Create: `tools/dependency-analyzer/internal/graph/build.go`
- Create: `tools/dependency-analyzer/internal/graph/algorithms.go`
- Create: `tools/dependency-analyzer/internal/graph/graph_test.go`
- Create: `tools/dependency-analyzer/internal/policy/config.go`
- Create: `tools/dependency-analyzer/internal/policy/evaluate.go`
- Create: `tools/dependency-analyzer/internal/policy/policy_test.go`
- Create: `tools/dependency-analyzer/policy/agentsdk.json`

**Interfaces:**
- Consumes: `discovery.Snapshot`.
- Produces: `graph.Build(snapshot) graph.Analysis` with sorted package/module nodes, edges, evidence, selected versions, and direct requirements.
- Produces: `policy.Load(path) (Config, error)` and `policy.Evaluate(analysis, config) []Diagnostic`.

- [ ] Write tests for deduplication, package-to-module ownership, module collapse, Tarjan SCC, reachability, shortest paths, and shuffled-input determinism.
- [ ] Run `go test ./internal/graph`; expect initial failure.
- [ ] Implement normalized graph models and builders; preserve package-pair evidence for collapsed module edges.
- [ ] Implement Tarjan SCC for cycles and BFS for shortest paths; sort all nodes, edges, members, and evidence.
- [ ] Run graph tests; expect PASS.
- [ ] Write policy tests for exact/prefix patterns, forbidden/layer edges, version divergence, unused-direct candidates, heavy transitive paths, provenance, caveats, and severity overrides.
- [ ] Implement a strict JSON policy with named layers, allow targets, forbidden edges, heavy module weights, thresholds, and severity overrides; reject unknown JSON fields.
- [ ] Implement stable diagnostic codes and provenance values `go-tool-fact`/`policy-heuristic`; do not claim unused direct requirements are safe to remove.
- [ ] Add AgentSDK policy encoding `core` stdlib-only intent, `tui` independence, composition boundaries, and sibling module direction.
- [ ] Run `go test ./internal/graph ./internal/policy`; expect PASS.

### Task 3: Deterministic reports and CLI orchestration

**Files:**
- Create: `tools/dependency-analyzer/internal/report/model.go`
- Create: `tools/dependency-analyzer/internal/report/text.go`
- Create: `tools/dependency-analyzer/internal/report/json.go`
- Create: `tools/dependency-analyzer/internal/report/mermaid.go`
- Create: `tools/dependency-analyzer/internal/report/report_test.go`
- Modify: `tools/dependency-analyzer/cmd/command.go`
- Modify: `tools/dependency-analyzer/main.go`

**Interfaces:**
- Consumes: graph analysis plus policy diagnostics.
- Produces: `report.RenderText`, `report.RenderJSON`, and `report.RenderMermaid` writing to `io.Writer`.

- [ ] Write exact-output tests for empty/single/multi-module graphs, diagnostics, JSON schema version 1, Mermaid escaping, and shuffled-input determinism.
- [ ] Run `go test ./internal/report`; expect initial failure.
- [ ] Implement concise text summary and diagnostics with provenance/evidence.
- [ ] Implement versioned JSON containing completeness/test-mode metadata and complete normalized graphs.
- [ ] Implement module-level Mermaid with stable sanitized IDs and quoted labels/edge text.
- [ ] Wire discovery → graph → policy → report in `cmd.Run`; map threshold/usage/tool failures to exit codes 0/1/2/3 and support atomic output-file creation only after successful analysis.
- [ ] Run `go test ./... -count=1`; expect PASS.

### Task 4: Workspace integration, docs, and end-to-end verification

**Files:**
- Modify: `go.work`
- Create: `tools/dependency-analyzer/README.md`
- Modify: `README.md`
- Modify: `CLAUDE.md`

**Interfaces:**
- Produces: documented `go run ./tools/dependency-analyzer` workflow and current module topology.

- [ ] Add `./tools/dependency-analyzer` to `go.work`; verify default self-exclusion still reports 11 product modules.
- [ ] Document CLI flags, formats, policy schema, exit codes, facts versus heuristics, and build-tag/platform/tool limitations in the tool README.
- [ ] Correct current topology in `README.md` and `CLAUDE.md`; retain historical statements only in historical plans/specs.
- [ ] Run `gofmt` on analyzer Go files.
- [ ] Run `cd tools/dependency-analyzer && go test ./... -count=1 && go vet ./... && go build ./...`; expect all commands to pass.
- [ ] Run text, indented JSON, Mermaid, and policy/fail-on smoke commands from repo root; verify all 11 product modules and stable diagnostics.
- [ ] Run JSON twice and `diff -u` outputs; expect no difference.
- [ ] Run `go test ./... -count=1 -timeout=120s` independently in every module listed by `go work edit -json`, including analyzer; report any pre-existing failures separately.
- [ ] Inspect `git diff --check` and `git status --short`; confirm only intended files changed and unrelated initial changes remain untouched.
