package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/bizshuk/agentsdk/provider"
)

func writeProviderMatrix(out io.Writer) error {
	writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	header := "PROVIDER\tCHAT\tIMAGE\tMUSIC\tSPEECH\tTRANSCRIBE\tAUTH ENV"
	if _, err := fmt.Fprintln(writer, header); err != nil {
		return fmt.Errorf("write provider matrix header: %w", err)
	}
	for _, entry := range provider.Entries() {
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			entry.Name,
			yesNo(entry.Supports(provider.CAPABILITY_MODEL_GENERATE)),
			yesNo(entry.Supports(provider.CAPABILITY_IMAGE_GENERATE)),
			yesNo(entry.Supports(provider.CAPABILITY_MUSIC_GENERATE)),
			yesNo(entry.Supports(provider.CAPABILITY_AUDIO_SPEECH)),
			yesNo(entry.Supports(provider.CAPABILITY_AUDIO_TRANSCRIBE)),
			authEnvironmentSummary(entry.Metadata),
		); err != nil {
			return fmt.Errorf("write provider matrix row: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush provider matrix: %w", err)
	}
	return nil
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func authEnvironmentSummary(metadata provider.Metadata) string {
	var modes []string
	if len(metadata.APIKeyEnv) > 0 {
		modes = append(modes, "api_key:"+strings.Join(metadata.APIKeyEnv, "|"))
	}
	if len(metadata.OAuthEnv) > 0 {
		modes = append(modes, "oauth:"+strings.Join(metadata.OAuthEnv, "|"))
	}
	if len(modes) == 0 {
		return "none"
	}
	return strings.Join(modes, ",")
}
