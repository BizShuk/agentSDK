package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/sample/config"
)

func TestRunChatWithAPIKeyEnvironment(t *testing.T) {
	var requestPath string
	var authorization string
	var prompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if len(body.Messages) > 0 {
			prompt = body.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		fmtResponse := `{
			"choices":[{
				"message":{"role":"assistant","content":"hello from google"},
				"finish_reason":"stop"
			}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`
		if _, err := io.WriteString(w, fmtResponse); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := run(context.Background(), config.Config{
		Provider: "google",
		Type:     config.API_TYPE_CHAT,
		Auth:     config.AUTH_MODE_APIKEY,
		Model:    "chat-model",
		BaseURL:  server.URL,
		Prompt:   "hello",
	}, &out, func(key string) string {
		if key == "GOOGLE_API_KEY" {
			return "test-key"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("run chat: %v", err)
	}
	if requestPath != "/chat/completions" {
		t.Errorf("request path = %q, want /chat/completions", requestPath)
	}
	if authorization != "Bearer test-key" {
		t.Errorf("authorization = %q, want Bearer test-key", authorization)
	}
	if prompt != "hello" {
		t.Errorf("prompt = %q, want hello", prompt)
	}
	if !strings.Contains(out.String(), "hello from google") {
		t.Errorf("output = %q, want model text", out.String())
	}
}

func TestRunImageWithAPIKeyEnvironment(t *testing.T) {
	var requestPath string
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{
			"data":[{
				"url":"https://example.test/image.png",
				"mime_type":"image/png"
			}],
			"usage":{"cost_in_usd_ticks":200000000}
		}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := run(context.Background(), config.Config{
		Provider: "grok",
		Type:     config.API_TYPE_IMAGE,
		Auth:     config.AUTH_MODE_APIKEY,
		Model:    "grok-imagine-image-quality",
		BaseURL:  server.URL,
		Prompt:   "a paper fox",
	}, &out, func(key string) string {
		if key == "XAI_API_KEY" {
			return "test-key"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("run image: %v", err)
	}
	if requestPath != "/images/generations" {
		t.Errorf("request path = %q, want /images/generations", requestPath)
	}
	if authorization != "Bearer test-key" {
		t.Errorf("authorization = %q, want Bearer test-key", authorization)
	}
	if !strings.Contains(out.String(), "https://example.test/image.png") {
		t.Errorf("output = %q, want image URL", out.String())
	}
}

func TestRunMusicCoverWithAPIKeyEnvironment(t *testing.T) {
	var requestPath string
	var authorization string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{
			"data":{"audio":"https://example.test/generated.mp3","status":2},
			"trace_id":"trace-sample-1",
			"extra_info":{
				"music_duration":25364,
				"music_sample_rate":44100,
				"music_channel":2,
				"bitrate":256000,
				"music_size":813651
			},
			"base_resp":{"status_code":0}
		}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := run(context.Background(), config.Config{
		Provider:     "minimax",
		Type:         config.API_TYPE_MUSIC,
		Auth:         config.AUTH_MODE_APIKEY,
		Model:        "music-cover",
		BaseURL:      server.URL,
		Prompt:       "Jazz, smooth, late night lounge, saxophone",
		AudioURL:     "https://example.com/original-song.mp3",
		OutputFormat: "url",
		SampleRate:   44100,
		Bitrate:      256000,
		AudioFormat:  "mp3",
	}, &out, func(key string) string {
		if key == "MINIMAX_API_KEY" {
			return "test-key"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("run music: %v", err)
	}
	if requestPath != "/v1/music_generation" {
		t.Errorf("request path = %q, want /v1/music_generation", requestPath)
	}
	if authorization != "Bearer test-key" {
		t.Errorf("authorization = %q, want Bearer test-key", authorization)
	}
	if payload["model"] != "music-cover" {
		t.Errorf("model = %v, want music-cover", payload["model"])
	}
	if payload["audio_url"] != "https://example.com/original-song.mp3" {
		t.Errorf("audio_url = %v, want supplied URL", payload["audio_url"])
	}
	if payload["output_format"] != "url" {
		t.Errorf("output_format = %v, want url", payload["output_format"])
	}
	if !strings.Contains(out.String(), "https://example.test/generated.mp3") {
		t.Errorf("output = %q, want music URL", out.String())
	}
}

func TestRunSpeechWithAPIKeyEnvironment(t *testing.T) {
	var requestPath string
	var apiKey string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		apiKey = r.Header.Get("xi-api-key")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		if _, err := w.Write([]byte("synthesized")); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := run(context.Background(), config.Config{
		Provider:     "elevenlabs",
		Type:         config.API_TYPE_SPEECH,
		Auth:         config.AUTH_MODE_APIKEY,
		BaseURL:      server.URL,
		Prompt:       "say hello",
		Voice:        "voice-42",
		SpeechFormat: "mp3_44100_128",
	}, &out, func(key string) string {
		if key == "ELEVENLABS_API_KEY" {
			return "test-key"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("run speech: %v", err)
	}
	if requestPath != "/v1/text-to-speech/voice-42" {
		t.Errorf("request path = %q, want /v1/text-to-speech/voice-42", requestPath)
	}
	if apiKey != "test-key" {
		t.Errorf("xi-api-key = %q, want test-key", apiKey)
	}
	if payload["text"] != "say hello" {
		t.Errorf("text = %v, want say hello", payload["text"])
	}
	if !strings.Contains(out.String(), "speech.bytes=11") {
		t.Errorf("output = %q, want the synthesized byte count", out.String())
	}
}

func TestRunTranscribeWithAPIKeyEnvironment(t *testing.T) {
	var requestPath string
	var apiKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		apiKey = r.Header.Get("xi-api-key")
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"language_code":"en","text":"hello there"}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := run(context.Background(), config.Config{
		Provider: "elevenlabs",
		Type:     config.API_TYPE_TRANSCRIBE,
		Auth:     config.AUTH_MODE_APIKEY,
		BaseURL:  server.URL,
		AudioURL: "https://example.test/clip.mp3",
		Language: "en",
		Diarize:  true,
	}, &out, func(key string) string {
		if key == "ELEVENLABS_API_KEY" {
			return "test-key"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("run transcribe: %v", err)
	}
	if requestPath != "/v1/speech-to-text" {
		t.Errorf("request path = %q, want /v1/speech-to-text", requestPath)
	}
	if apiKey != "test-key" {
		t.Errorf("xi-api-key = %q, want test-key", apiKey)
	}
	if !strings.Contains(out.String(), "hello there") {
		t.Errorf("output = %q, want the transcript", out.String())
	}
}

func TestRunSpeechReturnsTypedUnsupportedCapability(t *testing.T) {
	err := run(context.Background(), config.Config{
		Provider: "google",
		Type:     config.API_TYPE_SPEECH,
		Auth:     config.AUTH_MODE_AUTO,
		Prompt:   "say hello",
	}, io.Discard, func(string) string { return "" })
	if !errors.Is(err, provider.ErrUnsupportedCapability) {
		t.Fatalf("error = %v, want ErrUnsupportedCapability", err)
	}
	var unsupported *provider.UnsupportedCapabilityError
	if !errors.As(err, &unsupported) {
		t.Fatalf("error = %T, want *provider.UnsupportedCapabilityError", err)
	}
	if unsupported.Provider != "google" ||
		unsupported.Capability != provider.CAPABILITY_AUDIO_SPEECH {
		t.Errorf("unsupported = %+v, want google/audio_speech", unsupported)
	}
}

func TestRunTranscribeRequiresAnAudioSource(t *testing.T) {
	err := run(context.Background(), config.Config{
		Provider: "elevenlabs",
		Type:     config.API_TYPE_TRANSCRIBE,
		Auth:     config.AUTH_MODE_AUTO,
	}, io.Discard, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "--audio-file or --audio-url") {
		t.Fatalf("error = %v, want an audio source requirement", err)
	}
}

func TestWriteProviderMatrixShowsTypeAndAuthSupport(t *testing.T) {
	var out bytes.Buffer
	if err := writeProviderMatrix(&out); err != nil {
		t.Fatalf("write matrix: %v", err)
	}
	text := out.String()
	for _, want := range []string{
		"PROVIDER",
		"CHAT",
		"IMAGE",
		"MUSIC",
		"SPEECH",
		"TRANSCRIBE",
		"google",
		"GOOGLE_API_KEY",
		"grok",
		"XAI_API_KEY",
		"minimax",
		"MINIMAX_API_KEY",
		"elevenlabs",
		"ELEVENLABS_API_KEY",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("matrix missing %q:\n%s", want, text)
		}
	}

	// chat, image, music, speech, transcribe — in header order.
	want := map[string][5]string{
		"google":     {"yes", "yes", "no", "no", "no"},
		"minimax":    {"yes", "yes", "yes", "yes", "no"},
		"elevenlabs": {"no", "no", "no", "yes", "yes"},
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		expected, ok := want[fields[0]]
		if !ok {
			continue
		}
		seen[fields[0]] = true
		if got := [5]string(fields[1:6]); got != expected {
			t.Errorf("%s capability row = %v, want %v", fields[0], got, expected)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("matrix missing %s row:\n%s", name, text)
		}
	}
}

func TestExecuteRejectsMissingPrompt(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := execute(
		context.Background(),
		[]string{"--provider", "ollama", "--type", "chat"},
		&stdout,
		&stderr,
	)
	if err == nil || !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("error = %v, want prompt is required", err)
	}
}
