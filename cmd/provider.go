// Package cmd hosts the cobra subcommands mounted by the root agentsdk
// binary. provider.go wires the "provider" subcommand — a manual-test CLI
// over the provider layer with no Agent, Engine, or harness in the path.
// The per-type handlers (chat, image, music, speech, transcribe) live in
// cmd/provider; this file owns flags and dispatch.
package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"os"

	providercli "github.com/bizshuk/agentsdk/cmd/provider"
	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/all"
	gosdkconfig "github.com/bizshuk/gosdk/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// DEFAULT_PROVIDER_TIMEOUT bounds one manual-test request; media generation
// (music, speech) routinely needs more than a chat round-trip.
const DEFAULT_PROVIDER_TIMEOUT = 2 * time.Minute

var (
	ProviderName           string
	ProviderType           string
	ProviderModel          string
	ProviderAPIKey         string
	ProviderBaseURL        string
	ProviderSystem         string
	ProviderMaxTokens      int
	ProviderStream         bool
	ProviderAsJSON         bool
	ProviderListModels     bool
	ProviderCredentialKind string
	ProviderTimeout        time.Duration

	ProviderLyrics       string
	ProviderAudioURL     string
	ProviderAudioFile    string
	ProviderVoice        string
	ProviderSpeechFormat string
	ProviderLanguage     string
	ProviderDiarize      bool
	ProviderOutputFormat string
	ProviderSampleRate   int
	ProviderBitrate      int
	ProviderAudioFormat  string
)

// ProviderCmd is the package-level "provider" subcommand.
//
//	provider [flags] <prompt...>
//
// It is mounted by main.go as a child of the root cobra command. The CLI
// intentionally bypasses runtime.Engine / harness packages so it stays
// useful as a wire-format smoke test: any provider-side regression
// (auth header, DTO translate, SSE parser) is exposed here without
// requiring a full agentic loop.
var ProviderCmd = &cobra.Command{
	Use:   "provider [flags] <prompt>",
	Short: "Run a single prompt against a provider, bypassing the agent loop",
	Args:  cobra.ArbitraryArgs,
	Long: strings.TrimSpace(`
provider is the manual-test CLI for the provider adapter family.
It calls the provider layer directly — no Agent, Engine, tools, or
harness — so any provider regression is exposed immediately. --type
selects the API surface; chat is the default.

Examples:

  provider "ping" --provider minimax
  provider "summarize X" --provider anthropic --model claude-sonnet-5
  provider "summarize X" -m claude-sonnet-5 --provider anthropic     # -m 是 --model 短名
  provider "hello" --provider ollama --base-url http://localhost:11434/v1
  provider --stream "stream me a haiku" --provider anthropic
  provider "a paper fox" --provider google --type image
  provider "Jazz, smooth, late night lounge" --provider minimax --type music \
    --model music-cover --audio-url https://example.com/song.mp3
  provider "早安，新加坡" --provider elevenlabs --type speech --speech-format mp3_44100_128
  provider --provider elevenlabs --type transcribe --audio-file ./clip.mp3 --diarize
  provider --list-models --provider google
  provider --list-providers
  provider --list                          # provider × capability × auth-env matrix
`),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		errOut := cmd.ErrOrStderr()

		// Boot gosdk/config once per invocation so .env / config.yaml /
		// settings.json (in cwd, ./conf, ~/.config/agentsdk/) participate
		// in credential resolution. Failures degrade gracefully — flag
		// + OS env still work without gosdk.
		if err := bootGosdkConfig(); err != nil {
			fmt.Fprintf(errOut, "[provider] gosdk/config: %v (continuing with OS env only)\n", err)
		}

		if listProviders, _ := cmd.Flags().GetBool("list-providers"); listProviders {
			fmt.Fprintln(out, strings.Join(provider.Names(), ", "))
			return nil
		}
		if list, _ := cmd.Flags().GetBool("list"); list {
			return providercli.WriteMatrix(out)
		}

		entry, ok := provider.Lookup(ProviderName)
		if !ok {
			return fmt.Errorf("unknown provider %q (registered: %s)",
				ProviderName, strings.Join(provider.Names(), ", "))
		}
		label := entry.Name
		options := provider.Options{
			Model:          ProviderModel,
			APIKey:         ProviderAPIKey,
			BaseURL:        ProviderBaseURL,
			LookupEnv:      envLookup,
			CredentialKind: ProviderCredentialKind,
		}

		// Catalog listing is a discovery surface, not a chat call: an
		// audio-only provider (elevenlabs) still ships a static Catalog and
		// a live one.
		if ProviderListModels {
			return providercli.Catalog(cmd.Context(), entry, options, errOut, out)
		}

		if ProviderTimeout <= 0 {
			return fmt.Errorf("timeout must be greater than zero")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), ProviderTimeout)
		defer cancel()

		request := providercli.Request{
			Provider:     ProviderName,
			Prompt:       strings.TrimSpace(strings.Join(args, " ")),
			JSON:         ProviderAsJSON,
			Options:      options,
			System:       ProviderSystem,
			MaxTokens:    ProviderMaxTokens,
			Stream:       ProviderStream,
			Lyrics:       ProviderLyrics,
			OutputFormat: ProviderOutputFormat,
			SampleRate:   ProviderSampleRate,
			Bitrate:      ProviderBitrate,
			AudioFormat:  ProviderAudioFormat,
			Voice:        ProviderVoice,
			SpeechFormat: ProviderSpeechFormat,
			AudioURL:     ProviderAudioURL,
			AudioFile:    ProviderAudioFile,
			Language:     ProviderLanguage,
			Diarize:      ProviderDiarize,
		}

		model := ProviderModel
		if model == "" {
			model = "default"
		}
		fmt.Fprintf(errOut, "[provider] %s | model=%s | type=%s | stream=%v\n",
			label, model, ProviderType, ProviderStream)

		switch ProviderType {
		case "chat":
			if !entry.Supports(provider.CAPABILITY_CHAT) {
				return fmt.Errorf("provider %s has no chat surface; supported capabilities: %s",
					label, providercli.JoinCapabilities(entry.Capabilities()))
			}
			if request.Prompt == "" {
				return fmt.Errorf("prompt is required (or pass --list-models / --list-providers)")
			}
			return providercli.Chat(ctx, request, out)
		case "image":
			return providercli.Image(ctx, request, out)
		case "music":
			return providercli.Music(ctx, request, out)
		case "speech":
			return providercli.Speech(ctx, request, out)
		case "transcribe":
			return providercli.Transcribe(ctx, request, out)
		default:
			return fmt.Errorf("type %q must be chat, image, music, speech, or transcribe",
				ProviderType)
		}
	},
}

