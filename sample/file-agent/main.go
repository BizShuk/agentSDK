// Command file-agent is a sample that demonstrates agentsdk by driving the
// six built-in tools (Read/Write/Edit/Bash/Glob/Grep) to operate on files.
//
// Per-run inputs (target, task) come from argv[1] and stdin first line.
// The agent configuration itself is loaded from file-agent.yaml — the
// canonical wizard output equivalent. Credentials resolve through viper
// from .env / config.yaml / shell env; missing credentials surface as a
// visible startup error (no silent fallbacks).
//
// Run:
//
//	echo "summarize the files" | go run . ./pkg
//	# → JSONL envelope on stdout, one event per line
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
	appconfig "github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/runtime"
)

// configPath is the canonical config file for this sample. It sits next
// to main.go so the sample stays runnable from one location without
// touching global flag state.
const configPath = "file-agent.yaml"

// stdinLines feeds Bootstrap (opening task) and NextRound (follow-ups).
// A single owning goroutine is deliberate: a round that times out
// abandons its nextLine call, and one owner means there is never a
// second Scan racing the first for the same input.
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

// fileAgent embeds *agent.Agent and overrides Bootstrap to inject the
// task message. The target path is captured at construction so main()
// can pass it without polluting the agent.Config surface.
type fileAgent struct {
	*agent.Agent
	target string
}

// Bootstrap reads the opening task from stdin and seeds the user
// message. Empty stdin runs with the persona only — same shape as
// `provider "ask me anything"` with no follow-up.
func (f fileAgent) Bootstrap(ctx context.Context, ac *appconfig.AppConfig) (*runtime.Engine, core.State, error) {
	engine, state, err := f.Agent.Bootstrap(ctx, ac)
	if err != nil {
		return engine, state, err
	}
	task, _ := nextLine(ctx)
	msg := fmt.Sprintf("%s\nTarget path: %s", task, f.target)
	state.Messages = append(state.Messages, core.Message{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: msg}},
		Ts:    time.Now().UTC(),
	})
	return engine, state, nil
}

func main() {
	go readStdin()

	cfg, err := agent.LoadFile(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}

	// Optional first positional arg: target path (default cwd). Flags
	// come before this if any are added later.
	target := "."
	for _, a := range os.Args[1:] {
		if !strings.HasPrefix(a, "-") {
			target = a
			break
		}
	}

	// JSONL envelope per stream event. Same shape as the previous
	// writeEnvelope(cmd, eff core.Instruction) used to emit, just
	// translated from Instruction to StreamEvent at the sink boundary.
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

	agent.Main(
		fileAgent{Agent: agent.MustNew(cfg, agent.WithSink(sink)), target: target},
		agent.WithLogToStdout(),
	)
}
