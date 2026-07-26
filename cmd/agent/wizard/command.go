package wizard

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/utils/agentconfig"
	"github.com/spf13/cobra"
)

// DEFAULT_WIZARD_OUT is where the wizard writes when -o is omitted.
const DEFAULT_WIZARD_OUT = "agent.yaml"

var (
	out     string
	yes     bool
	tier    string
	edit    string
	force   bool
	printGo bool
	listKey string
)

// WizardCmd is the package-level `wizard` subcommand (alias `w`).
var WizardCmd = &cobra.Command{
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
	RunE:          runWizardCmd,
}

func init() {
	f := WizardCmd.Flags()
	f.StringVarP(&out, "out", "o", DEFAULT_WIZARD_OUT, `Output path; "-" writes to stdout.`)
	f.BoolVarP(&yes, "yes", "y", false, "Take every default; ask nothing.")
	f.StringVar(&tier, "tier", "", "Preselect the tier: oneshot | basic | standard | full.")
	f.StringVar(&edit, "edit", "", "Load an existing config and use it as the defaults.")
	f.BoolVar(&force, "force", false, "Overwrite the output file if it exists.")
	f.BoolVar(&printGo, "print-go", false, "Also print the equivalent Go literal.")
	f.StringVar(&listKey, "list", "", "Print the choices for one field and exit (e.g. reasoning.style).")
}

// ResetFlags resets flag values before execution in multi-run tests.
func ResetFlags() {
	out = DEFAULT_WIZARD_OUT
	yes = false
	tier = ""
	edit = ""
	force = false
	printGo = false
	listKey = ""
}

func runWizardCmd(cmd *cobra.Command, _ []string) error {
	stdout := cmd.OutOrStdout()

	if listKey != "" {
		return listChoices(stdout, listKey)
	}

	base := agent.Config{}
	if edit != "" {
		loaded, err := agentconfig.LoadFile(edit)
		if err != nil {
			return err
		}
		base = loaded
	}
	if tier != "" {
		base.Tier = tier
	}

	isTTY := false
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		isTTY = true
	}

	w := &wizard{
		in:    bufio.NewScanner(cmd.InOrStdin()),
		out:   cmd.ErrOrStderr(), // prompts on stderr so -o - stays clean
		yes:   yes,
		isTTY: isTTY,
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
		body, err := agentconfig.Marshal(cfg, agentconfig.FORMAT_YAML)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, string(body))
	} else {
		if err := agentconfig.SaveFile(out, cfg, force); err != nil {
			return err
		}
		absOut := out
		if abs, err := filepath.Abs(out); err == nil {
			absOut = abs
		}
		fmt.Fprintf(w.out, "\nwrote %s\n", absOut)
	}
	if printGo {
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, goLiteral(cfg))
	}
	return nil
}
