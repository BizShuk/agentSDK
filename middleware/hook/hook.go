// Package hook implements core.Hooks: lifecycle hook rules with in-process
// function handlers and external command handlers, following the claude-code
// hook contract (JSON event on stdin, JSON decision on stdout, exit code 2
// blocks the gated action).
//
// The package depends only on core — configuration loading and wiring into
// runtime.Engine happen at the composition root.
package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

const (
	// DEFAULT_COMMAND_TIMEOUT bounds one external hook command. Hooks fire
	// inline on the engine loop, so a hung command would hang the run.
	DEFAULT_COMMAND_TIMEOUT = 10 * time.Second

	// COMMAND_EXIT_BLOCK is the exit code an external command uses to block
	// the gated action; stderr becomes the block reason.
	COMMAND_EXIT_BLOCK = 2
)

// Handler responds to one HookEvent.
type Handler interface {
	Handle(ctx context.Context, ev core.HookEvent) (core.HookDecision, error)
}

// Func adapts an in-process function to Handler.
type Func func(ctx context.Context, ev core.HookEvent) (core.HookDecision, error)

// Handle implements Handler.
func (f Func) Handle(ctx context.Context, ev core.HookEvent) (core.HookDecision, error) {
	return f(ctx, ev)
}

// Command runs an external program per event: the HookEvent is written to
// stdin as JSON; exit 0 with a JSON HookDecision on stdout (or no output)
// proceeds, exit COMMAND_EXIT_BLOCK blocks with stderr as reason, any other
// failure is a hook infrastructure error.
type Command struct {
	Path    string
	Args    []string
	Timeout time.Duration // <= 0 → DEFAULT_COMMAND_TIMEOUT
}

// Handle implements Handler.
func (c Command) Handle(ctx context.Context, ev core.HookEvent) (core.HookDecision, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DEFAULT_COMMAND_TIMEOUT
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := json.Marshal(ev)
	if err != nil {
		return core.HookDecision{}, fmt.Errorf("hook event marshal: %w", err)
	}
	cmd := exec.CommandContext(cctx, c.Path, c.Args...)
	cmd.Stdin = bytes.NewReader(raw)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr == nil {
		out := bytes.TrimSpace(stdout.Bytes())
		if len(out) == 0 {
			return core.HookDecision{}, nil
		}
		var dec core.HookDecision
		if jerr := json.Unmarshal(out, &dec); jerr != nil {
			return core.HookDecision{}, fmt.Errorf("hook command output decode: %w", jerr)
		}
		return dec, nil
	}
	var xerr *exec.ExitError
	if errors.As(runErr, &xerr) && xerr.ExitCode() == COMMAND_EXIT_BLOCK {
		return core.HookDecision{Block: true, Reason: strings.TrimSpace(stderr.String())}, nil
	}
	return core.HookDecision{}, fmt.Errorf("hook command %q: %w", c.Path, runErr)
}

// Rule binds one event name and an optional tool matcher to handlers.
type Rule struct {
	Event    core.HookEventName
	Match    string // tool-name matcher: "" or "*" = all; glob with "|" alternation, e.g. "Edit|Write", "mcp__*"
	Handlers []Handler
}

func (r Rule) matches(ev core.HookEvent) bool {
	if r.Event != ev.Name {
		return false
	}
	return MatchTool(r.Match, ev.ToolName)
}

// MatchTool reports whether a tool name matches a rule matcher. Matchers are
// "|"-separated path.Match globs; empty or "*" match everything (including
// events with no tool name).
func MatchTool(pattern, name string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	for alt := range strings.SplitSeq(pattern, "|") {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		if ok, err := path.Match(alt, name); err == nil && ok {
			return true
		}
	}
	return false
}

// Runner implements core.Hooks: every matching rule's handlers fire in
// registration order and their decisions are merged (any Block wins, reasons
// and system notes accumulate, the last non-empty ReplaceArgs wins).
type Runner struct {
	Rules []Rule
}

// NewRunner builds a Runner over the given rules.
func NewRunner(rules ...Rule) *Runner {
	return &Runner{Rules: rules}
}

// Fire implements core.Hooks.
func (r *Runner) Fire(ctx context.Context, ev core.HookEvent) (core.HookDecision, error) {
	var merged core.HookDecision
	for _, rule := range r.Rules {
		if !rule.matches(ev) {
			continue
		}
		for _, h := range rule.Handlers {
			dec, err := h.Handle(ctx, ev)
			if err != nil {
				return merged, fmt.Errorf("hook %s: %w", ev.Name, err)
			}
			merged = mergeDecision(merged, dec)
		}
	}
	return merged, nil
}

func mergeDecision(a, b core.HookDecision) core.HookDecision {
	if b.Block {
		a.Block = true
	}
	if b.Reason != "" {
		if a.Reason == "" {
			a.Reason = b.Reason
		} else {
			a.Reason += "; " + b.Reason
		}
	}
	if b.SystemNote != "" {
		if a.SystemNote == "" {
			a.SystemNote = b.SystemNote
		} else {
			a.SystemNote += "\n" + b.SystemNote
		}
	}
	if len(b.ReplaceArgs) > 0 {
		a.ReplaceArgs = b.ReplaceArgs
	}
	return a
}
