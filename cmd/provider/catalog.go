package provider

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

// Catalog prints the provider's ModelSpec list behind --list-models. It
// prefers the live upstream catalog (provider.ModelLister) and falls back to the
// bundled static catalog when no client on this entry can enumerate models,
// the client will not build (missing credential, bad base URL), or the live
// call fails (offline, bad key). The source is reported on stderr so the
// stdout list stays clean for piping.
func Catalog(
	ctx context.Context,
	entry provider.Entry,
	options provider.Options,
	errOut, out io.Writer,
) error {
	var static []provider.ModelSpec
	if entry.Catalog != nil {
		static = entry.Catalog()
	}
	label := entry.Name
	lister, err := openLister(entry, options)
	if err != nil {
		fmt.Fprintf(errOut,
			"[provider] %s: live catalog unavailable, using static (%v)\n", label, err)
	}

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
		fmt.Fprintf(out,
			"  %-40s capabilities=%s input=%s output=%s family=%s reasoning=%v ctx=%d max=%d\n",
			s.ID,
			JoinCapabilities(s.Capabilities),
			joinModalities(s.InputModalities),
			joinModalities(s.OutputModalities),
			s.Family,
			s.Reasoning,
			s.ContextWindow,
			s.MaxTokens,
		)
	}
	return nil
}

func joinModalities(modalities []provider.Modality) string {
	if len(modalities) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(modalities))
	for _, modality := range modalities {
		names = append(names, string(modality))
	}
	return strings.Join(names, ",")
}

// openLister builds whichever client on this entry can enumerate models
// live, or returns nil when none can. The chat surface is preferred; an
// audio-only entry falls back to its speech client, which is where
// elevenlabs hangs its GET /v1/models call. A nil lister is not an error —
// Catalog then prints the bundled catalog alone.
func openLister(entry provider.Entry, options provider.Options) (provider.ModelLister, error) {
	switch {
	case entry.Supports(provider.CAPABILITY_CHAT):
		prov, err := provider.New(entry.Name, options)
		if err != nil {
			return nil, err
		}
		lister, _ := prov.(provider.ModelLister)
		return lister, nil
	case entry.Supports(provider.CAPABILITY_SPEECH):
		speech, err := provider.NewSpeech(entry.Name, options)
		if err != nil {
			return nil, err
		}
		lister, _ := speech.(provider.ModelLister)
		return lister, nil
	default:
		return nil, nil
	}
}

// JoinCapabilities renders an entry's capability list for error messages.
func JoinCapabilities(capabilities []provider.Capability) string {
	if len(capabilities) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		names = append(names, string(capability))
	}
	return strings.Join(names, ",")
}