func init() {
	flags := ProviderCmd.Flags()
	flags.StringVar(&ProviderName, "provider", provider.DEFAULT_NAME,
		"Provider family (case-insensitive); use --list to see linked providers.")
	flags.StringVar(&ProviderType, "type", "chat",
		"API type: chat | image | music | speech | transcribe.")
	flags.StringVarP(&ProviderModel, "model", "m", "",
		"Model id (alias -m); empty = adapter flagship default. "+
			"Use --list-models to see the provider's catalog.")
	flags.StringVar(&ProviderAPIKey, "api-key", "",
		"API key override; empty = resolved from .env / "+
			"~/.config/agentsdk/.env / shell env "+
			"(MINIMAX_API_KEY / ANTHROPIC_OAUTH_TOKEN+ANTHROPIC_API_KEY / "+
			"GOOGLE_API_KEY / XAI_API_KEY / OPENAI_API_KEY). "+
			"Precedence: --api-key > .env > OS env.")
	flags.StringVar(&ProviderBaseURL, "base-url", "",
		"Base URL override; empty = resolved from .env / shell env / "+
			"adapter default. Same precedence as --api-key.")
	flags.StringVar(&ProviderSystem, "system", "",
		"Optional system message prepended to the prompt (chat only).")
	flags.IntVar(&ProviderMaxTokens, "max-tokens", 0,
		"max_tokens for the request; 0 = adapter default (chat only).")
	flags.BoolVar(&ProviderStream, "stream", false,
		"Use SSE Stream instead of blocking Generate (chat only).")
	flags.BoolVar(&ProviderAsJSON, "json", false,
		"Print the full response as JSON.")
	flags.BoolVar(&ProviderListModels, "list-models", false,
		"Print the provider's model catalog and exit.")
	flags.Bool("list-providers", false,
		"Print the registered provider names and exit.")
	flags.Bool("list", false,
		"Print the provider × capability × auth-env matrix and exit.")
	flags.StringVar(&ProviderCredentialKind, "credential-kind", "auto",
		"Credential preference: auto | api_key | oauth. "+
			"auto = OAuth outranks API key when both env are set (legacy precedence, current behavior). "+
			"api_key = strict: only the API key env is consulted; missing env → startup error. "+
			"oauth = strict: only the OAuth env is consulted; missing env → startup error. "+
			"Matches agent/spec.Model.CredentialKind and core.CREDENTIAL_KIND_* constants.")
	flags.DurationVar(&ProviderTimeout, "timeout", DEFAULT_PROVIDER_TIMEOUT,
		"Request timeout.")

	flags.StringVar(&ProviderLyrics, "lyrics", "",
		"Music lyrics; use newline characters between lines.")
	flags.StringVar(&ProviderAudioURL, "audio-url", "",
		"Audio URL: music cover reference, or the clip to transcribe.")
	flags.StringVar(&ProviderAudioFile, "audio-file", "",
		"Local audio file to upload for transcription.")
	flags.StringVar(&ProviderVoice, "voice", "",
		"Speech voice id; empty uses the provider default.")
	flags.StringVar(&ProviderSpeechFormat, "speech-format", "",
		"Speech output encoding, e.g. mp3_44100_128 | pcm_16000.")
	flags.StringVar(&ProviderLanguage, "language", "",
		"ISO-639 language hint for transcription.")
	flags.BoolVar(&ProviderDiarize, "diarize", false,
		"Attribute transcribed words to speakers.")
	flags.StringVar(&ProviderOutputFormat, "output-format", "url",
		"Music response encoding: url | hex.")
	flags.IntVar(&ProviderSampleRate, "sample-rate", 44100,
		"Music sample rate in Hz.")
	flags.IntVar(&ProviderBitrate, "bitrate", 256000,
		"Music bitrate in bits per second.")
	flags.StringVar(&ProviderAudioFormat, "audio-format", "mp3",
		"Generated music format (mp3 | wav | pcm), or the transcribe source format.")
}

