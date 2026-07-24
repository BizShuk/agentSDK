// Package utils holds root-module shared plumbing that does not belong
// to any specific domain package. Today it carries two surfaces:
//
//   - frontmatter.Parse / frontmatter.List: the minimal "---" key/value
//     header shared by SKILL.md, slash commands, and subagent
//     definitions. Intentionally not YAML — flat "key: value" lines only,
//     which is all the markdown-definition formats of claude-code / codex
//     / pi require.
//   - ScriptedProvider / MemStore / RecordingNotifier: in-process fakes
//     for core.Provider, core.StateStore and core.Notifier, consumed by
//     every test file in the repo. They live here (rather than under
//     internal/) so sample/ modules outside the root module can import
//     them too.
package utils

import (
	"strings"
)

const DELIMITER = "---"

// Parse splits content into frontmatter fields and the remaining body.
// Content without a leading delimiter returns empty fields and the full
// content as body. Keys are lower-cased; unknown lines inside the header
// are ignored.
func Parse(content string) (fields map[string]string, body string) {
	fields = map[string]string{}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != DELIMITER {
		return fields, content
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == DELIMITER {
			end = i
			break
		}
	}
	if end < 0 {
		return fields, content
	}
	for _, line := range lines[1:end] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		fields[key] = strings.TrimSpace(value)
	}
	return fields, strings.TrimPrefix(strings.Join(lines[end+1:], "\n"), "\n")
}

// List splits a frontmatter value into items: "[a, b]" or "a, b" → [a b].
func List(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}