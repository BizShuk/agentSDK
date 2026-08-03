package elevenlabs_test

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/elevenlabs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multipartForm is one decoded upload: the flat fields plus the file part's
// name and bytes.
type multipartForm struct {
	fields   map[string]string
	fileName string
	fileData []byte
}

func decodeMultipart(t *testing.T, r *http.Request) multipartForm {
	t.Helper()

	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)

	form := multipartForm{fields: map[string]string{}}
	reader := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		raw, err := io.ReadAll(part)
		require.NoError(t, err)
		if part.FileName() != "" {
			form.fileName = part.FileName()
			form.fileData = raw
			continue
		}
		form.fields[part.FormName()] = string(raw)
	}
	return form
}

const transcriptBody = `{
	"language_code":"en",
	"language_probability":0.98,
	"text":"Hello there friend",
	"words":[
		{"text":"Hello","start":0.119,"end":0.4,"type":"word","speaker_id":"speaker_0"},
		{"text":" ","start":0.4,"end":0.42,"type":"spacing","speaker_id":"speaker_0"},
		{"text":"there","start":0.42,"end":0.9005,"type":"word","speaker_id":"speaker_1"}
	]
}`

func TestTranscriberMatchesBytesRequestContract(t *testing.T) {
	var (
		capturedMethod string
		capturedPath   string
		capturedKey    string
		capturedForm   multipartForm
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedKey = r.Header.Get("xi-api-key")
		capturedForm = decodeMultipart(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, transcriptBody)
	}))
	t.Cleanup(server.Close)

	transcriber, err := elevenlabs.NewTranscriber(speechConfig(server.URL))
	require.NoError(t, err)

	result, err := transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{
			Bytes:  []byte("RIFF-audio-bytes"),
			Format: "wav",
		},
		Language: "en",
		Diarize:  true,
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "/v1/speech-to-text", capturedPath)
	assert.Equal(t, "test-key", capturedKey)
	assert.Equal(t, "audio.wav", capturedForm.fileName)
	assert.Equal(t, []byte("RIFF-audio-bytes"), capturedForm.fileData)
	assert.Equal(t, elevenlabs.DefaultTranscribeModel, capturedForm.fields["model_id"])
	assert.Equal(t, "en", capturedForm.fields["language_code"])
	assert.Equal(t, "true", capturedForm.fields["diarize"])
	assert.NotContains(t, capturedForm.fields, "cloud_storage_url")

	assert.Equal(t, "Hello there friend", result.Text)
	assert.Equal(t, "en", result.Language)
	require.Len(t, result.Words, 3, "spacing entries are kept verbatim for callers to filter")
	assert.Equal(t, provider.TranscribedWord{
		Text: "Hello", StartMs: 119, EndMs: 400, Speaker: "speaker_0",
	}, result.Words[0])
	assert.Equal(t, provider.TranscribedWord{
		Text: " ", StartMs: 400, EndMs: 420, Speaker: "speaker_0",
	}, result.Words[1])
	assert.Equal(t, provider.TranscribedWord{
		Text: "there", StartMs: 420, EndMs: 901, Speaker: "speaker_1",
	}, result.Words[2],
		"fractional seconds round to the nearest millisecond")
}

func TestTranscriberSendsCloudStorageURL(t *testing.T) {
	var capturedForm multipartForm
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedForm = decodeMultipart(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"language_code":"ja","text":"こんにちは"}`)
	}))
	t.Cleanup(server.Close)

	transcriber, err := elevenlabs.NewTranscriber(speechConfig(server.URL))
	require.NoError(t, err)

	result, err := transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Model: "scribe_v1_experimental",
		Audio: provider.AudioSource{URL: "https://storage.example.test/clip.mp3"},
	})
	require.NoError(t, err)

	assert.Equal(t,
		"https://storage.example.test/clip.mp3",
		capturedForm.fields["cloud_storage_url"])
	assert.Equal(t, "scribe_v1_experimental", capturedForm.fields["model_id"])
	assert.Empty(t, capturedForm.fileName, "a URL request uploads no file part")
	assert.NotContains(t, capturedForm.fields, "language_code")
	assert.NotContains(t, capturedForm.fields, "diarize")

	assert.Equal(t, "こんにちは", result.Text)
	assert.Equal(t, "ja", result.Language)
	assert.Empty(t, result.Words)
}

func TestTranscriberFileNameFollowsFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   string
	}{
		{name: "plain", format: "mp3", want: "audio.mp3"},
		{name: "composite", format: "pcm_16000", want: "audio.pcm"},
		{name: "uppercase", format: "WAV", want: "audio.wav"},
		{name: "unset", format: "", want: "audio"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedForm multipartForm
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedForm = decodeMultipart(t, r)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"text":"ok"}`)
			}))
			t.Cleanup(server.Close)

			transcriber, err := elevenlabs.NewTranscriber(speechConfig(server.URL))
			require.NoError(t, err)
			_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
				Audio: provider.AudioSource{Bytes: []byte("audio"), Format: tt.format},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, capturedForm.fileName)
		})
	}
}

func TestTranscriberRejectsInvalidRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("an invalid request must not reach the upstream server")
	}))
	t.Cleanup(server.Close)

	transcriber, err := elevenlabs.NewTranscriber(speechConfig(server.URL))
	require.NoError(t, err)

	tests := []struct {
		name    string
		request provider.TranscribeRequest
		wantErr string
	}{
		{
			name:    "no audio",
			request: provider.TranscribeRequest{},
			wantErr: "audio bytes or URL is required",
		},
		{
			name: "both sources",
			request: provider.TranscribeRequest{
				Audio: provider.AudioSource{
					Bytes: []byte("audio"),
					URL:   "https://example.test/clip.mp3",
				},
			},
			wantErr: "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := transcriber.Transcribe(context.Background(), tt.request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestTranscriberRequiresCredential(t *testing.T) {
	transcriber, err := elevenlabs.NewTranscriber(provider.ResolvedConfig{
		BaseURL: "https://example.invalid",
	})
	require.NoError(t, err)
	_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{Bytes: []byte("audio")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential is required")
}

func TestTranscriberReturnsTypedAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "request-stt-1")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"detail":{"status":"invalid_file","message":"unsupported codec"}}`)
	}))
	t.Cleanup(server.Close)

	transcriber, err := elevenlabs.NewTranscriber(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{Bytes: []byte("audio")},
	})
	require.Error(t, err)

	var apiErr *provider.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "elevenlabs", apiErr.Provider)
	assert.Equal(t, "transcribe", apiErr.Operation)
	assert.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)
	assert.Equal(t, "invalid_file", apiErr.Code)
	assert.Equal(t, "unsupported codec", apiErr.Message)
	assert.Equal(t, "request-stt-1", apiErr.RequestID)
	assert.NotContains(t, err.Error(), "test-key")
}

func TestTranscriberRejectsUndecodableResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":`)
	}))
	t.Cleanup(server.Close)

	transcriber, err := elevenlabs.NewTranscriber(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{Bytes: []byte("audio")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}

func TestTranscriberRequestAuthOverridesConstructionAuth(t *testing.T) {
	var capturedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("xi-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"ok"}`)
	}))
	t.Cleanup(server.Close)

	transcriber, err := elevenlabs.NewTranscriber(speechConfig(server.URL))
	require.NoError(t, err)
	_, err = transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{Bytes: []byte("audio")},
		Auth:  core.Auth{APIKey: "request-key"},
	})
	require.NoError(t, err)
	assert.Equal(t, "request-key", capturedKey)
}

func TestNewTranscriberResolvesRegistryEnvironment(t *testing.T) {
	var capturedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("xi-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"ok"}`)
	}))
	t.Cleanup(server.Close)

	env := map[string]string{
		"ELEVENLABS_API_KEY":  "env-key",
		"ELEVENLABS_BASE_URL": server.URL,
	}
	transcriber, err := provider.NewTranscriber("elevenlabs", provider.Options{
		LookupEnv: func(key string) string { return env[key] },
	})
	require.NoError(t, err)
	result, err := transcriber.Transcribe(context.Background(), provider.TranscribeRequest{
		Audio: provider.AudioSource{URL: "https://storage.example.test/clip.mp3"},
	})
	require.NoError(t, err)
	assert.Equal(t, "env-key", capturedKey)
	assert.Equal(t, "ok", result.Text)
}

func TestDefaultCatalogListsBothCapabilities(t *testing.T) {
	ids := make([]string, 0, len(elevenlabs.DefaultCatalog()))
	for _, spec := range elevenlabs.DefaultCatalog() {
		ids = append(ids, spec.ID)
	}
	assert.Equal(t, []string{
		"scribe_v1",
		"eleven_flash_v2_5",
		"eleven_turbo_v2_5",
		"eleven_multilingual_v2",
	}, ids)
	assert.Equal(t,
		[]core.Modality{core.MODALITY_AUDIO},
		elevenlabs.DefaultCatalog()[0].Input,
		"the transcription model takes audio in")
	assert.True(t, strings.HasPrefix(elevenlabs.DefaultSpeechModel, "eleven_"))
}
