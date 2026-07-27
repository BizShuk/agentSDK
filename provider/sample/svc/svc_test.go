package svc

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

func TestAudio(t *testing.T) {
	err := Audio("google")
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
