// Package cmd hosts the cobra subcommands mounted by the root agentsdk
// binary. provider.go wires the "provider" subcommand — a thin smoke-test
// CLI that calls core.Provider.Generate or core.StreamProvider.Stream
// directly, with no Agent, Engine, or harness in the path.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/all"
	gosdkconfig "github.com/bizshuk/gosdk/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	ProviderName           string
	ProviderModel          string
	ProviderAPIKey         string
	ProviderBaseURL        string
	ProviderSystem         string
	ProviderMaxTokens      int
	ProviderStream         bool
	ProviderAsJSON         bool
	ProviderListModels     bool
	ProviderCredentialKind string
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
	Long: strings.TrimSpace(`
provider is the minimal smoke-test CLI for the provider adapter family.
It calls core.Provider.Generate (or core.StreamProvider.Stream with --stream) directly —
no Agent, Engine, tools, or harness — so any provider regression is
exposed immediately.

Examples:

  provider "ping" --provider minimax
  provider "summarize X" --provider anthropic --model claude-sonnet-5
  provider "summarize X" -m claude-sonnet-5 --provider anthropic     # -m 是 --model 短名
  provider "summarize X" --provider google --model gemini-3-flash-preview
  provider "hello" --provider grok --model grok-4
  provider "hello" --provider ollama --base-url http://localhost:11434/v1
  provider --stream "stream me a haiku" --provider anthropic
  provider --list-models --provider google
  provider --list-models --provider ollama    # lists the models your server actually pulled
  provider --list-providers
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
		// a live one. model_generate is therefore not a precondition here —
		// the client is only built to reach the live core.ModelLister.
		if ProviderListModels {
			var static []core.ModelSpec
			if entry.Catalog != nil {
				static = entry.Catalog()
			}
			lister, err := openLister(entry, options)
			if err != nil {
				// A client that will not build (missing credential, bad
				// base URL) fails the same way a failed live call does:
				// discovery still answers from the bundled catalog.
				fmt.Fprintf(errOut,
					"[provider] %s: live catalog unavailable, using static (%v)\n", label, err)
			}
			return dumpCatalog(cmd.Context(), errOut, out, lister, label, static)
		}

		if !entry.Supports(provider.CAPABILITY_MODEL_GENERATE) {
			return fmt.Errorf("provider %s has no chat surface; supported capabilities: %s",
				label, joinCapabilities(entry.Capabilities()))
		}

		prov, err := provider.New(ProviderName, options)
		if err != nil {
			return err
		}

		prompt := strings.TrimSpace(strings.Join(args, " "))
		if prompt == "" {
			return fmt.Errorf("prompt is required (or pass --list-models / --list-providers)")
		}

		req := buildRequest(prompt, ProviderSystem, ProviderMaxTokens)

		model := ProviderModel
		if model == "" {
			model = "default"
		}
		fmt.Fprintf(errOut, "[provider] %s | model=%s | stream=%v\n",
			label, model, ProviderStream)

		if ProviderStream {
			return runStream(cmd.Context(), prov, req, out, ProviderAsJSON)
		}
		return runGenerate(cmd.Context(), prov, req, out, ProviderAsJSON)
	},
}

func init() {
	flags := ProviderCmd.Flags()
	flags.StringVar(&ProviderName, "provider", provider.DEFAULT_NAME,
		"Provider family (minimax | anthropic | google | grok | ollama; case-insensitive).")
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
		"Optional system message prepended to the prompt.")
	flags.IntVar(&ProviderMaxTokens, "max-tokens", 0,
		"max_tokens for the request; 0 = adapter default.")
	flags.BoolVar(&ProviderStream, "stream", false,
		"Use SSE Stream instead of blocking Generate.")
	flags.BoolVar(&ProviderAsJSON, "json", false,
		"Print the full ModelResult / chunk stream as JSON lines.")
	flags.BoolVar(&ProviderListModels, "list-models", false,
		"Print the provider's static catalog and exit.")
	flags.Bool("list-providers", false,
		"Print the registered provider names and exit.")
	flags.StringVar(&ProviderCredentialKind, "credential-kind", "auto",
		"Credential preference: auto | api_key | oauth. "+
			"auto = OAuth outranks API key when both env are set (legacy precedence, current behavior). "+
			"api_key = strict: only the API key env is consulted; missing env → startup error. "+
			"oauth = strict: only the OAuth env is consulted; missing env → startup error. "+
			"Matches agent/spec.Model.CredentialKind and core.CREDENTIAL_KIND_* constants.")
}

// ResetFlags resets ProviderCmd flag state for clean test execution.
func ResetFlags() {
	ProviderName = provider.DEFAULT_NAME
	ProviderModel = ""
	ProviderAPIKey = ""
	ProviderBaseURL = ""
	ProviderSystem = ""
	ProviderMaxTokens = 0
	ProviderStream = false
	ProviderAsJSON = false
	ProviderListModels = false
	ProviderCredentialKind = "auto"

	ProviderCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
}

// ---------------------------------------------------------------------------
// provider registry
// ---------------------------------------------------------------------------

// The name → adapter mapping lives in provider, shared with the
// agent composition layer so a config file and this CLI cannot disagree
// about which providers exist or how their credentials resolve.

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
// one-shot CLI, the process exits after one Generate.
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

// ---------------------------------------------------------------------------
// request building
// ---------------------------------------------------------------------------

// buildRequest turns a CLI prompt into a single-turn core.ModelRequest.
// The system message is optional; parts default to plain text.
func buildRequest(prompt, system string, maxTokens int) core.ModelRequest {
	msgs := []core.Message{}
	if sys := strings.TrimSpace(system); sys != "" {
		msgs = append(msgs, core.Message{
			Role:  core.ROLE_SYSTEM,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: sys}},
			Ts:    time.Now().UTC(),
		})
	}
	msgs = append(msgs, core.Message{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: prompt}},
		Ts:    time.Now().UTC(),
	})
	return core.ModelRequest{Messages: msgs, MaxTokens: maxTokens}
}

// ---------------------------------------------------------------------------
// dispatch
// ---------------------------------------------------------------------------

func runGenerate(ctx context.Context, prov core.Provider, req core.ModelRequest,
	out io.Writer, asJSON bool,
) error {
	res, err := prov.Generate(ctx, req)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	if asJSON {
		raw, err := json.Marshal(res)
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
		fmt.Fprintln(out, string(raw))
		return nil
	}
	if res.Text != "" {
		fmt.Fprintln(out, res.Text)
	}
	fmt.Fprintf(out, "[stop=%s tokens=%d/%d]\n",
		res.StopReason, res.Usage.PromptTokens, res.Usage.CompletionTokens)
	return nil
}

func runStream(ctx context.Context, prov core.StreamProvider, req core.ModelRequest,
	out io.Writer, asJSON bool,
) error {
	ch, err := prov.Stream(ctx, req)
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	sawDone := false
	enc := json.NewEncoder(out)
	for c := range ch {
		if c.Done {
			sawDone = true
		}
		if asJSON {
			if err := enc.Encode(c); err != nil {
				return err
			}
			continue
		}
		if c.Done {
			continue
		}
		if c.Kind == core.PART_KIND_PLAIN_TEXT && c.Text != "" {
			fmt.Fprint(out, c.Text)
		}
	}
	if !sawDone {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("stream interrupted: %w", err)
		}
		return fmt.Errorf("stream closed before terminal chunk")
	}
	if asJSON {
		return nil
	}
	fmt.Fprintln(out)
	return nil
}

// openLister builds whichever client on this entry can enumerate models
// live, or returns nil when none can. The chat surface is preferred; an
// audio-only entry falls back to its speech client, which is where
// elevenlabs hangs its GET /v1/models call. A nil lister is not an error —
// dumpCatalog then prints the bundled catalog alone.
func openLister(entry provider.Entry, options provider.Options) (core.ModelLister, error) {
	switch {
	case entry.Supports(provider.CAPABILITY_MODEL_GENERATE):
		prov, err := provider.New(entry.Name, options)
		if err != nil {
			return nil, err
		}
		lister, _ := prov.(core.ModelLister)
		return lister, nil
	case entry.Supports(provider.CAPABILITY_AUDIO_SPEECH):
		speech, err := provider.NewSpeech(entry.Name, options)
		if err != nil {
			return nil, err
		}
		lister, _ := speech.(core.ModelLister)
		return lister, nil
	default:
		return nil, nil
	}
}

// joinCapabilities renders an entry's capability list for error messages.
func joinCapabilities(capabilities []provider.Capability) string {
	if len(capabilities) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		names = append(names, string(capability))
	}
	return strings.Join(names, ", ")
}

// dumpCatalog prints the provider's ModelSpec list. It prefers the live
// upstream catalog (core.ModelLister) and falls back to the bundled static
// catalog when the provider does not implement the live port or the live
// call fails (offline, bad key). A nil lister means no client on this entry
// can enumerate models, so only the static catalog is printed. The source is
// reported on stderr so the stdout list stays clean for piping.
func dumpCatalog(
	ctx context.Context,
	errOut, out io.Writer,
	lister core.ModelLister,
	label string,
	static []core.ModelSpec,
) error {
	specs := static
	source := "static"
	if lister != nil {
		if live, err := lister.ListModels(ctx); err != nil {
			fmt.Fprintf(errOut, "[provider] %s: live catalog unavailable, using static (%v)\n", label, err)
		} else {
			specs = live
			source = "live"
		}
	}
	if len(specs) == 0 {
		fmt.Fprintf(out, "%s: (empty catalog)\n", label)
		return nil
	}
	fmt.Fprintf(out, "%s catalog (%d models, %s):\n", label, len(specs), source)
	for _, s := range specs {
		fmt.Fprintf(out, "  %-40s family=%-18s reasoning=%v ctx=%d max=%d\n",
			s.ID, s.Family, s.Reasoning, s.ContextWindow, s.MaxTokens)
	}
	return nil
}
