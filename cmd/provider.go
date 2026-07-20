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
	"sort"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	anthropicprovider "github.com/bizshuk/agentsdk/provider/anthropic"
	googleprovider "github.com/bizshuk/agentsdk/provider/google"
	grokprovider "github.com/bizshuk/agentsdk/provider/grok"
	minimaxprovider "github.com/bizshuk/agentsdk/provider/minimax"
	ollamaprovider "github.com/bizshuk/agentsdk/provider/ollama"
	"github.com/spf13/cobra"
)

// NewProviderCommand returns the cobra subcommand:
//
//	provider [flags] <prompt...>
//
// It is mounted by main.go as a child of the root cobra command. The CLI
// intentionally bypasses runtime.Engine / harness packages so it stays
// useful as a wire-format smoke test: any provider-side regression
// (auth header, DTO translate, SSE parser) is exposed here without
// requiring a full agentic loop.
func NewProviderCommand() *cobra.Command {
	var (
		providerName string
		model        string
		apiKey       string
		baseURL      string
		system       string
		maxTokens    int
		stream       bool
		asJSON       bool
		listModels   bool
	)

	cmd := &cobra.Command{
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

			if listProviders, _ := cmd.Flags().GetBool("list-providers"); listProviders {
				fmt.Fprintln(out, strings.Join(registeredProviders(), ", "))
				return nil
			}

			factory, label, err := resolveProvider(providerName)
			if err != nil {
				return err
			}

			prov, err := factory(factoryOptions{
				Model:   model,
				APIKey:  apiKey,
				BaseURL: baseURL,
			})
			if err != nil {
				return fmt.Errorf("%s provider: %w", label, err)
			}

			if listModels {
				return dumpCatalog(cmd.Context(), errOut, out, prov, label)
			}

			prompt := strings.TrimSpace(strings.Join(args, " "))
			if prompt == "" {
				return fmt.Errorf("prompt is required (or pass --list-models / --list-providers)")
			}

			req := buildRequest(prompt, system, maxTokens)

			fmt.Fprintf(errOut, "[provider] %s | model=%s | stream=%v\n",
				label, effectiveModel(prov), stream)

			if stream {
				return runStream(cmd.Context(), prov, req, out, asJSON)
			}
			return runGenerate(cmd.Context(), prov, req, out, asJSON)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&providerName, "provider", "minimax",
		"Provider family (minimax | anthropic | google | grok | ollama; case-insensitive).")
	flags.StringVarP(&model, "model", "m", "",
		"Model id (alias -m); empty = adapter flagship default. "+
			"Use --list-models to see the provider's catalog.")
	flags.StringVar(&apiKey, "api-key", "",
		"API key override; empty = adapter reads its own env "+
			"(MINIMAX_API_KEY / ANTHROPIC_API_KEY / GOOGLE_API_KEY / XAI_API_KEY / OPENAI_API_KEY; "+
			"ollama is keyless by default).")
	flags.StringVar(&baseURL, "base-url", "",
		"Base URL override; empty = adapter reads its own env / default.")
	flags.StringVar(&system, "system", "",
		"Optional system message prepended to the prompt.")
	flags.IntVar(&maxTokens, "max-tokens", 0,
		"max_tokens for the request; 0 = adapter default.")
	flags.BoolVar(&stream, "stream", false,
		"Use SSE Stream instead of blocking Generate.")
	flags.BoolVar(&asJSON, "json", false,
		"Print the full ModelResult / chunk stream as JSON lines.")
	flags.BoolVar(&listModels, "list-models", false,
		"Print the provider's static catalog and exit.")
	flags.Bool("list-providers", false,
		"Print the registered provider names and exit.")

	return cmd
}

// ---------------------------------------------------------------------------
// provider registry
// ---------------------------------------------------------------------------

// factoryOptions are the resolved credential / endpoint overrides from
// cobra flags. Empty fields let the adapter apply its own env fallback.
type factoryOptions struct {
	Model   string
	APIKey  string
	BaseURL string
}

// factory returns a core.Provider already wired with the requested
// credential and model. Each adapter reads its own env when the
// corresponding field is empty (e.g. minimax reads MINIMAX_API_KEY,
// anthropic reads ANTHROPIC_API_KEY).
type factory func(o factoryOptions) (core.Provider, error)

// registry maps the --provider flag value to its adapter. Names are
// case-insensitive at the boundary (see resolveProvider).
var registry = map[string]factory{
	"minimax": func(o factoryOptions) (core.Provider, error) {
		opts := []minimaxprovider.Option{}
		if o.Model != "" {
			opts = append(opts, minimaxprovider.WithModel(o.Model))
		}
		if o.APIKey != "" {
			opts = append(opts, minimaxprovider.WithAPIKey(o.APIKey))
		}
		if o.BaseURL != "" {
			opts = append(opts, minimaxprovider.WithBaseURL(o.BaseURL))
		}
		return minimaxprovider.New(opts...)
	},
	"anthropic": func(o factoryOptions) (core.Provider, error) {
		opts := []anthropicprovider.Option{}
		if o.Model != "" {
			opts = append(opts, anthropicprovider.WithModel(o.Model))
		}
		if o.APIKey != "" {
			opts = append(opts, anthropicprovider.WithAPIKey(o.APIKey))
		}
		if o.BaseURL != "" {
			opts = append(opts, anthropicprovider.WithBaseURL(o.BaseURL))
		}
		return anthropicprovider.New(opts...)
	},
	"google": func(o factoryOptions) (core.Provider, error) {
		opts := []googleprovider.Option{}
		if o.Model != "" {
			opts = append(opts, googleprovider.WithModel(o.Model))
		}
		if o.APIKey != "" {
			opts = append(opts, googleprovider.WithAPIKey(o.APIKey))
		}
		if o.BaseURL != "" {
			opts = append(opts, googleprovider.WithBaseURL(o.BaseURL))
		}
		return googleprovider.New(opts...)
	},
	"grok": func(o factoryOptions) (core.Provider, error) {
		opts := []grokprovider.Option{}
		if o.Model != "" {
			opts = append(opts, grokprovider.WithModel(o.Model))
		}
		if o.APIKey != "" {
			opts = append(opts, grokprovider.WithAPIKey(o.APIKey))
		}
		if o.BaseURL != "" {
			opts = append(opts, grokprovider.WithBaseURL(o.BaseURL))
		}
		return grokprovider.New(opts...)
	},
	"ollama": func(o factoryOptions) (core.Provider, error) {
		opts := []ollamaprovider.Option{}
		if o.Model != "" {
			opts = append(opts, ollamaprovider.WithModel(o.Model))
		}
		if o.APIKey != "" {
			opts = append(opts, ollamaprovider.WithAPIKey(o.APIKey))
		}
		if o.BaseURL != "" {
			opts = append(opts, ollamaprovider.WithBaseURL(o.BaseURL))
		}
		return ollamaprovider.New(opts...)
	},
}

func registeredProviders() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func resolveProvider(name string) (factory, string, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = "minimax"
	}
	f, ok := registry[key]
	if !ok {
		return nil, "", fmt.Errorf("unknown provider %q (registered: %s)",
			name, strings.Join(registeredProviders(), ", "))
	}
	return f, key, nil
}

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
	out io.Writer, asJSON bool) error {
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
	out io.Writer, asJSON bool) error {
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