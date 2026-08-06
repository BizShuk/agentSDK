// Command cmd runs the benchmark flow for one provider-model pair chosen by
// flags, or sweeps a provider's entire DefaultCatalog with -model all.
// Results land in the same place as the pinned packages:
// benchmark/pkg/<pair-slug>/tmp/<session-id>/.
//
//	go run ./benchmark/cmd -list
//	go run ./benchmark/cmd -provider minimax -model MiniMax-M3
//	go run ./benchmark/cmd -provider google -model gemini-2.5-pro -kinds chat
//	go run ./benchmark/cmd -provider elevenlabs -kinds speech -model eleven_v3
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

// MODEL_ALL sweeps every DefaultCatalog model instead of naming one.
const MODEL_ALL = "all"

// kindOrder fixes both the -kinds vocabulary and the case execution order.
var kindOrder = []benchmark.Kind{
	benchmark.KIND_CHAT,
	benchmark.KIND_IMAGE,
	benchmark.KIND_SPEECH,
	benchmark.KIND_TRANSCRIBE,
	benchmark.KIND_VIDEO,
	benchmark.KIND_MUSIC,
}

var caseSets = map[benchmark.Kind]func() []benchmark.Case{
	benchmark.KIND_CHAT:       benchmark.ChatCases,
	benchmark.KIND_IMAGE:      benchmark.ImageCases,
	benchmark.KIND_SPEECH:     benchmark.SpeechCases,
	benchmark.KIND_TRANSCRIBE: benchmark.TranscribeCases,
	benchmark.KIND_VIDEO:      benchmark.VideoCases,
	benchmark.KIND_MUSIC:      benchmark.MusicCases,
}

var kindCapability = map[benchmark.Kind]provider.Capability{
	benchmark.KIND_CHAT:       provider.CAPABILITY_MODEL_GENERATE,
	benchmark.KIND_IMAGE:      provider.CAPABILITY_IMAGE_GENERATE,
	benchmark.KIND_SPEECH:     provider.CAPABILITY_AUDIO_SPEECH,
	benchmark.KIND_TRANSCRIBE: provider.CAPABILITY_AUDIO_TRANSCRIBE,
	benchmark.KIND_VIDEO:      provider.CAPABILITY_VIDEO_GENERATE,
	benchmark.KIND_MUSIC:      provider.CAPABILITY_MUSIC_GENERATE,
}

func main() {
	providerName := flag.String("provider", provider.DEFAULT_NAME,
		"provider name; see -list")
	model := flag.String("model", "",
		"model id from the provider DefaultCatalog (off-catalog ids warn), or \"all\" to sweep every catalog model. Empty = adapter default. Applies to the chat cases; when chat is not among the selected kinds it applies to every selected case (audio/image/video/music models are models too)")
	kinds := flag.String("kinds", "",
		"comma-separated case kinds (chat,image,speech,transcribe,video,music); empty = every kind the provider supports; an explicitly requested unsupported kind runs and is recorded as FAIL")
	list := flag.Bool("list", false,
		"list registered providers with capabilities and DefaultCatalog models (each annotated with the kinds it can serve), then exit")
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

	selected, err := selectKinds(entry, *kinds)
	if err != nil {
		fatal(err)
	}

	if *model == MODEL_ALL {
		if err := sweepCatalog(context.Background(), entry, selected); err != nil {
			fatal(err)
		}
		return
	}

	var cases []benchmark.Case
	for _, kind := range selected {
		cases = append(cases, caseSets[kind]()...)
	}
	if len(cases) == 0 {
		fatal(fmt.Errorf("provider %s supports none of the benchmark kinds", entry.Name))
	}
	// Without a chat kind in the selection, -model can only mean the media
	// model — a media-only run (elevenlabs speech, minimax music) would
	// otherwise silently ignore the flag and ride the adapter default.
	if *model != "" && !slices.Contains(selected, benchmark.KIND_CHAT) {
		cases = benchmark.WithModel(*model, cases)
	}

	target := benchmark.Target{Provider: entry.Name, Model: *model}
	if err := benchmark.RunPair(context.Background(), target, cases); err != nil {
		fatal(err)
	}
}

// sweepCatalog runs every DefaultCatalog model on the kinds it can serve
// (KindsOf ∩ the selected kinds). Models the benchmark cannot drive are
// reported and skipped, and one model's failure never stops the sweep —
// the same report-and-continue contract the per-case flow has.
func sweepCatalog(ctx context.Context, entry provider.Entry, selected []benchmark.Kind) error {
	specs := benchmark.CatalogSpecs(entry.Name)
	if len(specs) == 0 {
		return fmt.Errorf("provider %s ships no DefaultCatalog to sweep", entry.Name)
	}
	for _, spec := range specs {
		var kinds []benchmark.Kind
		for _, kind := range benchmark.KindsOf(entry.Name, spec) {
			if slices.Contains(selected, kind) {
				kinds = append(kinds, kind)
			}
		}
		if len(kinds) == 0 {
			fmt.Printf("skip %s: no runnable kind in this benchmark\n", spec.ID)
			continue
		}
		var cases []benchmark.Case
		for _, kind := range kinds {
			cases = append(cases, benchmark.WithModel(spec.ID, caseSets[kind]())...)
		}
		target := benchmark.Target{Provider: entry.Name, Model: spec.ID}
		if err := benchmark.RunPair(ctx, target, cases); err != nil {
			fmt.Fprintf(os.Stderr, "benchmark: %s: %v\n", spec.ID, err)
		}
	}
	return nil
}

// selectKinds resolves the -kinds flag. Empty selects every kind the entry
// supports, in kindOrder; an explicit list is validated against the
// vocabulary only, so an unsupported-but-requested kind still runs and gets
// recorded as a failing case.
func selectKinds(entry provider.Entry, flagValue string) ([]benchmark.Kind, error) {
	if strings.TrimSpace(flagValue) == "" {
		var out []benchmark.Kind
		for _, kind := range kindOrder {
			if entry.Supports(kindCapability[kind]) {
				out = append(out, kind)
			}
		}
		return out, nil
	}

	var out []benchmark.Kind
	for raw := range strings.SplitSeq(flagValue, ",") {
		kind := benchmark.Kind(strings.TrimSpace(strings.ToLower(raw)))
		if _, ok := caseSets[kind]; !ok {
			return nil, fmt.Errorf("unknown kind %q (known: %s)", raw, joinKinds(kindOrder))
		}
		out = append(out, kind)
	}
	return out, nil
}

func printList() {
	for _, entry := range provider.Entries() {
		var supported []benchmark.Kind
		for _, kind := range kindOrder {
			if entry.Supports(kindCapability[kind]) {
				supported = append(supported, kind)
			}
		}
		fmt.Printf("%-12s kinds: %s\n", entry.Name, joinKinds(supported))
		for _, spec := range benchmark.CatalogSpecs(entry.Name) {
			kinds := joinKinds(benchmark.KindsOf(entry.Name, spec))
			if kinds == "" {
				kinds = "-"
			}
			fmt.Printf("  %-42s %s\n", spec.ID, kinds)
		}
	}
}

func joinKinds(kinds []benchmark.Kind) string {
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, string(kind))
	}
	return strings.Join(names, ",")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "benchmark: %v\n", err)
	os.Exit(1)
}
