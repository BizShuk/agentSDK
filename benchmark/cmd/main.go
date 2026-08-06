// Command cmd runs the benchmark flow for one provider-model pair chosen by
// flags, or sweeps a provider's entire bundled catalog with -model all.
// Results land in benchmark/pkg/<pair-slug>/tmp/<session-id>/.
//
//	go run ./benchmark/cmd -list
//	go run ./benchmark/cmd -provider minimax -model MiniMax-M3
//	go run ./benchmark/cmd -provider google -model gemini-2.5-pro -capabilities chat
//	go run ./benchmark/cmd -provider elevenlabs -capabilities speech -model eleven_v3
//	go run ./benchmark/cmd -provider elevenlabs -model all
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/bizshuk/agentsdk/benchmark"
	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/all"
)

// MODEL_ALL sweeps every bundled catalog model instead of naming one.
const MODEL_ALL = "all"

var capabilityOrder = benchmark.BenchmarkCapabilities()

func main() {
	providerName := flag.String("provider", provider.DEFAULT_NAME,
		"provider name; see -list")
	model := flag.String("model", "",
		"model id from the provider catalog (off-catalog ids warn), or \"all\" to sweep the catalog; empty uses adapter defaults")
	capabilities := flag.String("capabilities", "",
		"comma-separated benchmark capabilities (chat,image,speech,transcribe,video,music); empty derives them from provider and model metadata; explicitly unsupported capabilities run and are recorded as FAIL")
	list := flag.Bool("list", false,
		"list registered providers and catalog models with runnable benchmark capabilities, then exit")
	flag.Parse()

	if *list {
		printList()
		return
	}

	entry, ok := provider.Lookup(*providerName)
	if !ok {
		fatal(fmt.Errorf("unknown provider %q (registered: %s)",
			*providerName, strings.Join(provider.Names(), ", ")))
	}

	if *model == MODEL_ALL {
		selected, err := selectCapabilities(entry, nil, *capabilities)
		if err != nil {
			fatal(err)
		}
		if err := sweepCatalog(context.Background(), entry, selected); err != nil {
			fatal(err)
		}
		return
	}

	spec := findCatalogSpec(entry, *model)
	selected, err := selectCapabilities(entry, spec, *capabilities)
	if err != nil {
		fatal(err)
	}
	cases := casesForSelection(entry, spec, selected)
	if spec == nil && *model != "" &&
		!slices.Contains(selected, provider.CAPABILITY_CHAT) {
		cases = pinMediaModel(*model, cases)
	}
	if len(cases) == 0 {
		fatal(fmt.Errorf("provider %s model %s has no runnable benchmark cases",
			entry.Name, displayModel(*model)))
	}

	target := benchmark.Target{Provider: entry.Name, Model: *model}
	if err := benchmark.RunPair(context.Background(), target, cases); err != nil {
		fatal(err)
	}
}

// sweepCatalog runs every bundled model on the intersection of its declared
// runnable capabilities and the selected benchmark capabilities.
func sweepCatalog(ctx context.Context, entry provider.Entry, selected []provider.Capability) error {
	specs := benchmark.CatalogSpecs(entry.Name)
	if len(specs) == 0 {
		return fmt.Errorf("provider %s ships no catalog to sweep", entry.Name)
	}
	for _, spec := range specs {
		var capabilities []provider.Capability
		for _, capability := range benchmark.RunnableCapabilities(entry, spec) {
			if slices.Contains(selected, capability) {
				capabilities = append(capabilities, capability)
			}
		}
		cases := casesForSelection(entry, &spec, capabilities)
		if len(cases) == 0 {
			fmt.Printf("skip %s: no runnable benchmark capability\n", spec.ID)
			continue
		}
		target := benchmark.Target{Provider: entry.Name, Model: spec.ID}
		if err := benchmark.RunPair(ctx, target, cases); err != nil {
			fmt.Fprintf(os.Stderr, "benchmark: %s: %v\n", spec.ID, err)
		}
	}
	return nil
}

// selectCapabilities derives an empty selection from model metadata when a
// catalog model is known, otherwise from the provider entry. An explicit list
// is vocabulary-checked only so unsupported requests still produce a result.
func selectCapabilities(entry provider.Entry, spec *provider.ModelSpec, flagValue string) ([]provider.Capability, error) {
	if strings.TrimSpace(flagValue) == "" {
		if spec != nil {
			return benchmark.RunnableCapabilities(entry, *spec), nil
		}
		var out []provider.Capability
		for _, capability := range capabilityOrder {
			if entry.Supports(capability) {
				out = append(out, capability)
			}
		}
		return out, nil
	}

	var out []provider.Capability
	for raw := range strings.SplitSeq(flagValue, ",") {
		capability := provider.Capability(strings.TrimSpace(strings.ToLower(raw)))
		if !slices.Contains(capabilityOrder, capability) {
			return nil, fmt.Errorf("unknown capability %q (known: %s)", raw, joinCapabilities(capabilityOrder))
		}
		if !slices.Contains(out, capability) {
			out = append(out, capability)
		}
	}
	return out, nil
}

// casesForSelection applies catalog model requirements to supported
// capabilities. Explicit unsupported capabilities keep their base cases so
// the benchmark records the typed provider failure.
func casesForSelection(entry provider.Entry, spec *provider.ModelSpec, selected []provider.Capability) []benchmark.Case {
	if spec == nil {
		var out []benchmark.Case
		for _, capability := range selected {
			out = append(out, benchmark.CasesForCapability(capability)...)
		}
		return out
	}

	modelCases := benchmark.CasesForModel(entry, *spec)
	var out []benchmark.Case
	for _, capability := range selected {
		if entry.Supports(capability) && spec.Supports(capability) {
			for _, testCase := range modelCases {
				if testCase.Capability == capability {
					out = append(out, testCase)
				}
			}
			continue
		}
		cases := benchmark.CasesForCapability(capability)
		if capability != provider.CAPABILITY_CHAT {
			cases = benchmark.WithModel(spec.ID, cases)
		}
		out = append(out, cases...)
	}
	return out
}

func pinMediaModel(model string, cases []benchmark.Case) []benchmark.Case {
	out := slices.Clone(cases)
	for i := range out {
		if out[i].Capability != provider.CAPABILITY_CHAT && out[i].Model == "" {
			out[i].Model = model
		}
	}
	return out
}

func findCatalogSpec(entry provider.Entry, model string) *provider.ModelSpec {
	if model == "" {
		return nil
	}
	for _, spec := range benchmark.CatalogSpecs(entry.Name) {
		if spec.ID == model {
			matched := spec
			return &matched
		}
	}
	return nil
}

func printList() {
	for _, entry := range provider.Entries() {
		var supported []provider.Capability
		for _, capability := range capabilityOrder {
			if entry.Supports(capability) {
				supported = append(supported, capability)
			}
		}
		fmt.Printf("%-12s capabilities: %s\n", entry.Name, joinCapabilities(supported))
		for _, spec := range benchmark.CatalogSpecs(entry.Name) {
			capabilities := joinCapabilities(benchmark.RunnableCapabilities(entry, spec))
			if capabilities == "" {
				capabilities = "-"
			}
			fmt.Printf("  %-42s %s\n", spec.ID, capabilities)
		}
	}
}

func joinCapabilities(capabilities []provider.Capability) string {
	names := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		names = append(names, string(capability))
	}
	return strings.Join(names, ",")
}

func displayModel(model string) string {
	if model == "" {
		return "(adapter default)"
	}
	return model
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "benchmark: %v\n", err)
	os.Exit(1)
}
