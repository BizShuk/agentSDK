package provider

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

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/all"
)

func TestChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"message":{"role":"assistant","content":"hello from handler chat"},"finish_reason":"stop"}],
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
	if !strings.Contains(out.String(), "hello from handler chat") {
		t.Errorf("output = %q, want model text", out.String())
	}
}

type streamStub struct {
	chunks []core.ModelChunk
}

func (s streamStub) Generate(context.Context, core.ModelRequest) (core.ModelResult, error) {
	return core.ModelResult{}, nil
}

func (s streamStub) Stream(context.Context, core.ModelRequest) (<-chan core.ModelChunk, error) {
	ch := make(chan core.ModelChunk, len(s.chunks))
	for _, chunk := range s.chunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func TestRunStreamRejectsMissingTerminalChunk(t *testing.T) {
	var out bytes.Buffer
	err := runStream(
		context.Background(),
		streamStub{chunks: []core.ModelChunk{{
			Kind: core.PART_KIND_PLAIN_TEXT,
			Text: "partial",
		}}},
		core.ModelRequest{},
		&out,
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "stream closed before terminal chunk") {
		t.Fatalf("error = %v, want terminal-chunk failure", err)
	}
	if out.String() != "partial" {
		t.Errorf("output = %q, want partial text", out.String())
	}
}

func TestImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"data":[{"url":"https://example.test/handler.png"}],
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
	if !strings.Contains(out.String(), "https://example.test/handler.png") {
		t.Errorf("output = %q, want image URL", out.String())
	}
}

func TestImageRequiresPrompt(t *testing.T) {
	err := Image(context.Background(), Request{Provider: "grok"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("error = %v, want prompt is required", err)
	}
}

func TestMusic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"data":{"audio":"https://example.test/handler.mp3","status":2},
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
	if !strings.Contains(out.String(), "https://example.test/handler.mp3") {
		t.Errorf("output = %q, want music URL", out.String())
	}
}

func TestSpeech(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("output_format"); got != "mp3_44100_128" {
			t.Errorf("output_format = %q, want mp3_44100_128", got)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("cli-audio-bytes"))
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
		_, _ = io.WriteString(w, `{"language_code":"en","text":"hello from handler"}`)
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
	if !strings.Contains(out.String(), "hello from handler") {
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

func TestTranscribeRequiresAnAudioSource(t *testing.T) {
	err := Transcribe(context.Background(), Request{Provider: "elevenlabs"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "requires --audio-file or --audio-url") {
		t.Fatalf("error = %v, want audio source requirement", err)
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

func TestWriteMatrixShowsTypeAndAuthSupport(t *testing.T) {
	var out bytes.Buffer
	if err := WriteMatrix(&out); err != nil {
		t.Fatalf("WriteMatrix: %v", err)
	}
	text := out.String()

	for _, want := range []string{
		"PROVIDER",
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

	// chat, image, music, speech, transcribe, live, translate — in header order.
	want := map[string][7]string{
		"google":     {"yes", "yes", "no", "no", "no", "yes", "yes"},
		"codex":      {"yes", "no", "no", "no", "no", "yes", "no"},
		"minimax":    {"yes", "yes", "yes", "yes", "no", "no", "no"},
		"elevenlabs": {"no", "no", "no", "yes", "yes", "no", "no"},
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		expected, ok := want[fields[0]]
		if !ok {
			continue
		}
		seen[fields[0]] = true
		if got := [7]string(fields[1:8]); got != expected {
			t.Errorf("%s capability row = %v, want %v", fields[0], got, expected)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("matrix missing %s row:\n%s", name, text)
		}
	}
}
