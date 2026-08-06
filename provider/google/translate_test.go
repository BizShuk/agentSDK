package google

import (
	"context"
	"net/http"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// translateHandler fakes one translation turn: assert the setup carries the
// translationConfig, then stream the translated text in two chunks.
func translateHandler(t *testing.T, ctx context.Context, c *websocket.Conn, r *http.Request) {
	setup := readClientMessage(t, ctx, c)
	if !assert.NotNil(t, setup.Setup) {
		return
	}
	assert.Equal(t, "models/"+DefaultTranslateModel, setup.Setup.Model)
	assert.Equal(t, []string{"TEXT"}, setup.Setup.GenerationConfig.ResponseModalities)
	assert.Equal(t, "es",
		setup.Setup.GenerationConfig.TranslationConfig.TargetLanguageCode)
	writeServerMessage(t, ctx, c, liveServerMessage{SetupComplete: &struct{}{}})

	turn := readClientMessage(t, ctx, c)
	if !assert.NotNil(t, turn.ClientContent) {
		return
	}
	assert.Equal(t, "hello world", turn.ClientContent.Turns[0].Parts[0].Text)

	writeServerMessage(t, ctx, c, liveServerMessage{
		ServerContent: &liveServerContent{
			ModelTurn: &liveContent{Parts: []livePart{{Text: "hola "}}},
		},
	})
	writeServerMessage(t, ctx, c, liveServerMessage{
		ServerContent: &liveServerContent{
			ModelTurn:    &liveContent{Parts: []livePart{{Text: "mundo"}}},
			TurnComplete: true,
		},
	})
	_, _, _ = c.Read(ctx) // wait for the client close handshake
}

func TestTranslateFoldsOneTurn(t *testing.T) {
	server := liveTestServer(t, translateHandler)

	translator, err := NewTranslate(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "secret-key"},
	})
	require.NoError(t, err)

	result, err := translator.Translate(context.Background(), provider.TranslateRequest{
		Text:           "hello world",
		TargetLanguage: "es",
	})
	require.NoError(t, err)
	assert.Equal(t, "hola mundo", result.Text)
}

func TestStreamTranslationYieldsChunks(t *testing.T) {
	server := liveTestServer(t, translateHandler)

	translator, err := NewTranslate(provider.ResolvedConfig{
		BaseURL: server.URL,
		Auth:    core.Auth{APIKey: "secret-key"},
	})
	require.NoError(t, err)

	chunks, err := translator.StreamTranslation(context.Background(), provider.TranslateRequest{
		Text:           "hello world",
		TargetLanguage: "es",
	})
	require.NoError(t, err)

	var got []string
	for chunk := range chunks {
		require.NoError(t, chunk.Err)
		got = append(got, chunk.Text)
	}
	assert.Equal(t, []string{"hola ", "mundo"}, got)
}

func TestTranslateValidatesRequest(t *testing.T) {
	translator, err := NewTranslate(provider.ResolvedConfig{
		Auth: core.Auth{APIKey: "secret-key"},
	})
	require.NoError(t, err)

	_, err = translator.Translate(context.Background(), provider.TranslateRequest{
		Text: "hello",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target language is required")
}
