// interactive.go is the tui-backed REPL: differential transcript region,
// spinner while the engine runs, and cooked-mode line input. Input typed
// while a run is active becomes an Engine.Steer message — the pi steering
// UX on top of the skeleton tui (raw-mode editor arrives later).
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/sample/code-agent/tui"
)

const (
	RENDER_INTERVAL = 120 * time.Millisecond
	MAX_TRANSCRIPT  = 400 // lines kept in the render region
	HINT_IDLE       = "輸入任務；/help 看指令；/quit 離開"
	HINT_RUNNING    = "執行中… 這時輸入的文字會用 Steer 插話"
)

type runResult struct {
	state core.State
	err   error
}

// ui owns the render model. All mutations happen on the event loop
// goroutine; the sink forwards events through a channel.
type ui struct {
	renderer   *tui.Renderer
	transcript []string
	loader     *tui.Loader
	running    bool
	hint       string
}

func (u *ui) append(lines ...string) {
	u.transcript = append(u.transcript, lines...)
	if n := len(u.transcript); n > MAX_TRANSCRIPT {
		u.transcript = u.transcript[n-MAX_TRANSCRIPT:]
	}
}

func (u *ui) render() {
	children := []tui.Component{tui.Text{Content: strings.Join(u.transcript, "\n")}}
	if u.running {
		u.loader.Tick()
		children = append(children, u.loader)
	}
	children = append(children, tui.Rule{}, tui.Text{Content: u.hint})
	_ = u.renderer.Flush(tui.Container{Children: children})
}

// chanSink forwards engine StreamEvents onto the event loop.
type chanSink struct{ ch chan core.StreamEvent }

func (c chanSink) OnStreamEvent(ev core.StreamEvent) { c.ch <- ev }

// runInteractive drives the REPL until /quit, EOF, or ctx cancel.
func runInteractive(ctx context.Context, parts *agentParts, state core.State) error {
	events := make(chan core.StreamEvent, 64)
	parts.Engine.Sink = chanSink{ch: events}

	u := &ui{
		renderer: tui.NewRenderer(tui.NewProcessTerminal()),
		loader:   &tui.Loader{Message: "thinking"},
		hint:     HINT_IDLE,
	}
	u.append(
		"code-agent — 全 harness 組合 demo（session: "+state.RunID+"）",
		"tools: read/write/edit/bash/glob/grep（+ task，若有 agents 定義）",
		"",
	)

	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			lines <- sc.Text()
		}
		_ = sc.Err() // stdin read error → treat as EOF; loop exits on close
		close(lines)
	}()

	done := make(chan runResult, 1)
	ticker := time.NewTicker(RENDER_INTERVAL)
	defer ticker.Stop()
	u.render()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			if u.running {
				u.render()
			}

		case ev := <-events:
			u.append(formatEvent(ev)...)
			u.render()

		case res := <-done:
			u.running = false
			u.hint = HINT_IDLE
			if res.err != nil {
				u.append("✗ run 失敗: "+res.err.Error(), "")
			} else {
				state = res.state
				u.append("")
			}
			u.render()

		case line, ok := <-lines:
			if !ok {
				return nil // stdin EOF
			}
			quit, newState := handleLine(parts, u, state, strings.TrimSpace(line), done)
			if quit {
				return nil
			}
			state = newState
			u.render()
		}
	}
}

// handleLine routes one input line: steering when running, slash commands,
// or a new run. Returns (quit, state).
func handleLine(parts *agentParts, u *ui, state core.State, line string, done chan runResult) (bool, core.State) {
	if line == "" {
		return false, state
	}
	if u.running {
		parts.Engine.Steer(line)
		u.append("↳ (steer) " + line)
		return false, state
	}
	if strings.HasPrefix(line, "/") {
		name, rest, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
		switch name {
		case "quit", "exit":
			return true, state
		case "help":
			u.append(helpLines(parts)...)
			return false, state
		case "sessions":
			metas, err := parts.Sessions.List(parts.Cwd)
			if err != nil {
				u.append("✗ " + err.Error())
				return false, state
			}
			for _, m := range metas {
				u.append(fmt.Sprintf("  %s  %s", m.ID, m.CreatedAt.Local().Format("01-02 15:04")))
			}
			u.append("")
			return false, state
		default:
			expanded, err := parts.Skills.ExpandCommand(name, strings.TrimSpace(rest))
			if err != nil {
				u.append("✗ 未知指令 /" + name + "（/help 看清單）")
				return false, state
			}
			return false, startRun(parts, u, state, expanded, "/"+name+" "+rest, done)
		}
	}
	return false, startRun(parts, u, state, line, line, done)
}

// startRun appends the user message and drives the engine off-loop.
func startRun(parts *agentParts, u *ui, state core.State, prompt, display string, done chan runResult) core.State {
	u.append("❯ "+strings.TrimSpace(display), "")
	u.running = true
	u.hint = HINT_RUNNING
	state.Messages = append(state.Messages, userMessage(prompt))
	go func(st core.State) {
		final, err := parts.Engine.Run(context.Background(), st)
		done <- runResult{state: final, err: err}
	}(state)
	return state
}

func formatEvent(ev core.StreamEvent) []string {
	switch ev.Kind {
	case core.STREAM_MESSAGE:
		var out []string
		for i, l := range strings.Split(strings.TrimSpace(ev.Text), "\n") {
			prefix := "  "
			if i == 0 {
				prefix = "● "
			}
			out = append(out, prefix+l)
		}
		return out
	case core.STREAM_TOOL_START:
		if ev.ToolCall != nil {
			return []string{"→ " + ev.ToolCall.Name + toolTarget(ev.ToolCall)}
		}
	case core.STREAM_TOOL_RESULT:
		if ev.ToolResult != nil && !ev.ToolResult.OK {
			return []string{"← " + ev.ToolResult.Name + " ✗ " + ev.ToolResult.Error}
		}
		if ev.ToolResult != nil {
			return []string{"← " + ev.ToolResult.Name + " ✓"}
		}
	}
	return nil
}

func toolTarget(tc *core.ToolCall) string {
	for _, key := range []string{"command", "path", "file_path", "pattern"} {
		if v, ok := tc.Args[key].(string); ok && v != "" {
			return " " + tui.TruncateToWidth(v, 48)
		}
	}
	return ""
}

func helpLines(parts *agentParts) []string {
	out := []string{
		"/help          顯示本清單",
		"/sessions      列出此目錄的 sessions",
		"/quit          離開",
	}
	for _, c := range parts.Skills.Commands() {
		out = append(out, "/"+c.Name+"  （slash command）")
	}
	for _, s := range parts.Skills.Skills() {
		out = append(out, "skill: "+s.Name+" — "+s.Description)
	}
	return append(out, "")
}
