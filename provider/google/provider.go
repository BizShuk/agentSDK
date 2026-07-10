// Package google adapts google.golang.org/genai to core.ModelProvider.
//
// The Gemini API supports multimodal inputs natively — agentsdk's
// core.Part (Text / Audio / Image) maps to genai.Part; we forward
// images as inline parts so the model can see them.
package google

import (
	"context"
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/core"
	genai "google.golang.org/genai"
)

// Provider implements core.ModelProvider against the Gemini API.
type Provider struct {
	client *genai.Client
	model  string
}

// New returns a Provider using GOOGLE_API_KEY env var (or pass-in).
// model defaults to "gemini-2.0-flash".
func New(ctx context.Context, opts ...Option) (*Provider, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.apiKey == "" {
		cfg.apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if cfg.apiKey == "" {
		return nil, fmt.Errorf("google: API key not set (use WithAPIKey or GOOGLE_API_KEY)")
	}
	cc := &genai.ClientConfig{
		APIKey: cfg.apiKey,
	}
	c, err := genai.NewClient(ctx, cc)
	if err != nil {
		return nil, fmt.Errorf("google: new client: %w", err)
	}
	return &Provider{client: c, model: cfg.model}, nil
}

// Name implements core.ModelProvider.
func (p *Provider) Name() string { return "google:" + p.model }

// Generate implements core.ModelProvider.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	parts := toGenaiParts(req.Messages)
	cfg := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(maxTokensOrZero(req)),
	}
	if len(req.Tools) > 0 {
		cfg.Tools = toGenaiTools(req.Tools)
	}
	// Wrap the parts in a single user Content block.
	contents := []*genai.Content{{Role: "user", Parts: parts}}
	resp, err := p.client.Models.GenerateContent(ctx, p.model, contents, cfg)
	if err != nil {
		return core.ModelResult{}, err
	}
	return fromGenaiResponse(resp), nil
}

// Stream implements core.ModelProvider.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	ch := make(chan core.ModelChunk, 1)
	go func() {
		defer close(ch)
		mr, err := p.Generate(ctx, req)
		if err != nil {
			ch <- core.ModelChunk{Done: true}
			return
		}
		if mr.Text != "" {
			ch <- core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Text: mr.Text}
		}
		ch <- core.ModelChunk{Done: true}
	}()
	return ch, nil
}

// CountTokens implements core.ModelProvider via chars/4 + 1 heuristic.
func (p *Provider) CountTokens(_ context.Context, msgs []core.Message) (int, error) {
	n := 0
	for _, m := range msgs {
		for _, c := range m.Parts {
			if c.Kind == core.PART_KIND_PLAIN_TEXT {
				n += len(c.Text)/4 + 1
			}
		}
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// translation helpers
// ---------------------------------------------------------------------------

func toGenaiParts(msgs []core.Message) []*genai.Part {
	// Gemini collapses system + user + assistant into the parts list
	// for a single user message. We do the same here for simplicity:
	// concatenate all text + images from every message.
	var parts []*genai.Part
	for _, m := range msgs {
		for _, c := range m.Parts {
			switch c.Kind {
			case core.PART_KIND_PLAIN_TEXT:
				if c.Text != "" {
					parts = append(parts, genai.NewPartFromText(c.Text))
				}
			case core.PART_KIND_IMAGE:
				if len(c.Image) > 0 {
					mime := c.ImageMIME
					if mime == "" {
						mime = "image/png"
					}
					parts = append(parts, genai.NewPartFromBytes(c.Image, mime))
				}
			}
		}
	}
	return parts
}

func toGenaiTools(schemas []core.ToolSpec) []*genai.Tool {
	out := make([]*genai.Tool, 0, len(schemas))
	for _, s := range schemas {
		out = append(out, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{{
				Name:        s.Name,
				Description: s.Description,
				Parameters:  mustSchemaToGenaiSchema(s.Parameters),
			}},
		})
	}
	return out
}

func fromGenaiResponse(resp *genai.GenerateContentResponse) core.ModelResult {
	out := core.ModelResult{}
	if resp.UsageMetadata != nil {
		out.Usage = core.TokenUsage{
			PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(resp.UsageMetadata.TotalTokenCount),
		}
	}
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, p := range cand.Content.Parts {
			if p.Text != "" {
				out.Text += p.Text
			}
			if p.FunctionCall != nil {
				args := p.FunctionCall.Args
				if args == nil {
					args = map[string]any{}
				}
				// Prefer the function-call id when the SDK provides one
				// (Gemini returns it for tool-call correlation). Fall back
				// to the name so single-call transcripts still pair up.
				callID := p.FunctionCall.ID
				if callID == "" {
					callID = p.FunctionCall.Name
				}
				out.ToolCalls = append(out.ToolCalls, core.ToolCall{
					ID:   callID,
					Name: p.FunctionCall.Name,
					Args: args,
				})
			}
		}
		if cand.FinishReason != "" {
			out.StopReason = string(cand.FinishReason)
		}
	}
	return out
}

// mustSchemaToGenaiSchema converts agentsdk's loose Parameters shape
// to a genai.Schema. Falls back to an empty object schema when the
// input is missing or malformed.
func mustSchemaToGenaiSchema(v any) *genai.Schema {
	if v == nil {
		return &genai.Schema{Type: "OBJECT"}
	}
	if raw, ok := v.([]byte); ok {
		var s genai.Schema
		if err := unmarshalJSON(raw, &s); err == nil {
			return &s
		}
	}
	return &genai.Schema{Type: "OBJECT"}
}

// jsonUnmarshal indirection so the file compiles without importing
// encoding/json at the top.
func jsonUnmarshal(data []byte, v any) error {
	return unmarshalJSON(data, v)
}

// maxTokensOrZero returns req.MaxTokens or 0. The genai SDK treats
// 0 as "use API default" so we hand it through unchanged.
func maxTokensOrZero(req core.ModelRequest) int {
	return req.MaxTokens
}