// ResetFlags resets ProviderCmd flag state for clean test execution.
func ResetFlags() {
	ProviderName = provider.DEFAULT_NAME
	ProviderType = "chat"
	ProviderModel = ""
	ProviderAPIKey = ""
	ProviderBaseURL = ""
	ProviderSystem = ""
	ProviderMaxTokens = 0
	ProviderStream = false
	ProviderAsJSON = false
	ProviderListModels = false
	ProviderCredentialKind = "auto"
	ProviderTimeout = DEFAULT_PROVIDER_TIMEOUT
	ProviderLyrics = ""
	ProviderAudioURL = ""
	ProviderAudioFile = ""
	ProviderVoice = ""
	ProviderSpeechFormat = ""
	ProviderLanguage = ""
	ProviderDiarize = false
	ProviderOutputFormat = "url"
	ProviderSampleRate = 44100
	ProviderBitrate = 256000
	ProviderAudioFormat = "mp3"
	ProviderPricingWrite = false

	ProviderCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
	ProviderPricingRefreshCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
}

// ---------------------------------------------------------------------------
// gosdk config wiring
// ---------------------------------------------------------------------------

// bootGosdkConfig wires viper from gosdk's standard search paths
// (`.`, `./conf`, `~/.config/agentsdk/`) for filenames `.env`,
// `.env.local`, `config.yaml`, `config.local.yaml`, `settings.json`,
// `settings.local.json`. It also binds every provider env key to viper
// so envLookup falls through to OS env when no .env override exists.
//
// We deliberately do NOT enable gosdkconfig.WithWatch — provider is a
// one-shot CLI, the process exits after one request.
func bootGosdkConfig() error {
	gosdkconfig.Default(gosdkconfig.WithAppName("agentsdk"))
	if gosdkconfig.GetAppName() == "" {
		return fmt.Errorf("gosdk/config: appName not bound")
	}
	return nil
}

// envLookup returns the merged value for `key`, fanning out through
// viper (config.yaml / .env / gosdk's loaded sources) → OS env. The
// viper instance does not auto-bind env vars, so an explicit os.Getenv
// fallback ensures shell-exported credentials still flow through. Empty
// when nothing is set; callers propagate that to the adapter which then
// applies its own default.
//
// Viper normalizes config-file keys to lowercase, so we look up the
// lower-cased form while still using uppercase env-var names in the
// registry (those ARE the literal env-var names, so they stay upper-case).
func envLookup(key string) string {
	if v := viper.GetString(strings.ToLower(key)); v != "" {
		return v
	}
	return os.Getenv(key)
}
