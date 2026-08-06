package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/provider"
)

const (
	STATUS_OK   = "ok"
	STATUS_FAIL = "fail"

	// SESSION_TIME_FORMAT names one run's result directory by date.
	SESSION_TIME_FORMAT = "20060102-150405"
)

// Record is the persisted outcome of one case: the per-case meta.json and one
// row of the session-level summary.json.
type Record struct {
	Case       string              `json:"case"`
	Capability provider.Capability `json:"capability"`
	Provider   string              `json:"provider"`
	Model      string              `json:"model,omitempty"`
	Prompt     string              `json:"prompt,omitempty"`
	InputFile  string              `json:"input_file,omitempty"`
	StartedAt  time.Time           `json:"started_at"`
	DurationMs int64               `json:"duration_ms"`
	Status     string              `json:"status"`
	Error      string              `json:"error,omitempty"`
	Outputs    []string            `json:"outputs,omitempty"`
	Extra      map[string]string   `json:"extra,omitempty"`
}

// makeSessionDir creates tmp/<sessionID>/ under the provider-model package.
func makeSessionDir(pkgDir, sessionID string) (string, error) {
	dir := filepath.Join(pkgDir, "tmp", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}
	return dir, nil
}

// makeCaseDir creates case-NN-<name>/ under the session directory.
func makeCaseDir(sessionDir string, index int, name string) (string, error) {
	dir := filepath.Join(sessionDir, fmt.Sprintf("case-%02d-%s", index+1, slug(name)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create case dir: %w", err)
	}
	return dir, nil
}

// PairSlug names one provider-model pair's directory under benchmark/pkg/.
// The provider prefix is deduplicated so minimax + MiniMax-M3 stays
// "minimax-m3"; an empty model names the provider alone (audio-only pairs).
func PairSlug(provider, model string) string {
	p := slug(provider)
	if model == "" {
		return p
	}
	m := slug(model)
	if m == p || strings.HasPrefix(m, p+"-") {
		return m
	}
	return p + "-" + m
}

// slug keeps directory names to lowercase letters, digits, and dashes.
func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// writeJSON persists v as indented JSON.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// saveBytes writes one output file into dir and returns its file name.
func saveBytes(dir, name string, data []byte) (string, error) {
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	return name, nil
}

// saveText writes one text output file into dir and returns its file name.
func saveText(dir, name, text string) (string, error) {
	return saveBytes(dir, name, []byte(text))
}
