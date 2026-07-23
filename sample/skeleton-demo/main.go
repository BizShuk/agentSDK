// Command skeleton-demo is the single-file sample that demonstrates the
// agent skeleton in its canonical, wizard-generated form:
//
//	func main() { app.Main(agent.MustNew(cfg, opts...), appOpts...) }
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
// skeleton-demo is the case where none of that is needed: the persona
// is the task, the input is stdin, the output is one line on stdout.
// Three small adjustments to the wizard template make that work:
//
//  1. stdinAgent embeds *agent.Agent and overrides Bootstrap only, so
//     the actual main() stays short.
//  2. WithSink(SinkFunc) prints the assistant text to stdout. Without
//     this the agent's replies land in the file-backed slog handler at
//     ~/.config/skeleton-demo/logs/ — the operator sees nothing.
//  3. WithLogToStdout() makes any preflight / run-time failure visible
//     on the terminal instead of vanishing into the same log file. A
//     production CLI would keep the default file handler; this is a
//     demo, so tradeoffs are explicit.
//
// Run:
//
//	echo "Payment page throws 500 on /checkout" \
//	  | go run ./sample/skeleton-demo
//
//	→ stdout: P1|HTTP 500 on /checkout blocks paid conversions but page is reachable
//
// Missing API key on the path MINIMAX_API_KEY (for the minimax provider)
// now produces a visible error line instead of silent exit 1:
//
//	export MINIMAX_API_KEY=...  # or pass --provider anthropic + ANTHROPIC_API_KEY
//	go run ./sample/skeleton-demo < ticket.txt
//
// Then compare against the wizard's literal output:
//
//	go run . w -y --tier basic -o - --print-go
package main

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/app"
	appconfig "github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/runtime"
)

// stdinAgent embeds *agent.Agent and overrides Bootstrap only. Every
// other call delegates, so the closing main() stays close to the
// wizard template.
type stdinAgent struct{ *agent.Agent }

// Bootstrap reads stdin into the opening state's first user message.
// If the operator forgot to pipe input, the agent runs with the persona
// only — equivalent to the provider CLI's `provider "ask me anything"`
// with no follow-up.
func (s stdinAgent) Bootstrap(ctx context.Context, ac *appconfig.AppConfig) (*runtime.Engine, core.State, error) {
	engine, state, err := s.Agent.Bootstrap(ctx, ac)
	if err != nil {
		return engine, state, err
	}
	text, _ := io.ReadAll(os.Stdin)
	if prompt := strings.TrimSpace(string(text)); prompt != "" {
		state.Messages = append(state.Messages, core.Message{
			Role:  core.ROLE_USER,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: prompt}},
			Ts:    time.Now().UTC(),
		})
	}
	return engine, state, nil
}

func main() {
	// Mostly the wizard --print-go output for -t basic, with three edits:
	//
	//   - Persona: ""        (no system instruction — the model uses its
	//     default helpful-assistant behavior; "say hello" replies "hello",
	//     not "P3|User request is not a support issue...")
	//   - Reasoning.Style: "think_then_act"  (multi-turn loop; the
	//     wizard --print-go -t basic default; one_shot would cap at 1)
	//   - Limits.MaxTurns: 50  (a REPL-style sample keeps budget room
	//     for back-and-forth; MaxTurns is loop iterations, not calls)
	//
	// The wizard `goLiteral` would emit this same Config if you answered
	// the prompt-driven session with these values; we just write it
	// directly to keep the file self-contained.
	cfg := agent.Config{
		Name:      "skeleton-demo",
		Tier:      spec.TIER_BASIC,
		Model:     agent.Model{Provider: "minimax"},
		Reasoning: agent.Reasoning{Style: "think_then_act"},
		Limits:    agent.Limits{MaxTurns: 50, Autonomy: "L1"},
	}

	// The seam the wizard template does NOT cover: who renders the reply.
	// Without WithSink the engine's emit is nil and the verdict flows
	// only to the file-backed slog handler. WithSink is the literal
	// three-line wrapper that turns this into a useful CLI.
	sink := agent.SinkFunc(func(ev core.StreamEvent) {
		if ev.Kind == core.STREAM_MESSAGE && ev.Text != "" {
			os.Stdout.WriteString(ev.Text)
		}
	})

	// This is the line the tutorial and the wizard both point at.
	// The first Option drives the engine; the second Option drives the
	// lifecycle logger. Both are necessary for a demo to be observable
	// in 2026 default terminals (which lack a configured slog handler).
	app.Main(
		stdinAgent{agent.MustNew(cfg, agent.WithSink(sink))},
		app.WithLogToStdout(),
	)
}
