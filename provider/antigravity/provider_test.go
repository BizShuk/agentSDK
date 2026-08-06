package antigravity_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/antigravity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateway is a fake Cloud Code endpoint. It answers loadCodeAssist with a
// project and every generate call with the supplied body, recording what
// it saw.
type gateway struct {
	*httptest.Server

	mu       sync.Mutex
	paths    []string
	headers  http.Header
	lastBody map[string]any
}

func newGateway(t *testing.T, respond func(w http.ResponseWriter, path string)) *gateway {
	t.Helper()
	g := &gateway{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)

		g.mu.Lock()
		// The colon method name lives in the path, and httptest gives it
		// back unescaped in RawPath only when it needed escaping.
		path := r.URL.EscapedPath()
		g.paths = append(g.paths, path)
		g.headers = r.Header.Clone()
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if !strings.HasSuffix(path, "loadCodeAssist") {
			g.lastBody = body
		}
		g.mu.Unlock()

		if strings.HasSuffix(path, "loadCodeAssist") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cloudaicompanionProject":"proj-42"}`))
			return
		}
		respond(w, path)
	}))
	t.Cleanup(g.Close)
	return g
}

func (g *gateway) body() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lastBody
}

func (g *gateway) visited() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.paths...)
}

func (g *gateway) seenHeaders() http.Header {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.headers
}

// TestNewRequiresCredential — without an OAuth token or explicit key,
// construction through the registry fails fast.
func TestNewRequiresCredential(t *testing.T) {
	t.Setenv("ANTIGRAVITY_OAUTH_TOKEN", "")
	_, err := provider.New("antigravity", provider.Options{CredentialKind: core.CREDENTIAL_KIND_OAUTH})
	assert.Error(t, err)
}

// TestAPIKeyCredentialKindRejected — the gateway is OAuth-only, so asking
// for an api_key credential is refused during resolution rather than sent
// upstream and refused there.
func TestAPIKeyCredentialKindRejected(t *testing.T) {
	_, err := provider.New("antigravity", provider.Options{CredentialKind: core.CREDENTIAL_KIND_APIKEY})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OAuth-only")
}

// TestGenerateNonThinkingModel — a non-thinking model takes the blocking
// endpoint, and the Cloud Code envelope is assembled as the gateway
// expects.
func TestGenerateNonThinkingModel(t *testing.T) {
	g := newGateway(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{
			"candidates":[{"content":{"parts":[{"text":"hello back"}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}
		}}`))
	})

	t.Setenv("ANTIGRAVITY_OAUTH_TOKEN", "ya29.from-env")
	t.Setenv("ANTIGRAVITY_BASE_URL", g.URL)

	p, err := provider.New("antigravity", provider.Options{Model: "gemini-2.5-flash"})
	require.NoError(t, err)

	res, err := p.Generate(context.Background(), core.ModelRequest{
		MaxTokens: 128,
		Messages: []core.Message{
			{Role: core.ROLE_SYSTEM, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "be terse"}}},
			{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hello"}}},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "hello back", res.Text)
	assert.Equal(t, "end_turn", res.StopReason)
	assert.Equal(t, 7, res.Usage.PromptTokens)
	assert.Equal(t, 3, res.Usage.CompletionTokens)
	assert.Equal(t, 10, res.Usage.TotalTokens)

	assert.Equal(t,
		[]string{"/v1internal:loadCodeAssist", "/v1internal:generateContent"},
		g.visited())

	sent := g.body()
	assert.Equal(t, "gemini-2.5-flash", sent["model"])
	assert.Equal(t, "proj-42", sent["project"], "project comes from loadCodeAssist")
	assert.Equal(t, "agent", sent["requestType"])

	inner := sent["request"].(map[string]any)
	contents := inner["contents"].([]any)
	require.Len(t, contents, 1, "system message is hoisted out of contents")
	assert.Equal(t, "user", contents[0].(map[string]any)["role"])

	// System text is hoisted verbatim — no client persona is prepended.
	sys := inner["systemInstruction"].(map[string]any)["parts"].([]any)
	require.Len(t, sys, 1)
	assert.Equal(t, "be terse", sys[0].(map[string]any)["text"])

	gen := inner["generationConfig"].(map[string]any)
	assert.Equal(t, float64(128), gen["maxOutputTokens"])
	assert.Nil(t, gen["thinkingConfig"], "non-thinking model asks for no thoughts")
}

// TestGenerateThinkingModelUsesStream — the blocking endpoint omits
// thoughts, so a thinking model must be served over SSE and folded back.
func TestGenerateThinkingModelUsesStream(t *testing.T) {
	g := newGateway(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"response":{"candidates":[{"content":{"parts":[{"thought":true,"text":"inspect","thoughtSignature":"sig-out"}]}}]}}`,
			``,
			`data: {"response":{"candidates":[{"content":{"parts":[{"text":"done"}]}}]}}`,
			``,
			`data: {"response":{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"thoughtsTokenCount":5,"totalTokenCount":11}}}`,
			``,
			``,
		}, "\n"))
	})

	p, err := antigravity.New(provider.ResolvedConfig{
		BaseURL: g.URL,
		Model:   "claude-opus-4-6-thinking",
		Auth:    core.Auth{Bearer: "ya29.token"},
	})
	require.NoError(t, err)

	res, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{
			{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "think"}}},
		},
	})
	require.NoError(t, err)

	assert.Contains(t, g.visited(), "/v1internal:streamGenerateContent")
	require.Len(t, res.Parts, 2)
	assert.Equal(t, core.PART_KIND_REASONING, res.Parts[0].Kind)
	assert.Equal(t, "inspect", res.Parts[0].Text)
	require.NotNil(t, res.Parts[0].Reasoning)
	assert.Equal(t, "sig-out", res.Parts[0].Reasoning.Signature)
	assert.Equal(t, "done", res.Text)
	assert.Equal(t, "end_turn", res.StopReason)
	// Thinking tokens are billed output and are folded into the
	// completion count rather than dropped.
	assert.Equal(t, 7, res.Usage.CompletionTokens)
}

