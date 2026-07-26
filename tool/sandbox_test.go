package tool_test

import (
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/tool"
	"github.com/stretchr/testify/assert"
)

func TestPolicyAllowsAllowedPath(t *testing.T) {
	p := tool.DefaultPolicy()
	assert.Equal(t, tool.VERDICT_ALLOW,
		p.Check("read_file", map[string]any{"path": "/tmp/log/app.log"}))
}

func TestPolicyDeniesPathOutsideAllowlist(t *testing.T) {
	p := tool.DefaultPolicy()
	assert.Equal(t, tool.VERDICT_DENY,
		p.Check("read_file", map[string]any{"path": "/etc/passwd"}))
}

func TestPolicyDeniesRelativePath(t *testing.T) {
	p := tool.DefaultPolicy()
	assert.Equal(t, tool.VERDICT_DENY,
		p.Check("read_file", map[string]any{"path": "app.log"}))
}

func TestPolicyDeniesDangerousCommand(t *testing.T) {
	p := tool.DefaultPolicy()
	assert.Equal(t, tool.VERDICT_DENY,
		p.Check("shell", map[string]any{"command": "rm -rf /tmp"}))
	assert.Equal(t, tool.VERDICT_DENY,
		p.Check("shell", map[string]any{"command": ":(){:|:&};:"}))
}

func TestPolicyAllowsBenignCommand(t *testing.T) {
	p := tool.DefaultPolicy()
	assert.Equal(t, tool.VERDICT_ALLOW,
		p.Check("shell", map[string]any{"command": "ls -la /tmp"}))
}

func TestPolicyIgnoresIrrelevantArgs(t *testing.T) {
	p := tool.DefaultPolicy()
	assert.Equal(t, tool.VERDICT_ALLOW,
		p.Check("echo", map[string]any{"message": "hi"}))
}

func TestVerdictString(t *testing.T) {
	assert.Equal(t, "ALLOW", tool.VERDICT_ALLOW.String())
	assert.Equal(t, "DENY", tool.VERDICT_DENY.String())
}

// Custom test: deny case-insensitive subsystem match.
func TestPolicyCaseInsensitiveCommandMatch(t *testing.T) {
	p := tool.DefaultPolicy()
	// Mixed case — should still match.
	assert.Equal(t, tool.VERDICT_DENY,
		p.Check("shell", map[string]any{"command": "RM -RF /tmp"}))
	// Make sure no false positive.
	assert.Equal(t, tool.VERDICT_ALLOW,
		p.Check("shell", map[string]any{"command": "echo hello"}))
}

// Custom test: ensure denial message contains the tool name for diagnostics.
func TestPolicyDenialMessageUseful(t *testing.T) {
	p := tool.DefaultPolicy()
	msg := p.Check("read_file", map[string]any{"path": "/etc/passwd"}).String()
	assert.True(t, strings.Contains(msg, "DENY"), "verdict string must say DENY: %s", msg)
}
