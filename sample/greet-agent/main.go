// Command greet-agent is the minimal sample that demonstrates agentsdk
// by greeting a person by name. The agent configuration is loaded from
// greet-agent.yaml — the canonical wizard output equivalent — and the
// custom `greet` tool is registered through agent.WithTools (the
// closure surface, not the data surface, so it stays in code).
//
// Run:
//
//	echo "World" | go run .
//	# → "Hello, World!" via the registered greet tool
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/cli"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/sample/greet-agent/tool"
	"github.com/bizshuk/agentsdk/utils/agentconfig"
)

// configPath is the canonical config file for this sample. It sits next
// to main.go so the sample stays runnable from one location without
// touching global flag state.
const configPath = "greet-agent.yaml"

var stdinLines = make(chan string)

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

// greetAgent embeds *agent.Agent and overrides Bootstrap to inject the
// greeting request. The name is read from stdin first line; empty stdin
// falls back to "World" (matches the previous default behaviour).
type greetAgent struct {
	*agent.Agent
}

// Bootstrap reads the opening prompt and seeds the user message asking
// the agent to greet the named person via the greet tool.
func (g greetAgent) Bootstrap(ctx context.Context, ac *agent.Host) (*agent.Engine, core.State, error) {
	engine, state, err := g.Agent.Bootstrap(ctx, ac)
	if err != nil {
		return engine, state, err
	}
	name, _ := nextLine(ctx)
	if name == "" {
		name = "World"
	}
	msg := fmt.Sprintf("Please greet %s using the greet tool.", name)
	state.Messages = append(state.Messages, core.Message{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: msg}},
		Ts:    time.Now().UTC(),
	})
	return engine, state, nil
}

func main() {
	go readStdin()

	cfg, err := agentconfig.LoadFile(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}

	// JSONL envelope per stream event. Same shape as the previous
	// writeEnvelope(cmd, eff core.Instruction) used to emit, translated
	// from Instruction to StreamEvent at the sink boundary.
	sink := agent.SinkFunc(func(ev core.StreamEvent) {
		out, _ := json.Marshal(struct {
			Type string `json:"type"`
			Kind string `json:"kind,omitempty"`
			Text string `json:"text,omitempty"`
		}{
			Type: "event",
			Kind: string(ev.Kind),
			Text: ev.Text,
		})
		fmt.Fprintln(os.Stdout, string(out))
	})

	cli.Main(
		greetAgent{agent.MustNew(cfg,
			agent.WithSink(sink),
			agent.WithToolRegistrar(tool.NewGreet().Register),
		)},
		agent.WithLogToStdout(),
	)
}
