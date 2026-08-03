package svc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/provider"
)

func TestChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"message":{"role":"assistant","content":"hello from svc chat"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}
		}`)
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := Chat(context.Background(), Request{
		Provider: "google",
		Prompt:   "ping",
		Options: provider.Options{
			BaseURL: server.URL,
			LookupEnv: func(k string) string {
				if k == "GOOGLE_API_KEY" {
					return "test-key"
				}
				return ""
			},
		},
	}, &out)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !strings.Contains(out.String(), "hello from svc chat") {
		t.Errorf("output = %q, want model text", out.String())
	}
}

func TestImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"data":[{"url":"https://example.test/svc.png"}],
			"usage":{"cost_in_usd_ticks":100}
		}`)
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := Image(context.Background(), Request{
		Provider: "grok",
		Prompt:   "a cat",
		Options: provider.Options{
			BaseURL: server.URL,
			LookupEnv: func(k string) string {
				if k == "XAI_API_KEY" {
					return "test-key"
				}
				return ""
			},
		},
	}, &out)
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	if !strings.Contains(out.String(), "https://example.test/svc.png") {
		t.Errorf("output = %q, want image URL", out.String())
	}
}

func TestMusic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"data":{"audio":"https://example.test/svc.mp3","status":2},
			"base_resp":{"status_code":0}
		}`)
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := Music(context.Background(), Request{
		Provider:     "minimax",
		Prompt:       "Jazz, smooth, late night lounge",
		AudioURL:     "https://example.test/original.mp3",
		OutputFormat: "url",
		SampleRate:   44100,
		Bitrate:      256000,
		AudioFormat:  "mp3",
		Options: provider.Options{
			Model:   "music-cover",
			BaseURL: server.URL,
			LookupEnv: func(k string) string {
				if k == "MINIMAX_API_KEY" {
					return "test-key"
				}
				return ""
			},
		},
	}, &out)
	if err != nil {
		t.Fatalf("Music: %v", err)
	}
	if !strings.Contains(out.String(), "https://example.test/svc.mp3") {
		t.Errorf("output = %q, want music URL", out.String())
	}
}

func TestSpeech(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("output_format"); got != "mp3_44100_128" {
			t.Errorf("output_format = %q, want mp3_44100_128", got)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("svc-audio-bytes"))
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := Speech(context.Background(), Request{
		Provider:     "elevenlabs",
		Prompt:       "say hello",
		Voice:        "voice-42",
		SpeechFormat: "mp3_44100_128",
		Options: provider.Options{
			BaseURL: server.URL,
			LookupEnv: func(k string) string {
				if k == "ELEVENLABS_API_KEY" {
					return "test-key"
				}
				return ""
			},
		},
	}, &out)
	if err != nil {
		t.Fatalf("Speech: %v", err)
	}
	if !strings.Contains(out.String(), "speech.bytes=15") {
		t.Errorf("output = %q, want the synthesized byte count", out.String())
	}
}

func TestTranscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"language_code":"en","text":"hello from svc"}`)
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := Transcribe(context.Background(), Request{
		Provider: "elevenlabs",
		AudioURL: "https://example.test/clip.mp3",
		Options: provider.Options{
			BaseURL: server.URL,
			LookupEnv: func(k string) string {
				if k == "ELEVENLABS_API_KEY" {
					return "test-key"
				}
				return ""
			},
		},
	}, &out)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if !strings.Contains(out.String(), "hello from svc") {
		t.Errorf("output = %q, want the transcript", out.String())
	}
}

func TestTranscribeUploadsALocalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.wav")
	if err := os.WriteFile(path, []byte("RIFF-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"uploaded"}`)
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	err := Transcribe(context.Background(), Request{
		Provider:  "elevenlabs",
		AudioFile: path,
		Options: provider.Options{
			BaseURL:   server.URL,
			LookupEnv: func(string) string { return "test-key" },
		},
	}, &out)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Errorf("content type = %q, want multipart/form-data", contentType)
	}
	if !strings.Contains(out.String(), "uploaded") {
		t.Errorf("output = %q, want the transcript", out.String())
	}
}

func TestSpeechRejectsUnsupportedProvider(t *testing.T) {
	err := Speech(context.Background(), Request{
		Provider: "google",
		Prompt:   "say hello",
		Options:  provider.Options{LookupEnv: func(string) string { return "" }},
	}, io.Discard)
	if !errors.Is(err, provider.ErrUnsupportedCapability) {
		t.Fatalf("error = %v, want ErrUnsupportedCapability", err)
	}
}

func TestTranscribeRejectsUnsupportedProvider(t *testing.T) {
	err := Transcribe(context.Background(), Request{
		Provider: "minimax",
		AudioURL: "https://example.test/clip.mp3",
		Options:  provider.Options{LookupEnv: func(string) string { return "" }},
	}, io.Discard)
	if !errors.Is(err, provider.ErrUnsupportedCapability) {
		t.Fatalf("error = %v, want ErrUnsupportedCapability", err)
	}
}

func TestWriteJSON(t *testing.T) {
	var out bytes.Buffer
	data := map[string]string{"foo": "bar"}
	if err := WriteJSON(&out, data); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["foo"] != "bar" {
		t.Errorf("decoded = %v, want foo: bar", decoded)
	}
}
