package minimax_test

import (
	"context"
	"encoding/json"
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

func TestDesignVoiceDecodesHexPreview(t *testing.T) {
	var body map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/voice_design", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = io.WriteString(w, `{"voice_id":"ttv-1","trial_audio":"494433","base_resp":{"status_code":0,"status_msg":"success"}}`)
	}))
	t.Cleanup(server.Close)

	manager, err := minimax.NewVoice(speechConfig(server.URL))
	require.NoError(t, err)
	result, err := manager.DesignVoice(context.Background(), provider.VoiceDesignRequest{
		Prompt: "warm narrator", PreviewText: "hello",
	})
	require.NoError(t, err)
	assert.Equal(t, "warm narrator", body["prompt"])
	assert.Equal(t, "ttv-1", result.VoiceID)
	assert.Equal(t, []byte("ID3"), result.Preview.Bytes)
}

func TestCloneVoiceUploadsThenClones(t *testing.T) {
	var purposes []string
	var clone map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/files/upload":
			require.NoError(t, r.ParseMultipartForm(1<<20))
			purposes = append(purposes, r.FormValue("purpose"))
			id := map[string]string{"voice_clone": "111", "prompt_audio": "222"}[r.FormValue("purpose")]
			_, _ = io.WriteString(w, `{"file":{"file_id":`+id+`},"base_resp":{"status_code":0}}`)
		case "/v1/voice_clone":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&clone))
			_, _ = io.WriteString(w, `{"demo_audio":"https://cdn/demo.mp3","base_resp":{"status_code":0}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	manager, err := minimax.NewVoice(speechConfig(server.URL))
	require.NoError(t, err)
	result, err := manager.CloneVoice(context.Background(), provider.VoiceCloneRequest{
		VoiceID:     "MyVoice001",
		Sample:      provider.AudioSource{Bytes: []byte("sample"), Format: "wav"},
		PromptAudio: &provider.AudioSource{Bytes: []byte("prompt")},
		PromptText:  "hello there.",
		PreviewText: "preview",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"voice_clone", "prompt_audio"}, purposes)
	assert.EqualValues(t, 111, clone["file_id"])
	assert.Equal(t, map[string]any{"prompt_audio": float64(222), "prompt_text": "hello there."}, clone["clone_prompt"])
	assert.Equal(t, "speech-2.8-hd", clone["model"], "a preview needs a model; default to the speech default")
	assert.Equal(t, provider.VoiceCloneResult{VoiceID: "MyVoice001", PreviewURL: "https://cdn/demo.mp3"}, result)
}

func TestCloneVoiceRejectsBadIDAndSurfacesBaseResp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"base_resp":{"status_code":2038,"status_msg":"no cloning permission"}}`)
	}))
	t.Cleanup(server.Close)
	manager, err := minimax.NewVoice(speechConfig(server.URL))
	require.NoError(t, err)

	_, err = manager.CloneVoice(context.Background(), provider.VoiceCloneRequest{
		VoiceID: "1bad", Sample: provider.AudioSource{Bytes: []byte("x")},
	})
	assert.ErrorContains(t, err, "must be 8-256 chars")

	_, err = manager.CloneVoice(context.Background(), provider.VoiceCloneRequest{
		VoiceID: "GoodVoice1", Sample: provider.AudioSource{Bytes: []byte("x")},
	})
	var apiErr *provider.APIError
	require.True(t, errors.As(err, &apiErr), err)
	assert.Equal(t, "2038", apiErr.Code)
}

func TestDeleteVoiceRequiresCategory(t *testing.T) {
	var body map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/delete_voice", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = io.WriteString(w, `{"voice_id":"v","base_resp":{"status_code":0}}`)
	}))
	t.Cleanup(server.Close)
	manager, err := minimax.NewVoice(speechConfig(server.URL))
	require.NoError(t, err)

	assert.Error(t, manager.DeleteVoice(context.Background(), provider.VoiceDeleteRequest{VoiceID: "v"}))
	require.NoError(t, manager.DeleteVoice(context.Background(), provider.VoiceDeleteRequest{VoiceID: "v", Category: "voice_generation"}))
	assert.Equal(t, map[string]string{"voice_type": "voice_generation", "voice_id": "v"}, body)
}
