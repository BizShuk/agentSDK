// Package cmd hosts the cobra subcommands mounted by the root agentsdk
// binary. provider.go wires the "provider" subcommand — a thin smoke-test
// CLI that calls core.Provider.Generate / Stream directly, with no Agent,
// Engine, or harness in the path.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	_ "github.com/bizshuk/agentsdk/provider/all"
	"github.com/bizshuk/agentsdk/provider"
	gosdkconfig "github.com/bizshuk/gosdk/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	ProviderName       string
	ProviderModel      string
	ProviderAPIKey     string
	ProviderBaseURL    string
	ProviderSystem     string
	ProviderMaxTokens  int
	ProviderStream     bool
	ProviderAsJSON     bool
	ProviderListModels bool
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
It calls core.Provider.Generate (or Stream with --stream) directly —
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
			fmt.Fprintln(out, strings.Join(registry.Names(), ", "))
			return nil
		}

		entry, ok := registry.Lookup(ProviderName)
		if !ok {
			return fmt.Errorf("unknown provider %q (registered: %s)",
				ProviderName, strings.Join(registry.Names(), ", "))
		}
		label := entry.Name

		prov, err := registry.New(ProviderName, registry.Options{
			Model:     ProviderModel,
			APIKey:    ProviderAPIKey,
			BaseURL:   ProviderBaseURL,
			LookupEnv: envLookup,
		})
		if err != nil {
			return err
		}

		if ProviderListModels {
			return dumpCatalog(cmd.Context(), errOut, out, prov, label)
		}

		prompt := strings.TrimSpace(strings.Join(args, " "))
		if prompt == "" {
			return fmt.Errorf("prompt is required (or pass --list-models / --list-providers)")
		}

		req := buildRequest(prompt, ProviderSystem, ProviderMaxTokens)

		fmt.Fprintf(errOut, "[provider] %s | model=%s | stream=%v\n",
			label, effectiveModel(prov), ProviderStream)

		if ProviderStream {
			return runStream(cmd.Context(), prov, req, out, ProviderAsJSON)
		}
		return runGenerate(cmd.Context(), prov, req, out, ProviderAsJSON)
	},
}

func init() {
	flags := ProviderCmd.Flags()
	flags.StringVar(&ProviderName, "provider", "minimax",
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
}

// ResetFlags resets ProviderCmd flag state for clean test execution.
func ResetFlags() {
	ProviderName = "minimax"
	ProviderModel = ""
	ProviderAPIKey = ""
	ProviderBaseURL = ""
	ProviderSystem = ""
	ProviderMaxTokens = 0
	ProviderStream = false
	ProviderAsJSON = false
	ProviderListModels = false

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

// envLookup returns the merged viper value for `key`, which transparently
// fans out through: .env / config files (loaded by bootGosdkConfig) →
// OS env (via BindEnv above). Empty when none are set; callers propagate
// that to the adapter which then applies its own default.
//
// Viper normalizes config-file keys to lowercase, so we look up the
// lower-cased form while still using uppercase env-var names in the
// registry (those ARE the literal env-var names, so they stay upper-case).
func envLookup(key string) string { return viper.GetString(strings.ToLower(key)) }

// effectiveModel returns the model the provider was built with. Falls back
// to its ID when the adapter doesn't expose a separate accessor.
func effectiveModel(p core.Provider) string {
	type named interface{ Name() string }
	if n, ok := p.(named); ok {
		return n.Name()
	}
	return p.ID()
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

func runStream(ctx context.Context, prov core.Provider, req core.ModelRequest,
	out io.Writer, asJSON bool,
) error {
	ch, err := prov.Stream(ctx, req)
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	if asJSON {
		enc := json.NewEncoder(out)
		for c := range ch {
			if err := enc.Encode(c); err != nil {
				return err
			}
		}
		return nil
	}
	var sb strings.Builder
	for c := range ch {
		if c.Done {
			continue
		}
		if c.Kind == core.PART_KIND_PLAIN_TEXT && c.Text != "" {
			sb.WriteString(c.Text)
			fmt.Fprint(out, c.Text)
		}
	}
	fmt.Fprintln(out)
	return nil
}

// dumpCatalog prints the provider's ModelSpec list. It prefers the live
// upstream catalog (core.ModelLister) and falls back to the bundled static
// catalog when the provider does not implement the live port or the live
// call fails (offline, bad key). The source is reported on stderr so the
// stdout list stays clean for piping.
func dumpCatalog(ctx context.Context, errOut, out io.Writer, prov core.Provider, label string) error {
	specs := prov.Models()
	source := "static"
	if lister, ok := prov.(core.ModelLister); ok {
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