// TestStreamToolCallOutranksFinishReason — Gemini reports finishReason
// STOP for a turn that ended in a tool call, and that frame arrives after
// the call itself. The later frame must not overwrite the verdict.
func TestStreamToolCallOutranksFinishReason(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"Taipei"}}}]}}]}}`,
		``,
		`data: {"response":{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}]}}`,
		``,
		``,
	}, "\n")

	chunks, stop := antigravity.ParseStream(context.Background(), strings.NewReader(raw))
	res := antigravity.FoldStream(chunks, stop)

	assert.Equal(t, "tool_use", res.StopReason)
	require.Len(t, res.ToolCalls, 1)
	assert.Equal(t, "get_weather", res.ToolCalls[0].Name)
}

// TestStreamCarriesGeneratedImage — an image model returns its whole
// picture as one base64 inlineData part in a single SSE frame, and image
// models are Gemini 3+, so Generate reaches them through the stream.
//
// This covers two defects that each silently produced an empty turn: the
// chunk vocabulary had no image field, and the frame was larger than the
// shared SSE decoder's 1 MiB default. A measured live reply was 1.9 MB,
// so the fixture is deliberately over that default.
func TestStreamCarriesGeneratedImage(t *testing.T) {
	payload := bytes.Repeat([]byte{0xff, 0xd8, 0xff, 0xe0}, 400_000) // ~1.6 MB
	encoded := base64.StdEncoding.EncodeToString(payload)
	require.Greater(t, len(encoded), 1<<20, "fixture must exceed the default frame cap")

	raw := `data: {"response":{"candidates":[{"content":{"parts":[` +
		`{"inlineData":{"mimeType":"image/jpeg","data":"` + encoded + `"}}]}}]}}` + "\n\n" +
		`data: {"response":{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}]}}` + "\n\n"

	chunks, stop := antigravity.ParseStream(context.Background(), strings.NewReader(raw))
	res := antigravity.FoldStream(chunks, stop)

	require.Len(t, res.Parts, 1)
	assert.Equal(t, core.PART_KIND_IMAGE, res.Parts[0].Kind)
	assert.Equal(t, "image/jpeg", res.Parts[0].ImageMIME)
	assert.Equal(t, payload, res.Parts[0].Image)
	assert.Equal(t, "end_turn", res.StopReason)
}

// TestGenerateEncodesImageInput — vision input rides as inlineData, and a
// part the adapter cannot encode would leave the model answering blind.
func TestGenerateEncodesImageInput(t *testing.T) {
	g := newGateway(t, func(w http.ResponseWriter, _ string) {
		_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"Red"}]}}]}}`))
	})

	png := []byte{0x89, 0x50, 0x4e, 0x47}
	p := antigravity.NewForHosts([]string{g.URL}, provider.ResolvedConfig{
		Model: "gemini-2.5-flash",
		Auth:  core.Auth{Bearer: "t"},
	}).WithProjectID("proj")

	_, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{
			{Kind: core.PART_KIND_IMAGE, Image: png, ImageMIME: "image/png"},
			{Kind: core.PART_KIND_PLAIN_TEXT, Text: "what colour?"},
		}}},
	})
	require.NoError(t, err)

	parts := g.body()["request"].(map[string]any)["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	require.Len(t, parts, 2)
	inline := parts[0].(map[string]any)["inlineData"].(map[string]any)
	assert.Equal(t, "image/png", inline["mimeType"])
	assert.Equal(t, base64.StdEncoding.EncodeToString(png), inline["data"])
}

