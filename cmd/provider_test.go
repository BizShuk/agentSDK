package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFakeMessagesServer stands up an httptest server that mimics the
// Anthropic-Messages-compatible wire format used by minimax. It inspects
// the inbound POST body to verify routing and auth (minimax uses
// x-api-key, not Authorization), then returns a canned Response.
func newFakeMessagesServer(t *testing.T, expectModel string) (*httptest.Server, *string) {
	t.Helper()
	sawKey := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKey = r.Header.Get("x-api-key")
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Model != expectModel {
			http.Error(w, fmt.Sprintf("model=%q want=%q", req.Model, expectModel),
				http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id": "msg_test",
		  "type": "message",
		  "role": "assistant",
		  "model": "` + expectModel + `",
		  "stop_reason": "end_turn",
		  "content": [{"type": "text", "text": "pong from fake"}],
		  "usage": {"input_tokens": 11, "output_tokens": 7}
		}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &sawKey
}

// runCLI drives NewProviderCommand with the given args and returns the
// captured stdout / stderr + error from Execute().
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := NewProviderCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

// TestProviderGenerateRoundTrip points --base-url at a fake server and
// asserts the CLI emits the canned assistant text + token usage footer.
func TestProviderGenerateRoundTrip(t *testing.T) {
	srv, sawKey := newFakeMessagesServer(t, "minimax-M2")
	stdout, stderr, err := runCLI(t,
		"--provider", "minimax",
		"--model", "minimax-M2",
		"--base-url", srv.URL,
		"--api-key", "sk-test",
		"ping",
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "pong from fake")
	assert.Contains(t, stdout, "[stop=end_turn")
	assert.Contains(t, stdout, "tokens=11/7")
	assert.Contains(t, stderr, "[provider] minimax")
	assert.Equal(t, "sk-test", *sawKey,
		"x-api-key header must carry the API key verbatim")
}

// TestProviderListModels dumps the static catalog without making any HTTP
// call — the catalog is in-memory, so the fake server is irrelevant.
func TestProviderListModels(t *testing.T) {
	stdout, _, err := runCLI(t,
		"--provider", "minimax",
		"--list-models",
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "minimax catalog")
	assert.Contains(t, stdout, "minimax-M2")
}

// TestProviderListProviders prints the registered adapter names.
func TestProviderListProviders(t *testing.T) {
	stdout, _, err := runCLI(t, "--list-providers")
	require.NoError(t, err)
	assert.Contains(t, stdout, "minimax")
	assert.Contains(t, stdout, "anthropic")
}

// TestProviderRejectsUnknownProvider returns a clean error for an
// unregistered adapter name.
func TestProviderRejectsUnknownProvider(t *testing.T) {
	_, _, err := runCLI(t,
		"--provider", "nonsense",
		"--api-key", "sk-test",
		"ping",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
	assert.Contains(t, err.Error(), "minimax")
	assert.Contains(t, err.Error(), "anthropic")
}

// TestProviderRequiresPrompt confirms the CLI errors when no positional
// prompt is given and no --list-* flag is set.
func TestProviderRequiresPrompt(t *testing.T) {
	_, _, err := runCLI(t,
		"--provider", "minimax",
		"--api-key", "sk-test",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt is required")
}

// TestProviderJSON emits the full ModelResult as a single JSON line.
func TestProviderJSON(t *testing.T) {
	srv, _ := newFakeMessagesServer(t, "minimax-M2")
	stdout, _, err := runCLI(t,
		"--provider", "minimax",
		"--model", "minimax-M2",
		"--base-url", srv.URL,
		"--api-key", "sk-test",
		"--json",
		"ping",
	)
	require.NoError(t, err)
	// One JSON line, parseable.
	line := strings.TrimSpace(stdout)
	require.NotEmpty(t, line)
	var got struct {
		Text       string `json:"text"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, json.Unmarshal([]byte(line), &got))
	assert.Equal(t, "pong from fake", got.Text)
	assert.Equal(t, "end_turn", got.StopReason)
	assert.Equal(t, 11, got.Usage.PromptTokens)
	assert.Equal(t, 7, got.Usage.CompletionTokens)
}