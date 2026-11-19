package minimax_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/minimax"
)

func TestTranscribeSendsMultipartAndFoldsSegments(t *testing.T) {
	var got struct {
		path, auth, language, model, format, filename string
		file                                          []byte
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		got.language = r.Header.Get("language")
		require.NoError(t, r.ParseMultipartForm(1<<20))
		got.model = r.FormValue("model")
		got.format = r.FormValue("response_format")
		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		got.filename = header.Filename
		got.file, _ = io.ReadAll(file)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"你好。再見。","duration":3.5,"n_speakers":2,
			"segments":[{"id":0,"start":0.0,"end":1.2,"speaker":0,"text":"你好。"},
			            {"id":1,"start":1.5,"end":3.5,"speaker":1,"text":"再見。"}],"trace_id":"t1"}`)
	}))
	t.Cleanup(server.Close)

	transcriber, err := minimax.NewTranscriber(speechConfig(server.URL + "/anthropic"))
	require.NoError(t, err)
	result, err := transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio:    provider.AudioSource{Bytes: []byte("RIFF"), Format: "wav"},
		Language: "zh",
	})
	require.NoError(t, err)

	assert.Equal(t, "/v1/speech_to_text", got.path, "the /anthropic chat suffix must be trimmed")
	assert.Equal(t, "Bearer test-key", got.auth)
	assert.Equal(t, "zh", got.language)
	assert.Equal(t, minimax.DefaultTranscribeModel, got.model)
	assert.Equal(t, "verbose_json", got.format)
	assert.Equal(t, "audio.wav", got.filename)
	assert.Equal(t, []byte("RIFF"), got.file)

	assert.Equal(t, "你好。再見。", result.Text)
	assert.Equal(t, int64(3500), result.Usage.AudioDurationMilliseconds)
	require.Len(t, result.Words, 2)
	assert.Equal(t, provider.TranscribedWord{Text: "再見。", StartMs: 1500, EndMs: 3500, Speaker: "1"}, result.Words[1])
}

func TestTranscribeMapsOpenAIStyleError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"audio too long","http_code":"400"},"request_id":"r9"}`)
	}))
	t.Cleanup(server.Close)

	transcriber, err := minimax.NewTranscriber(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{Bytes: []byte("x")},
	})
	var apiErr *provider.APIError
	require.True(t, errors.As(err, &apiErr), err)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, "audio too long", apiErr.Message)
	assert.Equal(t, "r9", apiErr.RequestID)
}

func TestTranscribeRejectsURLAndOversizedAudio(t *testing.T) {
	transcriber, err := minimax.NewTranscriber(speechConfig("http://unused"))
	require.NoError(t, err)
	_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{URL: "https://example.com/a.mp3"},
	})
	assert.ErrorContains(t, err, "URL is not supported")
	_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{Bytes: []byte("x"), DurationMilliseconds: 600_000},
	})
	assert.ErrorContains(t, err, "exceeds 500 seconds")
}
