package core_test

import (
	"strings"
	"testing"

	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeLogRedactsCommonSecrets(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"Authorization: Bearer auth-secret",
		"password=hunter2",
		`api_key="key-secret"`,
		"token: token-secret",
		`{"access_token":"json-secret","client_secret":"client-secret"}`,
		"standalone Bearer abcdefghijklmnop",
		"openai=sk-abcdefghijklmnop123456",
	}, "\n"))

	got := domain.SanitizeLog(raw)

	for _, secret := range []string{
		"auth-secret",
		"hunter2",
		"key-secret",
		"token-secret",
		"json-secret",
		"client-secret",
		"abcdefghijklmnop",
		"sk-abcdefghijklmnop123456",
	} {
		assert.NotContains(t, got, secret)
	}
	assert.Contains(t, got, "Authorization: [REDACTED]")
	assert.Contains(t, got, "password=[REDACTED]")
	assert.Contains(t, got, "api_key=[REDACTED]")
	assert.Contains(t, got, `"access_token":[REDACTED]`)
	assert.Contains(t, got, "Bearer [REDACTED]")
}

func TestSanitizeLogRepairsUTF8AndRemovesControls(t *testing.T) {
	raw := []byte{'o', 'k', '\t', 0x00, 0xff, '\n', 'x'}
	got := domain.SanitizeLog(raw)

	assert.Equal(t, "ok\t�\nx", got)
}
