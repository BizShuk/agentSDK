package action_test

import (
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/stretchr/testify/assert"
)

func TestPolicyAllowsAllowedPath(t *testing.T) {
	p := action.DefaultPolicy()
	assert.Equal(t, action.VERDICT_ALLOW,
		p.Check("read_file", map[string]any{"path": "/tmp/log/app.log"}))
}

func TestPolicyDeniesPathOutsideAllowlist(t *testing.T) {
	p := action.DefaultPolicy()
	assert.Equal(t, action.VERDICT_DENY,
		p.Check("read_file", map[string]any{"path": "/etc/passwd"}))
}

func TestPolicyDeniesRelativePath(t *testing.T) {
	p := action.DefaultPolicy()
	assert.Equal(t, action.VERDICT_DENY,
		p.Check("read_file", map[string]any{"path": "app.log"}))
}

func TestPolicyDeniesDangerousCommand(t *testing.T) {
	p := action.DefaultPolicy()
	assert.Equal(t, action.VERDICT_DENY,
		p.Check("shell", map[string]any{"command": "rm -rf /tmp"}))
	assert.Equal(t, action.VERDICT_DENY,
		p.Check("shell", map[string]any{"command": ":(){:|:&};:"}))
}

func TestPolicyAllowsBenignCommand(t *testing.T) {
	p := action.DefaultPolicy()
	assert.Equal(t, action.VERDICT_ALLOW,
		p.Check("shell", map[string]any{"command": "ls -la /tmp"}))
}

func TestPolicyIgnoresIrrelevantArgs(t *testing.T) {
	p := action.DefaultPolicy()
	assert.Equal(t, action.VERDICT_ALLOW,
		p.Check("echo", map[string]any{"message": "hi"}))
}

func TestVerdictString(t *testing.T) {
	assert.Equal(t, "ALLOW", action.VERDICT_ALLOW.String())
	assert.Equal(t, "DENY", action.VERDICT_DENY.String())
}

// Custom test: deny case-insensitive subsystem match.
func TestPolicyCaseInsensitiveCommandMatch(t *testing.T) {
	p := action.DefaultPolicy()
	// Mixed case — should still match.
	assert.Equal(t, action.VERDICT_DENY,
		p.Check("shell", map[string]any{"command": "RM -RF /tmp"}))
	// Make sure no false positive.
	assert.Equal(t, action.VERDICT_ALLOW,
		p.Check("shell", map[string]any{"command": "echo hello"}))
}

// Custom test: ensure denial message contains the tool name for diagnostics.
func TestPolicyDenialMessageUseful(t *testing.T) {
	p := action.DefaultPolicy()
	msg := p.Check("read_file", map[string]any{"path": "/etc/passwd"}).String()
	assert.True(t, strings.Contains(msg, "DENY"), "verdict string must say DENY: %s", msg)
}