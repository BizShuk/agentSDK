# skeleton-agent

The single-file sample that demonstrates the agent skeleton in its canonical, wizard-generated form:

```go
func main() {
    app.Main(agent.MustNew(cfg, opts...), appOpts...)
}
```

`main.go` seeds the opening prompt from stdin (`stdinAgent.Bootstrap`), prints assistant text to stdout (`SinkFunc`), and turns the run into a small REPL through the `app.Interactive` seam (`stdinAgent.NextRound`) — the rest is docstring + literal `wizard --print-go` config.

Compare to [sample/code-agent](../code-agent), which uses cobra + `compose()` + four dispatch modes.
That structure is what you reach for when an application needs:

- flag surface with many switches
- per-mode `Engine.Sink` swapping (tui channel / wire / progress)
- direct access to `*Parts.Sessions` and `*Parts.Skills`
- `agent.Option` closures that can return errors (so it uses `agent.New` not `agent.MustNew`)

`skeleton-agent` is the case where none of that is needed: the persona is the task, the input is stdin, the output is one assistant line on stdout.

## Run

The sample reads its `agent.Config` from `skeleton-agent.yaml` (sitting next
to `main.go`), so the working directory must be the sample directory. Run
from anywhere as long as you `cd` in first:

```bash
cd sample/skeleton-agent
export MINIMAX_API_KEY=...

# One-shot — a single piped line runs one round and exits
echo "Payment page throws 500 on /checkout" | go run .
# → stdout:  P0|<reason>500 errors on the payment page block paid conversions.
# → stderr:  {"level":"INFO","msg":"run_done",...,"turns":1,"rounds":1,"status":"completed"}

# REPL — first line is the opening prompt, each later line is a follow-up
# round, blank line or EOF ends the session. This is the agent.Interactive
# seam: after a run completes, agent.Run hands the finished state back to
# NextRound, which reads one more line and feeds it as the next round's input.
printf 'ping\nreply with one word: sky color\n\n' | go run .
# → stdout:  Pong! ... \n Blue
# → stderr:  ...,"turns":2,"rounds":2,"status":"completed"
# interactively, just: go run .   (Ctrl-D to finish)
```

The same `NextRound` method also answers approval pauses
(`PAUSE_APPROVAL`) — this config gates nothing, so that branch stays dark
until you register a tool and tighten `Autonomy`.

Without an API key the failure surfaces visibly on the first model call —
this demo wires no `Preflighter`, so the error appears as a `run_failed` log
line rather than a preflight abort. `app.WithLogToStdout()` swaps the default
file-backed slog handler for a stdout one so the error is not buried in
`~/.config/skeleton-agent/logs/`; a production CLI would keep the file handler.

```bash
echo ping | go run ./sample/skeleton-agent
# {"level":"ERROR","msg":"run_failed","err":"...model generate: ...MINIMAX_API_KEY..."}
# exit 1
```

## Compare against the wizard output

The bundled `skeleton-agent.yaml` mirrors what `cmd/agent/wizard.go::goLiteral`
emits for `-t basic`, with three explicit edits (no persona,
`think_then_act` reasoning, `MaxTurns: 50`) — round-trip the wizard output
to regenerate it:

```bash
cd /Users/shuk/projects/ai/agentSDK
go run . w -y --tier basic -o - --print-go
```

To customize, edit `skeleton-agent.yaml` directly. `agent.LoadFile` reads it
through `spec.Decode`, which rejects unknown fields — typos surface
immediately at startup. No rebuild needed.

## When to reach for this pattern vs. code-agent's

| | skeleton-agent | code-agent |
| --- | --- | --- |
| flag surface | none | 11 flags |
| modes | 1 (stdin REPL → stdout) | 4 (sessions / -p / --json / interactive tui) |
| `agent.New` vs `MustNew` | MustNew | New (opts can fail) |
| `*Parts` exposure | none | Sessions / Skills / Engine / Cwd |
| `Engine.Sink` swap | WithSink(stdout) constant | per-mode (channel / wire / progress) |
| `agent.Option` blocks | WithSink | WithHooks(blockDestructiveBash()) |
| `app.Option` blocks | WithLogToStdout, WithRoundTimeout | none |
| `app.Interactive` | NextRound (stdin REPL) | tui steering loop |
| lines of Go (single binary) | ~200 | ~333 across 4 files |

## Files

```text
sample/skeleton-agent/
├── main.go              # stdinAgent + SinkFunc + agent.LoadFile(skeleton-agent.yaml) main()
├── skeleton-agent.yaml   # canonical YAML config loaded by main.go
├── go.mod               # depends only on the root module
└── README.md            # this file
```
