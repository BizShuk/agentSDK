package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// Chat runs one prompt through core.Provider.Generate, or
// core.StreamProvider.Stream when the request asks for it.
func Chat(ctx context.Context, req Request, out io.Writer) error {
	prov, err := provider.New(req.Provider, req.Options)
	if err != nil {
		return err
	}
	modelReq := buildModelRequest(req.Prompt, req.System, req.MaxTokens)
	if req.Stream {
		return runStream(ctx, prov, modelReq, out, req.JSON)
	}
	return runGenerate(ctx, prov, modelReq, out, req.JSON)
}

// buildModelRequest turns a CLI prompt into a single-turn core.ModelRequest.
// The system message is optional; parts default to plain text.
func buildModelRequest(prompt, system string, maxTokens int) core.ModelRequest {
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
	fmt.Fprintf(
		out,
		"[stop=%s tokens=%d/%d cache_read=%d web_search=%d cost_usd=%s cost_status=%s]\n",
		res.StopReason,
		res.Usage.InputTokens,
		res.Usage.OutputTokens,
		res.Usage.InputCacheReadTokens,
		res.Usage.WebSearchCount,
		res.Cost.AmountUSD,
		res.Cost.Status,
	)
	return nil
}

func runStream(ctx context.Context, prov core.StreamProvider, req core.ModelRequest,
	out io.Writer, asJSON bool,
) error {
	ch, err := prov.Stream(ctx, req)
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	sawDone := false
	var terminal core.ModelChunk
	enc := json.NewEncoder(out)
	for c := range ch {
		if c.Done {
			sawDone = true
			terminal = c
		}
		if asJSON {
			if err := enc.Encode(c); err != nil {
				return err
			}
			continue
		}
		if c.Done {
			continue
		}
		if c.Kind == core.PART_KIND_PLAIN_TEXT && c.Text != "" {
			fmt.Fprint(out, c.Text)
		}
	}
	if !sawDone {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("stream interrupted: %w", err)
		}
		return fmt.Errorf("stream closed before terminal chunk")
	}
	if asJSON {
		return nil
	}
	fmt.Fprintf(
		out,
		"\n[tokens=%d/%d cache_read=%d web_search=%d cost_usd=%s cost_status=%s]\n",
		terminal.Usage.InputTokens,
		terminal.Usage.OutputTokens,
		terminal.Usage.InputCacheReadTokens,
		terminal.Usage.WebSearchCount,
		terminal.Cost.AmountUSD,
		terminal.Cost.Status,
	)
	return nil
}
