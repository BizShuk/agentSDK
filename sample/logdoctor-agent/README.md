# logdoctor

Continuously reads new bytes from direct files under
`~/.config/*/logs/*`, asks MiniMax for a read-only diagnosis, and prints
safe fix suggestions.

The sample has one operation, `watch`. It uses MiniMax through
`MINIMAX_API_KEY`; there is no provider selector, fake mode, tool registry,
approval flow, or agent session lifecycle.

```bash
export MINIMAX_API_KEY=...
go run . watch --interval 1m
```

Each cycle reads at most `1 MiB` of raw log bytes. The first cycle runs
immediately; later cycles run after the configured interval. Markdown
diagnoses go to stdout and canonical `core.StreamEvent` JSONL goes to
stderr. Stop the foreground process with `Ctrl-C`.
