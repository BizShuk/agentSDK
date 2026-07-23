package cmd

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/spf13/cobra"
)

// DEFAULT_WIZARD_OUT is where the wizard writes when -o is omitted.
const DEFAULT_WIZARD_OUT = "agent.yaml"

// NewWizardCommand returns the `wizard` subcommand (alias `w`): an
// interactive walk through the configuration, one stage per build-pipeline
// stage, ending in a written config file.
//
// The stage sequence deliberately mirrors agent's assembly order, so a
// user who answers the questions once has also learned how the pieces fit
// together. Which stages appear depends on the tier — one-shot asks five
// questions, full asks all of them.
//
// Every choice list comes from spec (or agent.ProviderChoices for the one
// list that needs compile-time knowledge). The wizard holds no vocabulary
// of its own: adding a reasoning strategy or a provider must not require
// touching this file.
func NewWizardCommand() *cobra.Command {
	var (
		out     string
		yes     bool
		tier    string
		edit    string
		force   bool
		printGo bool
		listKey string
	)

	cmd := &cobra.Command{
		Use:     "wizard",
		Aliases: []string{"w"},
		Short:   "Build an agent config interactively, stage by stage",
		Long: strings.TrimSpace(`
wizard walks the agent configuration one stage at a time, in the same
order the builder assembles it. Press Enter at any prompt to take the
shown default.

Examples:

  wizard                          # interactive, writes ./agent.yaml
  wizard -y --tier full           # all defaults, no questions
  wizard --edit agent.yaml        # use an existing config as the defaults
  wizard -o - --print-go          # print to stdout, plus the Go equivalent
  wizard --list reasoning.style   # show the choices for one field
`),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			stdout := cmd.OutOrStdout()

			if listKey != "" {
				return listChoices(stdout, listKey)
			}

			base := agent.Config{}
			if edit != "" {
				loaded, err := agent.LoadFile(edit)
				if err != nil {
					return err
				}
				base = loaded
			}
			if tier != "" {
				base.Tier = tier
			}

			w := &wizard{
				in:  bufio.NewScanner(cmd.InOrStdin()),
				out: cmd.ErrOrStderr(), // prompts on stderr so -o - stays clean
				yes: yes,
			}
			cfg, err := w.run(base)
			if err != nil {
				return err
			}

			// Validate before writing. A wizard that emits a config the
			// builder rejects has wasted the user's answers.
			if _, err := cfg.Prepare(); err != nil {
				return fmt.Errorf("the answers do not form a valid config: %w", err)
			}

			if out == "-" {
				body, err := agent.Marshal(cfg, agent.FORMAT_YAML)
				if err != nil {
					return err
				}
				fmt.Fprint(stdout, string(body))
			} else {
				if err := agent.SaveFile(out, cfg, force); err != nil {
					return err
				}
				fmt.Fprintf(w.out, "\nwrote %s\n", out)
			}
			if printGo {
				fmt.Fprintln(stdout)
				fmt.Fprint(stdout, goLiteral(cfg))
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVarP(&out, "out", "o", DEFAULT_WIZARD_OUT, `Output path; "-" writes to stdout.`)
	f.BoolVarP(&yes, "yes", "y", false, "Take every default; ask nothing.")
	f.StringVar(&tier, "tier", "", "Preselect the tier: oneshot | basic | standard | full.")
	f.StringVar(&edit, "edit", "", "Load an existing config and use it as the defaults.")
	f.BoolVar(&force, "force", false, "Overwrite the output file if it exists.")
	f.BoolVar(&printGo, "print-go", false, "Also print the equivalent Go literal.")
	f.StringVar(&listKey, "list", "", "Print the choices for one field and exit (e.g. reasoning.style).")
	return cmd
}

// wizard carries the prompt loop's I/O and the non-interactive switch.
type wizard struct {
	in  *bufio.Scanner
	out io.Writer
	yes bool
}

// run walks the stages. Each stage may read and write cfg; the tier stage
// runs first because every later stage's visibility depends on it.
func (w *wizard) run(cfg agent.Config) (agent.Config, error) {
	// --- stage 0: tier ---
	w.section("0/9  tier — how much of the SDK to wire")
	cfg.Tier = w.choose("tier", spec.TierChoices(), orDefault(cfg.Tier, spec.DEFAULT_TIER))
	rank, ok := spec.Rank(cfg.Tier)
	if !ok {
		return cfg, fmt.Errorf("unknown tier %q", cfg.Tier)
	}

	// Expand now so every later stage offers the tier's own defaults
	// rather than empty strings.
	expanded, err := cfg.Expand()
	if err != nil {
		return cfg, err
	}
	cfg = expanded

	// --- stage 1: model ---
	w.section("1/9  model — which provider answers")
	cfg.Model.Provider = w.choose("provider", agent.ProviderChoices(), cfg.Model.Provider)
	cfg.Model.Name = w.ask("model id (empty = the adapter's flagship default)", cfg.Model.Name)
	cfg.Persona = w.ask("persona (fixed system identity, optional)", cfg.Persona)

	// --- stage 2: reasoning (orthogonal to tier, so always asked) ---
	w.section("2/9  reasoning — which strategy runs, and which are registered")
	cfg.Reasoning.Style = w.choose("style", spec.StyleChoices(), cfg.Reasoning.Style)
	cfg.Reasoning.Enable = w.chooseMulti(
		"also register (for mid-run switching; the selected style is always included)",
		spec.StyleChoices(), cfg.Reasoning.Enable)
	if !slices.Contains(cfg.Reasoning.Enable, cfg.Reasoning.Style) {
		cfg.Reasoning.Enable = append(cfg.Reasoning.Enable, cfg.Reasoning.Style)
	}

	// --- stage 3: tools ---
	if rank >= rankOf(spec.TIER_STANDARD) && cfg.Tools != nil {
		w.section("3/9  tools — what the model can do")
		cfg.Tools.Builtin = w.chooseMulti("built-in tools (none selected = all)",
			spec.VariantChoices("tools.builtin"), cfg.Tools.Builtin)
	}

	// --- stage 4: safety ---
	if cfg.Safety != nil {
		w.section("4/9  safety — who approves, and what may be touched")
		cfg.Safety.Mode = w.choose("mode", spec.VariantChoices("safety.mode"), cfg.Safety.Mode)
		cfg.Safety.Sandbox = w.confirm("sandbox path and command arguments", cfg.Safety.Sandbox)
		cfg.Safety.Deny = w.askList("deny rules, comma separated (e.g. bash(sudo:*))", cfg.Safety.Deny)
		cfg.Safety.Ask = w.askList("ask rules, comma separated", cfg.Safety.Ask)
	}

	// --- stage 5: prompt ---
	if cfg.Prompt != nil {
		w.section("5/9  prompt — what goes into the context window")
		cfg.Prompt.Sources = w.chooseMulti("content sources",
			spec.VariantChoices("prompt.sources"), cfg.Prompt.Sources)
		cfg.Prompt.ProjectDir = w.ask("project harness directory", cfg.Prompt.ProjectDir)
	}

	// --- stage 6: skills and subagents ---
	if cfg.Subagents != nil {
		w.section("6/9  subagents — delegation")
		cfg.Subagents.MaxTurns = w.askInt("max turns per delegated run", cfg.Subagents.MaxTurns)
		cfg.Subagents.MaxDepth = w.askInt("max nesting depth", cfg.Subagents.MaxDepth)
	}

	// --- stage 7: memory ---
	if cfg.Memory != nil {
		w.section("7/9  memory — persistence and history")
		cfg.Memory.Store = w.choose("state store", spec.VariantChoices("memory.store"), cfg.Memory.Store)
		cfg.Memory.Compaction = w.choose("compaction", spec.VariantChoices("memory.compaction"), cfg.Memory.Compaction)
	}

	// --- stage 8: output and limits ---
	w.section("8/9  output and limits")
	if cfg.Output != nil {
		cfg.Output.Format = w.choose("format", spec.VariantChoices("output.format"), cfg.Output.Format)
	}
	cfg.Limits.MaxTurns = w.askInt("max turns per run", cfg.Limits.MaxTurns)
	cfg.Limits.Autonomy = w.choose("autonomy", spec.VariantChoices("limits.autonomy"), cfg.Limits.Autonomy)

	// Name comes last because it is the only answer with no sensible
	// default, and asking for it first would greet the user with a blank.
	if cfg.Memory != nil && cfg.Memory.Store == spec.MEMORY_STORE_FILE {
		cfg.Name = w.ask("application name (resolves ~/.config/<name>)", orDefault(cfg.Name, "my-agent"))
	}

	// --- stage 9: review ---
	// Suppressed under -y: a non-interactive run has nothing to review,
	// and echoing the config to stderr would be noise in a script.
	if !w.yes {
		w.section("9/9  review")
		body, err := agent.Marshal(cfg, agent.FORMAT_YAML)
		if err != nil {
			return cfg, err
		}
		fmt.Fprintln(w.out, string(body))
	}
	return cfg, nil
}

// --- prompting ---

func (w *wizard) section(title string) {
	if w.yes {
		return
	}
	fmt.Fprintf(w.out, "\n=== %s ===\n", title)
}

// readLine returns the trimmed next line, or "" at EOF. EOF is treated as
// "take the default" so a piped run behaves like -y rather than erroring
// halfway through.
func (w *wizard) readLine() string {
	if !w.in.Scan() {
		w.yes = true
		return ""
	}
	return strings.TrimSpace(w.in.Text())
}

func (w *wizard) ask(label, current string) string {
	if w.yes {
		return current
	}
	if current != "" {
		fmt.Fprintf(w.out, "%s [%s]: ", label, current)
	} else {
		fmt.Fprintf(w.out, "%s []: ", label)
	}
	if line := w.readLine(); line != "" {
		return line
	}
	return current
}

func (w *wizard) askInt(label string, current int) int {
	got := w.ask(label, strconv.Itoa(current))
	n, err := strconv.Atoi(got)
	if err != nil {
		return current
	}
	return n
}

func (w *wizard) askList(label string, current []string) []string {
	got := w.ask(label, strings.Join(current, ", "))
	if strings.TrimSpace(got) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(got, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (w *wizard) confirm(label string, current bool) bool {
	def := "y"
	if !current {
		def = "n"
	}
	got := strings.ToLower(w.ask(label+" (y/n)", def))
	return strings.HasPrefix(got, "y")
}

// choose renders a numbered menu and returns the selected value. Enter
// takes the current value; the current value is the tier's default on a
// fresh run, or the loaded config's value under --edit.
func (w *wizard) choose(label string, choices []agent.Choice, current string) string {
	if current == "" {
		current = spec.DefaultOf(choices)
	}
	if w.yes || len(choices) == 0 {
		return current
	}
	fmt.Fprintf(w.out, "\n%s:\n", label)
	for i, c := range choices {
		mark := " "
		if c.Value == current {
			mark = "*"
		}
		fmt.Fprintf(w.out, " %s %d) %-18s %s\n", mark, i+1, c.Value, c.Note)
	}
	fmt.Fprintf(w.out, "select [%s]: ", current)

	line := w.readLine()
	if line == "" {
		return current
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(choices) {
		return choices[n-1].Value
	}
	// Accept the value typed out in full, too.
	for _, c := range choices {
		if strings.EqualFold(c.Value, line) {
			return c.Value
		}
	}
	fmt.Fprintf(w.out, "  (unrecognized %q, keeping %s)\n", line, current)
	return current
}

// chooseMulti takes a comma-separated list of numbers or values. An empty
// answer keeps the current selection, which is how "press Enter through
// everything" produces the tier's defaults.
func (w *wizard) chooseMulti(label string, choices []agent.Choice, current []string) []string {
	if w.yes || len(choices) == 0 {
		return current
	}
	fmt.Fprintf(w.out, "\n%s:\n", label)
	for i, c := range choices {
		mark := " "
		if slices.Contains(current, c.Value) {
			mark = "*"
		}
		fmt.Fprintf(w.out, " %s %d) %-18s %s\n", mark, i+1, c.Value, c.Note)
	}
	fmt.Fprintf(w.out, "select, comma separated [%s]: ", strings.Join(current, ","))

	line := w.readLine()
	if line == "" {
		return current
	}
	var out []string
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n >= 1 && n <= len(choices) {
			out = append(out, choices[n-1].Value)
			continue
		}
		for _, c := range choices {
			if strings.EqualFold(c.Value, part) {
				out = append(out, c.Value)
			}
		}
	}
	return out
}

// --- helpers ---

func rankOf(tier string) int {
	r, _ := spec.Rank(tier)
	return r
}

func orDefault(got, fallback string) string {
	if got != "" {
		return got
	}
	return fallback
}

// listChoices prints one field's candidates — the same metadata the
// interactive menus render, for scripting and for `--help`-style browsing.
func listChoices(out io.Writer, key string) error {
	var choices []agent.Choice
	switch key {
	case "tier":
		choices = spec.TierChoices()
	case "reasoning.style", "reasoning.enable":
		choices = spec.StyleChoices()
	case "model.provider":
		choices = agent.ProviderChoices()
	default:
		choices = spec.VariantChoices(key)
	}
	if len(choices) == 0 {
		return fmt.Errorf("unknown field %q (try: tier, model.provider, reasoning.style, %s)",
			key, strings.Join(spec.VariantKeys(), ", "))
	}
	for _, c := range choices {
		mark := " "
		if c.Default {
			mark = "*"
		}
		fmt.Fprintf(out, "%s %-18s %s\n", mark, c.Value, c.Note)
	}
	return nil
}

// goLiteral renders the config as Go source, for applications that would
// rather paste a literal into main.go than carry a config file. It is the
// one place Choice and Option meet: the same decisions, two carriers.
func goLiteral(cfg agent.Config) string {
	var b strings.Builder
	b.WriteString("app.Main(agent.MustNew(agent.Config{\n")
	fmt.Fprintf(&b, "\tName: %q,\n", cfg.Name)
	fmt.Fprintf(&b, "\tTier: %q,\n", cfg.Tier)
	if cfg.Persona != "" {
		fmt.Fprintf(&b, "\tPersona: %q,\n", cfg.Persona)
	}
	fmt.Fprintf(&b, "\tModel: agent.Model{Provider: %q", cfg.Model.Provider)
	if cfg.Model.Name != "" {
		fmt.Fprintf(&b, ", Name: %q", cfg.Model.Name)
	}
	b.WriteString("},\n")
	fmt.Fprintf(&b, "\tReasoning: agent.Reasoning{Style: %q},\n", cfg.Reasoning.Style)
	fmt.Fprintf(&b, "\tLimits: agent.Limits{MaxTurns: %d, Autonomy: %q},\n",
		cfg.Limits.MaxTurns, cfg.Limits.Autonomy)
	if cfg.Safety != nil {
		fmt.Fprintf(&b, "\tSafety: &agent.Safety{Mode: %q, Sandbox: %v},\n",
			cfg.Safety.Mode, cfg.Safety.Sandbox)
	}
	b.WriteString("}))\n")
	return b.String()
}