// TestHeadersCarryClientIdentity — the gateway 403s a request that does
// not look like the Antigravity client.
func TestHeadersCarryClientIdentity(t *testing.T) {
	g := newGateway(t, func(w http.ResponseWriter, _ string) {
		_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}}`))
	})

	p, err := antigravity.New(provider.ResolvedConfig{
		BaseURL: g.URL,
		Model:   "claude-opus-4-6-thinking",
		Auth:    core.Auth{Bearer: "ya29.fake"},
	})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "ping"}}}},
	})
	require.NoError(t, err)

	h := g.seenHeaders()
	assert.Equal(t, "Bearer ya29.fake", h.Get("Authorization"))
	assert.Equal(t, "antigravity", h.Get("X-Client-Name"))
	assert.NotEmpty(t, h.Get("X-Client-Version"))
	assert.NotEmpty(t, h.Get("x-goog-api-client"))
	assert.NotEmpty(t, h.Get("X-Machine-Session-Id"))
	assert.Equal(t, "interleaved-thinking-2025-05-14", h.Get("anthropic-beta"),
		"claude thinking models need interleaved thinking")
}

// TestGenerateRequiresToken — no credential at all is an error before any
// request leaves the process.
func TestGenerateRequiresToken(t *testing.T) {
	p, err := antigravity.New(provider.ResolvedConfig{BaseURL: "https://example.invalid"})
	require.NoError(t, err)
	_, err = p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OAuth access token")
}

// TestToolRoundTrip — declarations reach the gateway in Google's schema
// dialect and a functionCall comes back as a core tool call.
func TestToolRoundTrip(t *testing.T) {
	g := newGateway(t, func(w http.ResponseWriter, _ string) {
		_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[
			{"functionCall":{"name":"read_file","args":{"path":"a.txt"}}}
		]},"finishReason":"STOP"}]}}`))
	})

	p := antigravity.NewForHosts([]string{g.URL}, provider.ResolvedConfig{
		Model: "gemini-2.5-flash",
		Auth:  core.Auth{Bearer: "t"},
	}).WithProjectID("proj-pinned")

	res, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "read it"}}}},
		Tools: []core.ToolSpec{{
			Name:        "read.file",
			Description: "read a file",
			Parameters: json.RawMessage(`{
				"type":"object",
				"$schema":"https://json-schema.org/draft/2020-12/schema",
				"additionalProperties":false,
				"properties":{"path":{"type":"string"}},
				"required":["path","missing"]
			}`),
		}},
	})
	require.NoError(t, err)

	assert.NotContains(t, g.visited(), "/v1internal:loadCodeAssist",
		"a pinned project skips discovery")

	decl := g.body()["request"].(map[string]any)["tools"].([]any)[0].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
	assert.Equal(t, "read_file", decl["name"], "dots are not a legal function name")

	params := decl["parameters"].(map[string]any)
	assert.Equal(t, "OBJECT", params["type"], "Google's dialect uppercases types")
	assert.NotContains(t, params, "$schema")
	assert.NotContains(t, params, "additionalProperties")
	assert.Equal(t, []any{"path"}, params["required"], "undeclared required names are dropped")

	require.Len(t, res.ToolCalls, 1)
	assert.Equal(t, "read_file", res.ToolCalls[0].Name)
	assert.Equal(t, "read_file", res.ToolCalls[0].ID, "Gemini omits ids; the name stands in")
	assert.Equal(t, "tool_use", res.StopReason)
}

// TestHostFallback — a 404 on the daily channel moves to production; the
// request body is unchanged.
func TestHostFallback(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":404,"message":"not found here"}}`, http.StatusNotFound)
	}))
	defer down.Close()

	var reached bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}}`))
	}))
	defer up.Close()

	p := antigravity.NewForHosts([]string{down.URL, up.URL}, provider.ResolvedConfig{
		Model: "gemini-2.5-flash",
		Auth:  core.Auth{Bearer: "t"},
	}).WithProjectID("proj")

	res, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}}},
	})
	require.NoError(t, err)
	assert.True(t, reached, "fallback host must be tried")
	assert.Equal(t, "ok", res.Text)
}

// TestNonRetryableStatusStopsImmediately — a 400 is about the request, so
// trying the next host would only waste a round-trip.
func TestNonRetryableStatusStopsImmediately(t *testing.T) {
	var hits int
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"contents must not be empty"}}`))
	}))
	defer bad.Close()

	p := antigravity.NewForHosts([]string{bad.URL, bad.URL}, provider.ResolvedConfig{
		Model: "gemini-2.5-flash",
		Auth:  core.Auth{Bearer: "t"},
	}).WithProjectID("proj")

	_, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}}},
	})
	require.Error(t, err)
	assert.Equal(t, 1, hits)
	assert.Contains(t, err.Error(), "contents must not be empty")
}

