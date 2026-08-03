package config

import (
	"testing"
)

func TestAPITypeSet(t *testing.T) {
	var typ APIType
	if err := typ.Set("chat"); err != nil || typ != API_TYPE_CHAT {
		t.Errorf("Set(chat) = %v, %v", typ, err)
	}
	if err := typ.Set("music"); err != nil || typ != API_TYPE_MUSIC {
		t.Errorf("Set(music) = %v, %v", typ, err)
	}
	if err := typ.Set("speech"); err != nil || typ != API_TYPE_SPEECH {
		t.Errorf("Set(speech) = %v, %v", typ, err)
	}
	if err := typ.Set("TRANSCRIBE"); err != nil || typ != API_TYPE_TRANSCRIBE {
		t.Errorf("Set(TRANSCRIBE) = %v, %v", typ, err)
	}
	if err := typ.Set("audio"); err == nil {
		t.Error("Set(audio) expected error; audio split into speech and transcribe")
	}
	if err := typ.Set("invalid"); err == nil {
		t.Error("Set(invalid) expected error")
	}
	if typ.Type() != "string" {
		t.Errorf("Type() = %q, want string", typ.Type())
	}
}

func TestAuthModeSet(t *testing.T) {
	var mode AuthMode
	if err := mode.Set("api_key"); err != nil || mode != AUTH_MODE_APIKEY {
		t.Errorf("Set(api_key) = %v, %v", mode, err)
	}
	if err := mode.Set("invalid"); err == nil {
		t.Error("Set(invalid) expected error")
	}
	if mode.Type() != "string" {
		t.Errorf("Type() = %q, want string", mode.Type())
	}
}

func TestConfigValidate(t *testing.T) {
	cfg := Config{
		Provider: "google",
		Type:     API_TYPE_CHAT,
		Auth:     AUTH_MODE_AUTO,
		Prompt:   "hello",
	}
	if _, err := cfg.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}

	emptyPrompt := cfg
	emptyPrompt.Prompt = ""
	if _, err := emptyPrompt.Validate(); err == nil {
		t.Error("Validate() empty prompt expected error")
	}

	unknownProvider := cfg
	unknownProvider.Provider = "unknown_xyz"
	if _, err := unknownProvider.Validate(); err == nil {
		t.Error("Validate() unknown provider expected error")
	}

	// Transcription reads audio, so it needs an audio source rather than a
	// prompt.
	transcribe := Config{
		Provider: "elevenlabs",
		Type:     API_TYPE_TRANSCRIBE,
		Auth:     AUTH_MODE_AUTO,
	}
	if _, err := transcribe.Validate(); err == nil {
		t.Error("Validate() transcribe without audio expected error")
	}
	transcribe.AudioURL = "https://example.test/clip.mp3"
	if _, err := transcribe.Validate(); err != nil {
		t.Errorf("Validate() transcribe with audio URL: %v", err)
	}
}
