// Command skeleton-agent is the single-file sample that demonstrates the
// agent skeleton in its canonical, wizard-generated form:
//
//	func main() { cli.Main(agent.MustNew(cfg, opts...), runOpts...) }
//
// Compare to sample/code-agent, which uses cobra + a 101-line compose()
// + four dispatch modes (interactive / -p / --json / --sessions). That
// structure is what you reach for when an application needs:
//
//   - flag surface with many switches
//   - per-mode Engine.Sink (tui channel / wire.NewSink / progressSink)
//   - direct access to *Parts.Sessions and *Parts.Skills
//   - agent.Option closures that can fail (returned via agent.New, not MustNew)
//
// skeleton-agent is the case where none of that is needed: the persona
// is the task, the input is stdin, the output is the assistant text on
// stdout. Four small adjustments to the wizard template make that work:
//
//  1. stdinAgent embeds *agent.Agent and overrides Bootstrap only, so
//     the actual main() stays short.
//  2. WithSink(SinkFunc) prints the assistant text to stdout. Without
//     this the agent's replies land in the file-backed slog handler at
//     ~/.config/skeleton-agent/logs/ — the operator sees nothing.
//  3. WithLogToStdout() makes any preflight / run-time failure visible
//     on the terminal instead of vanishing into the same log file. A
//     production CLI would keep the default file handler; this is a
//     demo, so tradeoffs are explicit.
//  4. NextRound makes it a small REPL: the first stdin line is the
//     opening prompt, each later line is a follow-up round, and a blank
//     line or EOF ends the session. This is the agent.Interactive seam —
//     the same method also answers approval pauses, shown here for
//     completeness even though this config gates nothing.
//
// Run (one-shot):
//
//	echo "Payment page throws 500 on /checkout" \
//	  | go run ./sample/skeleton-agent
//
// Run (REPL — one line per round, blank line to finish):
//
//	printf 'first question\nfollow-up question\n\n' \
//	  | go run ./sample/skeleton-agent
//	# or interactively:
//	go run ./sample/skeleton-agent
//
// Missing API key on the path MINIMAX_API_KEY (for the minimax provider)
// produces a visible error line instead of a silent exit 1:
//
//	export MINIMAX_API_KEY=...  # or pass --provider anthropic + ANTHROPIC_API_KEY
//	go run ./sample/skeleton-agent < ticket.txt
//
// Then compare against the wizard's literal output:
//
//	go run . w -y --tier basic -o - --print-go
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/cli"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/utils/agentconfig"
)

// stdinLines is fed by a single background reader (readStdin) so both
// Bootstrap (opening prompt) and NextRound (each follow-up) pull lines
// from one place. A single owning goroutine is deliberate: a round that
// times out abandons its nextLine call, and one owner means there is
// never a second Scan racing the first for the same input.
var stdinLines = make(chan string)

// readStdin owns os.Stdin and streams trimmed lines until EOF. A scan
// error (rare on stdin) ends the stream the same as EOF — the consumer
// treats a closed channel as "no more input" either way.
func readStdin() {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		stdinLines <- sc.Text()
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "stdin:", err)
	}
	close(stdinLines)
}

// nextLine returns the next stdin line. ok is false on EOF or ctx
// cancellation (a round deadline or SIGINT), so a blocked read never
// pins the process open.
func nextLine(ctx context.Context) (line string, ok bool) {
	select {
	case <-ctx.Done():
		return "", false
	case l, open := <-stdinLines:
		if !open {
			return "", false
		}
		return strings.TrimSpace(l), true
	}
}

// stdinAgent embeds *agent.Agent and adds the two seams a REPL needs:
// Bootstrap seeds the opening prompt, NextRound feeds follow-ups. Every
// other call delegates, so main() stays close to the wizard template.
type stdinAgent struct{ *agent.Agent }

