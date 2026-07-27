package svc

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/all"
)

// Chat executes a text/chat model generation request against the target provider.
func Chat(ctx context.Context, req Request, out io.Writer) error {
	client, err := provider.New(req.Provider, req.Options)
	if err != nil {
		return err
	}
	result, err := client.Generate(ctx, core.ModelRequest{
		Messages: []core.Message{{
			Role: core.ROLE_USER,
			Parts: []core.Part{{
				Kind: core.PART_KIND_PLAIN_TEXT,
				Text: req.Prompt,
			}},
			Ts: time.Now().UTC(),
		}},
	})
	if err != nil {
		return fmt.Errorf("chat: %w", err)
	}
	if req.JSON {
		return WriteJSON(out, result)
	}
	if result.Text != "" {
		if _, err := fmt.Fprintln(out, result.Text); err != nil {
			return fmt.Errorf("write chat text: %w", err)
		}
	}
	if _, err := fmt.Fprintf(
		out,
		"[stop=%s tokens=%d/%d]\n",
		result.StopReason,
		result.Usage.PromptTokens,
		result.Usage.CompletionTokens,
	); err != nil {
		return fmt.Errorf("write chat usage: %w", err)
	}
	return nil
}
