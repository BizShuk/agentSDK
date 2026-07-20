# dependency-analyzer

`dependency-analyzer` is a read-only, standard-library-only CLI for inspecting the Go workspace module and package dependency graph.

```bash
go run ./tools/dependency-analyzer --format text
go run ./tools/dependency-analyzer --format json > dependency.json
go run ./tools/dependency-analyzer --format mermaid
go run ./tools/dependency-analyzer --policy ./tools/dependency-analyzer/policy/agentsdk.json
```

It uses `go env`, `go work edit -json`, and `go list`; tool output is treated as fact, while policy diagnostics are explicitly marked as heuristics. JSON is schema version 1. The analyzer excludes itself by default. `--workspace` selects a workspace file and `--format` accepts `text`, `json`, or `mermaid`.

Exit codes: `0` success, `2` usage or policy error, `3` Go-tool or output error. v1 does not modify `go.mod`, `go.work`, or policy files. Build tags and platform-specific package sets reflect the host Go toolchain.
