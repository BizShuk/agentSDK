# skeleton-demo

The single-file sample that demonstrates the agent skeleton in its canonical, wizard-generated form:

```go
func main() {
    app.Main(agent.MustNew(cfg, opts...), appOpts...)
}
```

`main.go` is 108 lines: 12 for `stdinAgent` (seed opening state from stdin), 5 for the `SinkFunc` (print to stdout), and the rest docstring + literal `wizard --print-go` config.

Compare to [sample/code-agent](../code-agent), which uses cobra + `compose()` + four dispatch modes.
That structure is what you reach for when an application needs:

- flag surface with many switches
- per-mode `Engine.Sink` swapping (tui channel / wire / progress)
- direct access to `*Parts.Sessions` and `*Parts.Skills`
- `agent.Option` closures that can return errors (so it uses `agent.New` not `agent.MustNew`)

`skeleton-demo` is the case where none of that is needed: the persona is the task, the input is stdin, the output is one assistant line on stdout.

## Run

```bash
export MINIMAX_API_KEY=...
echo "Payment page throws 500 on /checkout" | go run ./sample/skeleton-demo
# → stdout:  P0|<reason>500 errors on the payment page block paid conversions.
# → stderr:  {"level":"INFO","msg":"run_done",...,"turns":1,"status":"completed"}
```

Without an API key the preflight fails visibly (the `app.WithLogToStdout()` option
swaps the default file-backed slog handler for a stdout handler) — a production CLI
would keep the file handler; this is a demo, so observability beats hush.

```bash
go run ./sample/skeleton-demo </dev/null
# {"level":"ERROR","msg":"preflight failed","err":"...missing MINIMAX_API_KEY..."}
# exit 1
```

## Compare against the wizard output

The Config block is the literal `cmd/wizard.go::goLiteral` output for `-t basic`, with three explicit edits documented in `main.go`:

```bash
cd /Users/shuk/projects/ai/agentSDK
go run . w -y --tier basic -o - --print-go
```

## When to reach for this pattern vs. code-agent's

| | skeleton-demo | code-agent |
| --- | --- | --- |
| flag surface | none | 11 flags |
| modes | 1 (stdin → verdict → stdout) | 4 (sessions / -p / --json / interactive tui) |
| `agent.New` vs `MustNew` | MustNew | New (opts can fail) |
| `*Parts` exposure | none | Sessions / Skills / Engine / Cwd |
| `Engine.Sink` swap | WithSink(stdout) constant | per-mode (channel / wire / progress) |
| `agent.Option` blocks | WithSink | WithHooks(blockDestructiveBash()) |
| `app.Option` blocks | WithLogToStdout | none |
| lines of Go (single binary) | ~108 | ~333 across 4 files |

## Files

```
sample/skeleton-demo/
├── main.go       # stdinAgent + SinkFunc + wizard-template main()
├── go.mod        # depends only on the root module
└── README.md     # this file
```
