# Plan — Built-in Tools for agentSDK (M5)

## Context

agentSDK currently provides tool *infrastructure* (`TypedTool`, `Registry`, `Sandbox`, `ApprovalPolicy`) but ships **zero built-in tools**. Every user must write their own tools from scratch. Anthropic's Claude Code ships ~40 built-in tools (Bash, Read, Write, Edit, Glob, Grep, WebFetch, WebSearch, etc.). This plan adds the most essential 6 as a first-class `tool/` package, with an injection API (`RegisterDefaults`) that matches the existing composition-root pattern.

## Design Decisions

1. **Package**: `tool/` at repo root (peer of `action/`, `runtime/`, `memory/`) — module path `github.com/bizshuk/agentsdk/tool`
2. **Injection**: `tool.RegisterDefaults(reg, opts)` — explicit registration at composition root; no auto-injection in `runtime.Loop`
3. **Pattern**: every tool wraps `*action.TypedTool[TArgs, TOut]` and delegates `core.Tool` interface (mirrors `sample/greet-agent/tool/greet.go`)
4. **Sandbox**: tools defensively re-check `policy.Check()` even though middleware also checks; respects existing `action.Policy` path/command rules
5. **Risk**: read-only tools = `RISK_LEVEL_LOW`, mutating tools = `RISK_LEVEL_HIGH` (triggers HITL at L1/L2 via existing `ApprovalGate`)
6. **No new deps**: stdlib only (`os`, `os/exec`, `path/filepath`, `regexp`, `bufio`, `io`, `strings`, `net/http` for MIME sniff)

## Tools to Implement (6 tools, ~1100 LOC + ~900 LOC tests)

| Tool | Args | Output | Risk | Key Behavior |
|------|------|--------|------|-------------|
| `Read` | `path`, `offset?`, `limit?` | `content`, `encoding`, `truncated`, `size`, `mime` | LOW | Line-bounded read via `bufio.Scanner`; binary → base64 + MIME sniff; cap at 1 MiB |
| `Write` | `path`, `content` | `wrote`, `created` | HIGH | `os.WriteFile` with atomic temp+rename; `filepath.EvalSymlinks` + re-check against sandbox |
| `Edit` | `path`, `old_text`, `new_text`, `replace_all?` | `replacements`, `bytes_after` | HIGH | Read full file → `strings.Replace` (exact match, NOT regex); refuse 0 matches; refuse >1 unless `ReplaceAll`; atomic write |
| `Bash` | `command`, `timeout_ms?`, `cwd?` | `stdout`, `stderr`, `exit_code`, `duration` | HIGH | `/bin/sh -c` via `os/exec`; `Executor` interface for test stubs; `limitWriter` caps output; `commandDenied` via sandbox |
| `Glob` | `pattern`, `cwd?` | `matches`, `count` | LOW | Recursive `**` support via manual `filepath.WalkDir` (no `doublestar` dep); default 100 match cap |
| `Grep` | `pattern`, `path?`, `glob?`, `case_insensitive?`, `max_results?`, `line_numbers?` | `matches[]`, `count`, `truncated` | LOW | `regexp` + `bufio.Scanner` + `filepath.WalkDir`; `MaxResults` cap (default 100); skip binary files |

## File Structure

```tree
tool/
├── tool.go              # Package doc, Options, RegisterDefaults, Defaults
├── read.go              # Read + ReadArgs/ReadOutput + NewRead
├── write.go             # Write + NewWrite
├── edit.go              # Edit + NewEdit
├── bash.go              # Bash + Executor interface + realExecutor + limitWriter
├── glob.go              # Glob + NewGlob + doublestarMatch helper
├── grep.go              # Grep + GrepMatch + NewGrep
├── fs_helpers.go        # resolvePath, sniffMime, atomicWrite, safeCwd
├── tool_test.go         # RegisterDefaults integration test
├── read_test.go
├── write_test.go
├── edit_test.go
├── bash_test.go
├── glob_test.go
└── grep_test.go
```

## Injection API

```go
// tool/tool.go
package tool

type Options struct {
    Policy     action.Sandbox   // required for Write/Edit/Bash; optional for Read/Glob/Grep
    WorkingDir string           // default "."
    Read       ReadOptions
    Write      WriteOptions
    Edit       EditOptions
    Bash       BashOptions
    Glob       GlobOptions
    Grep       GrepOptions
}

type BashOptions struct {
    DefaultTimeout time.Duration  // 0 → 30s
    MaxOutputBytes int64          // 0 → 1 MiB
    Executor       Executor       // nil → real os/exec
    Env            []string       // nil → os.Environ()
}

// RegisterDefaults constructs all 6 tools and registers them into reg.
// Returns the list of registered tools. Errors if invariants break
// (e.g. Bash without a Policy).
func RegisterDefaults(reg *action.Registry, opts Options) ([]core.Tool, error)
```

Usage at composition root:

```go
reg := action.NewRegistry()
tool.RegisterDefaults(reg, tool.Options{
    Policy:     action.DefaultPolicy(),
    WorkingDir: ".",
})
// reg now has: read, write, edit, bash, glob, grep
```

## Integration Points

- **Sandbox middleware** (`middleware/security/sandbox_mw.go`): `Policy.Check(toolName, args)` already runs on every `CALL_TOOL` — built-ins use conventional arg keys (`path`, `command`) so the default policy works without changes
- **Approval gate** (`middleware/security/approval_gate.go`): reads `eff.CallTool.Call.Risk` — built-ins set their risk levels; the gate is automatic
- **Runtime** (`runtime/loop.go`): NO changes needed — the loop is tool-agnostic

## Verification

1. `go build ./tool/...` compiles clean
2. `go test ./tool/... -count=1 -timeout=30s` all green, 80%+ coverage
3. `go vet ./tool/...` passes
4. Manual: modify `sample/greet-agent/cmd/root.go` to call `tool.RegisterDefaults(reg, ...)` → `go run .` works, LLM can use `read`/`bash`/etc.
5. E2E: create a simple `sample/file-agent/` that wires all 6 tools + ReAct + FakeProvider → verify JSONL output includes tool calls to built-ins
