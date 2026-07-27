package config

import (
	"testing"
)

func TestAPITypeSet(t *testing.T) {
	var typ APIType
	if err := typ.Set("chat"); err != nil || typ != API_TYPE_CHAT {
		t.Errorf("Set(chat) = %v, %v", typ, err)
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
}
