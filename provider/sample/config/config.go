package config

import (
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/all"
)

type Config struct {
	Provider string
	Type     APIType
	Auth     AuthMode
	Model    string
	BaseURL  string
	Prompt   string
	JSON     bool

	Lyrics       string
	AudioURL     string
	OutputFormat string
	SampleRate   int
	Bitrate      int
	AudioFormat  string
}

func (cfg Config) Validate() (provider.Entry, error) {
	if strings.TrimSpace(cfg.Prompt) == "" {
		return provider.Entry{}, fmt.Errorf("prompt is required")
	}
	entry, ok := provider.Lookup(cfg.Provider)
	if !ok {
		return provider.Entry{}, fmt.Errorf(
			"unknown provider %q (registered: %s)",
			cfg.Provider,
			strings.Join(provider.Names(), ", "),
		)
	}
	switch cfg.Type {
	case API_TYPE_CHAT, API_TYPE_IMAGE, API_TYPE_MUSIC, API_TYPE_AUDIO:
	default:
		return provider.Entry{}, fmt.Errorf(
			"type %q must be chat, image, music, or audio",
			cfg.Type,
		)
	}
	switch cfg.Auth {
	case AUTH_MODE_AUTO, AUTH_MODE_APIKEY, AUTH_MODE_OAUTH:
	default:
		return provider.Entry{}, fmt.Errorf("auth %q must be auto, api_key, or oauth", cfg.Auth)
	}
	return entry, nil
}

func (cfg Config) ProviderOptions(lookupEnv func(string) string) provider.Options {
	kind := core.CREDENTIAL_KIND_AUTO
	switch cfg.Auth {
	case AUTH_MODE_APIKEY:
		kind = core.CREDENTIAL_KIND_APIKEY
	case AUTH_MODE_OAUTH:
		kind = core.CREDENTIAL_KIND_OAUTH
	}
	return provider.Options{
		Model:          cfg.Model,
		BaseURL:        cfg.BaseURL,
		LookupEnv:      lookupEnv,
		CredentialKind: kind,
	}
}
