package core

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	authorizationPattern = regexp.MustCompile(
		`(?im)(["']?authorization["']?\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|(?:bearer|basic)\s+[^\s,;}]+|[^\s,;}]+)`,
	)
	bearerPattern = regexp.MustCompile(
		`(?i)\bbearer\s+[a-z0-9._~+/=-]{8,}`,
	)
	secretValuePattern = regexp.MustCompile(
		`(?im)(["']?(?:password|passwd|secret|token|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret)["']?\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;}]+)`,
	)
	commonAPIKeyPattern = regexp.MustCompile(
		`\bsk-[A-Za-z0-9_-]{16,}\b`,
	)
)

// SanitizeLog repairs text and masks common credential shapes before LLM use.
func SanitizeLog(raw []byte) string {
	text := strings.ToValidUTF8(string(raw), "\uFFFD")
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, text)

	text = authorizationPattern.ReplaceAllString(text, `${1}[REDACTED]`)
	text = bearerPattern.ReplaceAllString(text, "Bearer [REDACTED]")
	text = secretValuePattern.ReplaceAllString(text, `${1}[REDACTED]`)
	return commonAPIKeyPattern.ReplaceAllString(text, "[REDACTED]")
}
