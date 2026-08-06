package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAdapter is a chat-only in-process provider so the run→iterate→store
// flow is testable without credentials or network.
type fakeAdapter struct{}

func (f *fakeAdapter) Generate(_ context.Context, req core.ModelRequest) (core.ModelResult, error) {
	last := req.Messages[len(req.Messages)-1]
	return core.ModelResult{Text: "pong: " + last.Parts[0].Text, StopReason: "end"}, nil
}

func (f *fakeAdapter) Stream(_ context.Context, _ core.ModelRequest) (<-chan core.ModelChunk, error) {
	ch := make(chan core.ModelChunk, 1)
	ch <- core.ModelChunk{Done: true}
	close(ch)
	return ch, nil
}

func init() {
	provider.Register(provider.Entry{
		Name:     "benchfake",
		Metadata: provider.Metadata{Label: "BenchFake"},
		New: func(provider.ResolvedConfig) (provider.Adapter, error) {
			return &fakeAdapter{}, nil
		},
	})
}

func TestRunStoresResultsAndSkipsFailures(t *testing.T) {
	pkgDir := t.TempDir()
	cases := []Case{
		{Name: "hello", Capability: provider.CAPABILITY_CHAT, Prompt: "hi"},
		// benchfake has no image factory: the case must fail and be skipped.
		{Name: "t2i", Capability: provider.CAPABILITY_IMAGE, Prompt: "draw"},
		{Name: "again", Capability: provider.CAPABILITY_CHAT, Prompt: "yo"},
	}

	err := Run(context.Background(), Target{Provider: "benchfake", Model: "m"}, cases, pkgDir)
	require.NoError(t, err)

	sessions, err := os.ReadDir(filepath.Join(pkgDir, "tmp"))
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	sessionDir := filepath.Join(pkgDir, "tmp", sessions[0].Name())

	raw, err := os.ReadFile(filepath.Join(sessionDir, "summary.json"))
	require.NoError(t, err)
	var records []Record
	require.NoError(t, json.Unmarshal(raw, &records))
	require.Len(t, records, 3)
	assert.Equal(t, provider.CAPABILITY_CHAT, records[0].Capability)
	assert.Contains(t, string(raw), `"capability"`)
	assert.NotContains(t, string(raw), `"kind"`)

	assert.Equal(t, STATUS_OK, records[0].Status)
	assert.Equal(t, STATUS_FAIL, records[1].Status)
	assert.Contains(t, records[1].Error, "image")
	assert.Equal(t, STATUS_OK, records[2].Status)

	text, err := os.ReadFile(filepath.Join(sessionDir, "case-01-hello", "output.txt"))
	require.NoError(t, err)
	assert.Equal(t, "pong: hi", string(text))

	var meta Record
	metaRaw, err := os.ReadFile(filepath.Join(sessionDir, "case-01-hello", "meta.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(metaRaw, &meta))
	assert.Equal(t, []string{"output.txt"}, meta.Outputs)
	assert.Equal(t, "end", meta.Extra["stop_reason"])
}

func TestPairSlug(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		want     string
	}{
		{"anthropic", "claude-sonnet-5", "anthropic-claude-sonnet-5"},
		{"minimax", "MiniMax-M3", "minimax-m3"},
		{"grok", "grok-4", "grok-4"},
		{"google", "gemini-3-flash-preview", "google-gemini-3-flash-preview"},
		{"ollama", "qwen2.5vl:3b", "ollama-qwen2-5vl-3b"},
		{"elevenlabs", "scribe_v2", "elevenlabs-scribe-v2"},
		{"elevenlabs", "eleven_flash_v2_5", "elevenlabs-eleven-flash-v2-5"},
		{"elevenlabs", "", "elevenlabs"},
	}
	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, PairSlug(tt.provider, tt.model))
		})
	}
}

