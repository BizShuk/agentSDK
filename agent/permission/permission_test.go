package permission

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
)

func callTool(name string, args map[string]any) core.CallToolInstruction {
	return core.CallToolInstruction{Call: core.ToolCall{ID: "t1", Name: name, Args: args}}
}

func TestMatchTarget(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    bool
	}{
		{"command prefix hit", "git:*", "git status", true},
		{"command prefix exact", "git:*", "git", true},
		{"command prefix miss", "git:*", "gitk", false},
		{"star within segment", "*.go", "main.go", true},
		{"star does not cross slash", "*.go", "cmd/main.go", false},
		{"doublestar crosses slash", "src/**", "src/a/b/c.go", true},
		{"doublestar tail", "**/*.go", "a/b/c.go", true},
		{"doublestar zero segments", "**/*.go", "main.go", true},
		{"doublestar miss", "src/**", "lib/a.go", false},
		{"question mark", "v?", "v1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MatchTarget(tt.pattern, tt.target))
		})
	}
}

func TestMatchSpec(t *testing.T) {
	tests := []struct {
		name   string
		spec   string
		tool   string
		target string
		want   bool
	}{
		{"star", "*", "Bash", "anything", true},
		{"bare tool", "Bash", "Bash", "rm -rf /", true},
		{"bare tool miss", "Bash", "Edit", "", false},
		{"tool with pattern", "Bash(git:*)", "Bash", "git push", true},
		{"tool with pattern miss", "Bash(git:*)", "Bash", "rm -rf /", false},
		{"path glob", "Edit(src/**)", "Edit", "src/core/x.go", true},
		{"path glob miss", "Edit(src/**)", "Edit", "docs/a.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MatchSpec(tt.spec, tt.tool, tt.target))
		})
	}
}

func TestEngineRulePrecedence(t *testing.T) {
	e := &Engine{
		Mode: MODE_DEFAULT,
		Rules: []Rule{
			{BEHAVIOR_ALLOW, "Bash(git:*)"},
			{BEHAVIOR_DENY, "Bash(git push:*)"},
			{BEHAVIOR_ASK, "Bash"},
		},
	}
	low := core.ToolSpec{Name: "Bash", Risk: core.RISK_LEVEL_LOW}

	// deny beats allow even though allow also matches
	got := e.Decide(struct{}{}, core.AUTONOMY_L2, callTool("Bash", map[string]any{"command": "git push origin"}), low)
	assert.Equal(t, core.APPROVAL_ACTION_DENY, got)

	// allow matches, ask also matches — but deny > ask > allow means ask wins
	got = e.Decide(struct{}{}, core.AUTONOMY_L2, callTool("Bash", map[string]any{"command": "git status"}), low)
	assert.Equal(t, core.APPROVAL_ACTION_ASK, got)

	// no bash rule matches Edit → falls through to default (low risk → allow)
	got = e.Decide(struct{}{}, core.AUTONOMY_L2, callTool("Edit", map[string]any{"file_path": "a.go"}), core.ToolSpec{Name: "Edit", Risk: core.RISK_LEVEL_LOW})
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, got)
}

func TestEngineModes(t *testing.T) {
	high := core.ToolSpec{Name: "Bash", Risk: core.RISK_LEVEL_HIGH}
	low := core.ToolSpec{Name: "Read", Risk: core.RISK_LEVEL_LOW}
	bash := callTool("Bash", map[string]any{"command": "rm -rf /"})
	read := callTool("Read", map[string]any{"path": "a.go"})

	tests := []struct {
		name string
		mode Mode
		eff  core.CallToolInstruction
		spec core.ToolSpec
		want core.ApprovalAction
	}{
		{"bypass allows high risk", MODE_BYPASS, bash, high, core.APPROVAL_ACTION_ALLOW},
		{"plan denies high risk", MODE_PLAN, bash, high, core.APPROVAL_ACTION_DENY},
		{"plan allows low risk", MODE_PLAN, read, low, core.APPROVAL_ACTION_ALLOW},
		{"acceptEdits asks high risk", MODE_ACCEPT_EDITS, bash, high, core.APPROVAL_ACTION_ASK},
		{"acceptEdits allows low risk", MODE_ACCEPT_EDITS, read, low, core.APPROVAL_ACTION_ALLOW},
		{"default asks high risk without fallback", MODE_DEFAULT, bash, high, core.APPROVAL_ACTION_ASK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Engine{Mode: tt.mode}
			assert.Equal(t, tt.want, e.Decide(struct{}{}, core.AUTONOMY_L2, tt.eff, tt.spec))
		})
	}
}

type fallbackDeny struct{}

func (fallbackDeny) Decide(struct{}, core.AutonomyLevel, core.CallToolInstruction, core.ToolSpec) core.ApprovalAction {
	return core.APPROVAL_ACTION_DENY
}

func TestEngineFallbackInjection(t *testing.T) {
	e := &Engine{Mode: MODE_DEFAULT, Fallback: fallbackDeny{}}
	got := e.Decide(struct{}{}, core.AUTONOMY_L2, callTool("Read", nil), core.ToolSpec{Risk: core.RISK_LEVEL_LOW})
	assert.Equal(t, core.APPROVAL_ACTION_DENY, got)
}

func TestEngineCustomTarget(t *testing.T) {
	e := &Engine{
		Mode:  MODE_DEFAULT,
		Rules: []Rule{{BEHAVIOR_DENY, "Fetch(https://internal.corp/**)"}},
		Targets: map[string]TargetFunc{
			"Fetch": func(c core.ToolCall) string { return c.Args["url"].(string) },
		},
	}
	got := e.Decide(struct{}{}, core.AUTONOMY_L2,
		callTool("Fetch", map[string]any{"url": "https://internal.corp/secrets"}),
		core.ToolSpec{Name: "Fetch", Risk: core.RISK_LEVEL_LOW})
	assert.Equal(t, core.APPROVAL_ACTION_DENY, got)
}