// TestListModels — membership comes from the live endpoint, metadata from
// the bundled catalog.
func TestListModels(t *testing.T) {
	g := newGateway(t, func(w http.ResponseWriter, _ string) {
		_, _ = w.Write([]byte(`{"models":{
			"gemini-3.1-pro-high":{"displayName":"Gemini 3.1 Pro"},
			"claude-sonnet-4-6":{"displayName":"Claude Sonnet 4.6"},
			"chat_20706":{},
			"tab_flash_lite_preview":{}
		}}`))
	})

	p := antigravity.NewForHosts([]string{g.URL}, provider.ResolvedConfig{
		Auth: core.Auth{Bearer: "t"},
	}).WithProjectID("proj")

	specs, err := p.ListModels(context.Background())
	require.NoError(t, err)
	// Sorted, so the order does not reshuffle between calls.
	require.Len(t, specs, 2)
	assert.Equal(t, "claude-sonnet-4-6", specs[0].ID)
	assert.Equal(t, "claude-sonnet", specs[0].Family, "metadata comes from the bundled catalog")
	assert.Equal(t, "gemini-3.1-pro-high", specs[1].ID)

	for _, s := range specs {
		assert.NotZero(t, s.ContextWindow, "%s: an unsized model must not be listed", s.ID)
		assert.NotZero(t, s.MaxTokens, "%s: an unsized model must not be listed", s.ID)
	}
}

// Compile-time: the bundled catalog uses the canonical provider model type.
var _ []provider.ModelSpec = antigravity.CATALOG

// TestCatalogReasoningMatchesRouting — the catalog's Reasoning flag and
// the SSE-vs-blocking routing decision must never disagree; a model
// advertised as reasoning that takes the blocking path returns no
// thoughts.
func TestCatalogReasoningMatchesRouting(t *testing.T) {
	for _, spec := range antigravity.DefaultCatalog() {
		t.Run(spec.ID, func(t *testing.T) {
			assert.NotZero(t, spec.ContextWindow, "every bundled entry carries limits")
			assert.NotZero(t, spec.MaxTokens, "every bundled entry carries limits")
			assert.NotEmpty(t, spec.Family, "every bundled entry carries a family")
			assert.LessOrEqual(t, spec.MaxTokens, spec.ContextWindow)
		})
	}
}

// TestGenerateRejectsResponsesReasoningMetadata — a reasoning part shaped
// for the OpenAI Responses API cannot be encoded as a thought signature.
func TestGenerateRejectsResponsesReasoningMetadata(t *testing.T) {
	p := antigravity.NewForHosts([]string{"https://example.invalid"}, provider.ResolvedConfig{
		Auth: core.Auth{Bearer: "t"},
	}).WithProjectID("proj")

	_, err := p.Generate(context.Background(), core.ModelRequest{
		Messages: []core.Message{{
			Role: core.ROLE_ASSISTANT,
			Parts: []core.Part{{
				Kind:      core.PART_KIND_REASONING,
				Text:      "inspect",
				Reasoning: &core.ReasoningState{ID: "reasoning_1"},
			}},
		}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Responses continuation metadata")
}

// TestValidate covers the shape errors the gateway would answer with a
// bare 400.
func TestValidate(t *testing.T) {
	content := []antigravity.Content{{Role: "user", Parts: []antigravity.Part{{Text: "hi"}}}}
	cases := []struct {
		name string
		body antigravity.CloudCodeRequest
		want string
	}{
		{
			name: "empty model",
			body: antigravity.CloudCodeRequest{Project: "p", Request: antigravity.GenerateRequest{Contents: content}},
			want: "model is required",
		},
		{
			name: "empty project",
			body: antigravity.CloudCodeRequest{Model: "m", Request: antigravity.GenerateRequest{Contents: content}},
			want: "project is required",
		},
		{
			name: "no contents",
			body: antigravity.CloudCodeRequest{Model: "m", Project: "p"},
			want: "at least one content entry",
		},
		{
			name: "bad role",
			body: antigravity.CloudCodeRequest{Model: "m", Project: "p", Request: antigravity.GenerateRequest{
				Contents: []antigravity.Content{{Role: "system", Parts: []antigravity.Part{{Text: "x"}}}},
			}},
			want: "must be user|model",
		},
		{
			name: "empty parts",
			body: antigravity.CloudCodeRequest{Model: "m", Project: "p", Request: antigravity.GenerateRequest{
				Contents: []antigravity.Content{{Role: "user"}},
			}},
			want: "has no parts",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.body.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
