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

	"github.com/bizshuk/agentsdk/provider"
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
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"MiniMax-M2"},{"id":"MiniMax-M3"}]}`))
			return
		}
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

// runCLI drives ProviderCmd with the given args and returns the
// captured stdout / stderr + error from Execute().
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	ResetFlags()
	root := ProviderCmd
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

// TestProviderListModels drives --list-models against a fake /v1/models
// endpoint and asserts the CLI prints the LIVE catalog (source=live) with
// the ids the upstream reported, not the compiled-in static list.
func TestProviderListModels(t *testing.T) {
	srv, _ := newFakeMessagesServer(t, "unused")
	stdout, stderr, err := runCLI(t,
		"--provider", "minimax",
		"--base-url", srv.URL,
		"--api-key", "sk-test",
		"--list-models",
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "minimax catalog")
	assert.Contains(t, stdout, "live", "should report the live source, not static")
	assert.Contains(t, stdout, "MiniMax-M2")
	assert.Contains(t, stdout, "MiniMax-M3")
	assert.NotContains(t, stderr, "live catalog unavailable")
}

// TestProviderListModelsFallback proves the CLI degrades to the static
// catalog when the live call fails (here: an unroutable base URL), and
// reports the fallback on stderr while keeping stdout a usable list.
func TestProviderListModelsFallback(t *testing.T) {
	stdout, stderr, err := runCLI(t,
		"--provider", "minimax",
		"--base-url", "http://127.0.0.1:1",
		"--api-key", "sk-test",
		"--list-models",
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "minimax catalog")
	assert.Contains(t, stdout, "static")
	assert.Contains(t, stderr, "live catalog unavailable")
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

// TestProviderCredentialsFromEnv verifies that when --api-key is omitted,
// credentials flow from viper (bound to OS env by bootGosdkConfig) into
// the request's x-api-key header. Mirrors the t.Setenv patterns in
// provider/<name>/provider_test.go.
func TestProviderCredentialsFromEnv(t *testing.T) {
	// Clear any leftover key from the test environment first so the
	// resolution path is deterministic across local dev shells.
	t.Setenv("MINIMAX_API_KEY", "sk-from-env")

	srv, sawKey := newFakeMessagesServer(t, "minimax-M2")
	stdout, _, err := runCLI(t,
		"--provider", "minimax",
		"--model", "minimax-M2",
		"--base-url", srv.URL,
		// no --api-key; expect OS-env resolution
		"ping",
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "pong from fake")
	assert.Equal(t, "sk-from-env", *sawKey)
}

// TestProviderCredentialKindFlagDefaultsToAuto checks that the new
// --credential-kind flag accepts its documented values and that the
// default is "auto" (legacy precedence preserved).
func TestProviderCredentialKindFlagDefaultsToAuto(t *testing.T) {
	flag := ProviderCmd.Flags().Lookup("credential-kind")
	require.NotNil(t, flag)
	assert.Equal(t, "auto", flag.DefValue,
		"the default must remain 'auto' so existing shell scripts are not broken by the new flag")
}

// TestProviderFlagDefaultReferencesProviderDefaultName guards the
// third "minimax" hardcoding site: --provider's default value and
// ResetFlags both flow from provider.DEFAULT_NAME. If a future refactor
// reintroduces a string literal here, the test fails before the drift
// reaches a release.
func TestProviderFlagDefaultReferencesProviderDefaultName(t *testing.T) {
	flag := ProviderCmd.Flags().Lookup("provider")
	require.NotNil(t, flag)
	assert.Equal(t, provider.DEFAULT_NAME, flag.DefValue,
		"--provider default must come from provider.DEFAULT_NAME, not a hardcoded literal")
}

// TestProviderCredentialKindStrictRejectsUnsupportedProvider locks in
// the failure mode: strict oauth against a provider that has no OAuth
// env (minimax) must error at startup, not silently fall through.
//
// No --api-key is supplied so the strict path actually runs — when an
// explicit credential is given, Options.Resolve never inspects
// CredentialKind at all.
func TestProviderCredentialKindStrictRejectsUnsupportedProvider(t *testing.T) {
	t.Setenv("MINIMAX_API_KEY", "")
	_, _, err := runCLI(t,
		"--provider", "minimax",
		"--credential-kind", "oauth",
		"ping",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not OAuth-capable")
}

// TestProviderCredentialKindAutoAcceptsExplicitAPIKey verifies that
// when --api-key is supplied, the strict-mode flag value is ignored —
// an explicit credential wins regardless of credential_kind. This keeps
// the flag orthogonal to --api-key instead of requiring operators to
// unset both.
func TestProviderCredentialKindAutoAcceptsExplicitAPIKey(t *testing.T) {
	srv, sawKey := newFakeMessagesServer(t, "minimax-M2")
	_, _, err := runCLI(t,
		"--provider", "minimax",
		"--model", "minimax-M2",
		"--base-url", srv.URL,
		"--api-key", "sk-explicit",
		"--credential-kind", "api_key",
		"ping",
	)
	require.NoError(t, err)
	assert.Equal(t, "sk-explicit", *sawKey,
		"--api-key must outrank the strict-mode env lookup so existing scripts keep working")
}
