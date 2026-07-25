# logdoctor

Watches a log file, diagnoses errors, and queues fixes. Five subcommands
cover the operational lifecycle:

```text
logdoctor run        # single pass against a log file
logdoctor watch      # continuous tail with auto-dispatch
logdoctor resume     # resume a paused run from StateStore + WAL
logdoctor list       # list persisted runs (StateStore metadata)
logdoctor approve    # out-of-band decision on a PendingApproval
```

## Why no `agent.yaml`

logdoctor is intentionally NOT in the `agent.LoadFile` + `agent.Main`
shape that skeleton-demo, code-agent, file-agent, and greet-agent use.

Two reasons:

1. **Multi-subcommand UX, not single-shot CLI.** `run` / `watch` /
   `resume` / `list` / `approve` are five distinct entry points with
   different state and flag surfaces. The skeleton-demo pattern (one
   `main`, one YAML, one `agent.Main`) collapses this into a single
   invocation, which loses the operational UX.

2. **Direct core.State lifecycle.** The sample demonstrates
   `StateStore` + `WriteAheadLog` resume, paused-run recovery, and
   out-of-band approval — the same surfaces agent.Runner hides behind
   `agent.Main`. Pulling those into spec.Config would require expanding
   the schema with operational knobs (`resume.runID`, `watch.path`,
   `approve.decision`), which is a different problem from "agent runs
   on stdin".

The split is intentional:

| Sample | agent.Config? | Pattern |
| --- | --- | --- |
| skeleton-demo | yes | single-file, agent.Main, YAML |
| code-agent | yes (built from flags) | cobra, agent.Runner, 4 modes |
| file-agent | yes (loaded from YAML) | single-file, agent.Main, YAML |
| greet-agent | yes (loaded from YAML) | single-file, agent.Main, YAML |
| logdoctor | **no** | cobra subcommands, direct core.State |

If a future change moves logdoctor onto the agent skeleton (e.g. via
`agent.Runner.RunOnce` + `agent.Runner.Resume(runID)`), this README and
the operational-knob design question are the first things to revisit.