// Bootstrap seeds the opening prompt from the first stdin line. Later
// lines are delivered round by round through NextRound. An empty first
// line (immediate EOF) runs with the persona only — the provider CLI's
// `provider "ask me anything"` with no follow-up.
func (s stdinAgent) Bootstrap(ctx context.Context, ac *agent.Host) (*agent.Engine, core.State, error) {
	engine, state, err := s.Agent.Bootstrap(ctx, ac)
	if err != nil {
		return engine, state, err
	}
	if prompt, ok := nextLine(ctx); ok && prompt != "" {
		state.Messages = append(state.Messages, core.Message{
			Role:  core.ROLE_USER,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: prompt}},
			Ts:    time.Now().UTC(),
		})
	}
	return engine, state, nil
}

// NextRound is the agent.Interactive seam. A finished round offers one more
// line of input; a blank line or EOF ends the run. The approval branch is
// wired for completeness — this config gates nothing, so it stays dark
// unless you add a tool and tighten autonomy — and shows that the same
// single method answers both "approve this?" and "what next?".
func (s stdinAgent) NextRound(ctx context.Context, p agent.Pause) (agent.Resume, error) {
	if p.Reason == agent.PAUSE_APPROVAL {
		fmt.Fprintf(os.Stderr, "\n[approval] %s — approve? [y/N] ", pendingLabel(p.State))
		line, _ := nextLine(ctx)
		if line == "y" || line == "yes" {
			return agent.Resume{Decision: core.APPROVAL_DECISION_APPROVE, By: "operator"}, nil
		}
		return agent.Resume{Decision: core.APPROVAL_DECISION_REJECT, By: "operator"}, nil
	}
	// PAUSE_ROUND_END: offer a follow-up. Blank / EOF → empty Input → stop.
	fmt.Fprint(os.Stderr, "\n> ")
	line, ok := nextLine(ctx)
	if !ok {
		return agent.Resume{}, nil
	}
	return agent.Resume{Input: line}, nil
}

// pendingLabel renders the open approval for the operator prompt.
func pendingLabel(s core.State) string {
	n := len(s.PendingApprovals)
	if n == 0 {
		return "(pending)"
	}
	pa := s.PendingApprovals[n-1]
	if pa.ToolCall != nil {
		return "tool " + pa.ToolCall.Name + " (" + pa.Reason + ")"
	}
	return pa.Reason
}

// configPath is the canonical config file for this sample. It sits next
// to main.go so the sample stays runnable from one location without
// touching global flag state.
const configPath = "skeleton-agent.yaml"

func main() {
	// The opening prompt and every follow-up come from stdin; start the
	// reader before the lifecycle so the first nextLine in Bootstrap has a
	// source.
	go readStdin()

	// Load the agent config from skeleton-agent.yaml — same schema the
	// wizard writes and `agentsdk run -c path` consumes. agentconfig.LoadFile
	// reads YAML/JSON based on extension and runs agentconfig.Decode
	// (which rejects unknown fields, so typos surface immediately).
	cfg, err := agentconfig.LoadFile(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}

	// The seam the wizard template does NOT cover: who renders the reply.
	// Without WithSink the engine's emit is nil and the verdict flows only
	// to the file-backed slog handler. The trailing newline keeps one
	// round's answer from running into the next round's prompt.
	sink := agent.SinkFunc(func(ev core.StreamEvent) {
		if ev.Kind == core.STREAM_MESSAGE && ev.Text != "" {
			os.Stdout.WriteString(ev.Text + "\n")
		}
	})

	// This is the line the tutorial and the wizard both point at. The
	// first Option drives the engine; WithLogToStdout drives the lifecycle
	// logger; WithRoundTimeout bounds how long one follow-up read may block
	// before the REPL gives up on an idle operator.
	cli.Main(
		stdinAgent{agent.MustNew(cfg, agent.WithSink(sink))},
		agent.WithLogToStdout(),
		agent.WithRoundTimeout(2*time.Minute),
	)
}
