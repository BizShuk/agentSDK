package svc

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/bizshuk/agentsdk/provider"
)

// Request holds the parameter payload required to execute a provider API request.
type Request struct {
	Provider string
	Prompt   string
	JSON     bool
	Options  provider.Options
}

// WriteJSON encodes value as formatted JSON to out.
func WriteJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode JSON output: %w", err)
	}
	return nil
}