func TestRunnableCapabilitiesUseCatalogMetadataAndApplicability(t *testing.T) {
	tests := []struct {
		provider string
		id       string
		want     []provider.Capability
	}{
		{"elevenlabs", "scribe_v2", []provider.Capability{provider.CAPABILITY_TRANSCRIBE}},
		{"elevenlabs", "eleven_flash_v2_5", []provider.Capability{provider.CAPABILITY_SPEECH}},
		{"elevenlabs", "eleven_v3", []provider.Capability{provider.CAPABILITY_SPEECH}},
		{"elevenlabs", "eleven_english_sts_v2", nil},
		{"elevenlabs", "eleven_multilingual_sts_v2", nil},
		{"minimax", "MiniMax-M3", []provider.Capability{provider.CAPABILITY_CHAT}},
		{"minimax", "MiniMax-Text-01", []provider.Capability{provider.CAPABILITY_CHAT}},
		{"minimax", "image-01", []provider.Capability{provider.CAPABILITY_IMAGE}},
		{"minimax", "MiniMax-H3", []provider.Capability{provider.CAPABILITY_VIDEO}},
		{"minimax", "MiniMax-Hailuo-2.3", []provider.Capability{provider.CAPABILITY_VIDEO}},
		{"minimax", "S2V-01", nil},
		{"minimax", "music-3.0", []provider.Capability{provider.CAPABILITY_MUSIC}},
		{"minimax", "music-cover", nil},
		{"minimax", "speech-02-hd", []provider.Capability{provider.CAPABILITY_SPEECH}},
		{"google", "gemini-2.5-flash", []provider.Capability{provider.CAPABILITY_CHAT}},
		{"google", "gemini-2.5-flash-image", []provider.Capability{provider.CAPABILITY_IMAGE}},
		{"google", "nano-banana-pro-preview", []provider.Capability{provider.CAPABILITY_IMAGE}},
		{"google", "gemini-2.5-flash-preview-tts", nil},
		{"google", "lyria-3-pro-preview", nil},
		{"antigravity", "gemini-3.1-flash-image", []provider.Capability{provider.CAPABILITY_IMAGE}},
		{"antigravity", "claude-sonnet-4-6", []provider.Capability{provider.CAPABILITY_CHAT}},
		{"anthropic", "claude-sonnet-5", []provider.Capability{provider.CAPABILITY_CHAT}},
		{"ollama", "qwen2.5vl:3b", []provider.Capability{provider.CAPABILITY_CHAT}},
		{"ollama", "z-uo/qwen2.5vl_tools:7b", nil},
		{"ollama", "bge-m3:latest", nil},
	}
	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.id, func(t *testing.T) {
			entry, spec := catalogModel(t, tt.provider, tt.id)
			got := RunnableCapabilities(entry, spec)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCatalogCasesFilterUnsupportedInputModalities(t *testing.T) {
	cases := CatalogCases("codex", "gpt-5.5")
	require.Len(t, cases, 2)
	for _, testCase := range cases {
		assert.NotEqual(t, "vision-describe", testCase.Name)
		assert.Equal(t, provider.CAPABILITY_CHAT, testCase.Capability)
	}
}

func catalogModel(t *testing.T, providerName, modelID string) (provider.Entry, provider.ModelSpec) {
	t.Helper()

	entry, ok := provider.Lookup(providerName)
	require.True(t, ok)
	for _, spec := range entry.Catalog() {
		if spec.ID == modelID {
			return entry, spec
		}
	}
	require.Failf(t, "catalog model missing", "%s/%s", providerName, modelID)
	return provider.Entry{}, provider.ModelSpec{}
}

func TestWithModel(t *testing.T) {
	cases := []Case{
		{Name: "a", Capability: provider.CAPABILITY_SPEECH},
		{Name: "b", Capability: provider.CAPABILITY_SPEECH, Model: "keep-me"},
	}
	out := WithModel("eleven_v3", cases)
	assert.Equal(t, "eleven_v3", out[0].Model)
	assert.Equal(t, "keep-me", out[1].Model, "an explicit case model must win")
	assert.Empty(t, cases[0].Model, "input slice must stay untouched")
}

func TestSlug(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"text-basic", "text-basic"},
		{"Vision Describe!", "vision-describe"},
		{"  a/b  ", "a-b"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, slug(tt.in))
		})
	}
}
