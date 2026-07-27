package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeLogMasksSecretsAndRepairsText(t *testing.T) {
	raw := []byte(
		"Authorization: Bearer abcdefghijklmnop\n" +
			"password=hunter2\n" +
			"api_key=sk-1234567890abcdefghijklmnop\n" +
			"message=ok\x00\xff\n",
	)

	got := sanitizeLog(raw)
	for _, secret := range []string{
		"abcdefghijklmnop",
		"hunter2",
		"sk-1234567890abcdefghijklmnop",
	} {
		assert.NotContains(t, got, secret)
	}
	assert.GreaterOrEqual(t, strings.Count(got, "[REDACTED]"), 3)
	assert.NotContains(t, got, "\x00")
	assert.Contains(t, got, "\uFFFD")
}
