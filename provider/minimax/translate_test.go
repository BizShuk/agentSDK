package minimax_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/minimax"
)

func TestTranslateUsesChatWithTargetLanguage(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"m1","type":"message","role":"assistant","model":"MiniMax-M3",
			"content":[{"type":"thinking","thinking":"..."},{"type":"text","text":" Hello, world. "}],
			"stop_reason":"end_turn","usage":{"input_tokens":12,"output_tokens":4}}`)
	}))
	t.Cleanup(server.Close)

	translator, err := minimax.NewTranslate(speechConfig(server.URL))
	require.NoError(t, err)
	result, err := translator.Translate(context.Background(), provider.TranslateRequest{
		Text: "你好, 世界.", TargetLanguage: "English",
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello, world.", result.Text, "thinking blocks are dropped and whitespace trimmed")
	assert.Equal(t, minimax.DefaultTranslateModel, body["model"])
	assert.Contains(t, body["system"], "into English")
}

func TestTranslateSurfacesTypedChatError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	}))
	t.Cleanup(server.Close)

	translator, err := minimax.NewTranslate(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = translator.Translate(context.Background(), provider.TranslateRequest{Text: "hi", TargetLanguage: "ja"})
	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, "slow down", apiErr.Message)
}

func TestStreamTranslationEmitsTextDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if body["stream"] != true {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"type":"message","content":[{"type":"text","text":"Hello"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"m","model":"MiniMax-M3","usage":{"input_tokens":3}}}`,
			`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`,
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
			`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop","index":0}`,
			`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
			`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
		}
		_, _ = io.WriteString(w, strings.Join(events, "\n\n")+"\n\n")
	}))
	t.Cleanup(server.Close)

	translator, err := minimax.NewTranslate(speechConfig(server.URL))
	require.NoError(t, err)
	chunks, err := translator.StreamTranslation(context.Background(), provider.TranslateRequest{Text: "你好", TargetLanguage: "en"})
	require.NoError(t, err)
	var text strings.Builder
	for chunk := range chunks {
		require.NoError(t, chunk.Err)
		text.WriteString(chunk.Text)
	}
	assert.Equal(t, "Hello", text.String())
}
