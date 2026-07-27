package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const defaultModel = "gpt-5.5"

// Provider implements core.Provider against the OpenAI Codex
// endpoint (chatgpt.com/backend-api/codex/responses). It is the
// sole entry point for clients.
type Provider struct {
	baseURL string
	// auth carries the credential AND the Codex account id. The account
	// id is a header (ChatGPT-Account-ID), so core.Auth.Headers is its
	// natural home — a separate field would be a second thing a
	// per-request credential override could not reach.
	auth   core.Auth
	model  string
	client *http.Client
}

// New returns a Provider from registry-resolved construction config.
func New(cfg provider.ResolvedConfig) (*Provider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	return &Provider{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		auth:    cfg.Auth,
		model:   cfg.Model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// Generate implements core.Provider. It POSTs the Codex-shaped
// request body and returns the folded ModelResult.
//
// The underlying /codex/responses endpoint always streams (we set
// stream: true unconditionally); a JSON-shaped response is returned
// only when stream=false, which is outside Codex's contract. The
// non-stream callers that anchor on this method go through a
// single-shot Generator that reads the entire SSE flow and folds
// it into one ModelResult — see Generate's implementation.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	body, err := p.buildRequestBody(req)
	if err != nil {
		return core.ModelResult{}, err
	}
	raw, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(raw))
	if err != nil {
		return core.ModelResult{}, err
	}
	p.applyHeaders(httpReq, req.Auth)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("codex: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		errBody, _ := io.ReadAll(resp.Body)
		return core.ModelResult{}, fmt.Errorf("codex: status %d: %s", resp.StatusCode, errBody)
	}
	// Codex always streams. Fold the SSE stream into a ModelResult.
	ch, err := ParseStream(ctx, resp.Body)
	if err != nil {
		return core.ModelResult{}, err
	}
	out := core.ModelResult{}
	sawDone := false
	for chunk := range ch {
		if chunk.Done {
			sawDone = true
			break
		}
		switch chunk.Kind {
		case core.PART_KIND_PLAIN_TEXT:
			appendModelPart(&out, core.Part{Kind: core.PART_KIND_PLAIN_TEXT, Text: chunk.Text})
			out.StopReason = "stop"
		case core.PART_KIND_REASONING:
			appendModelPart(&out, core.Part{
				Kind:      core.PART_KIND_REASONING,
				Text:      chunk.Text,
				Reasoning: chunk.Reasoning,
			})
		case core.PART_KIND_TOOL_USE:
			if chunk.ToolUse != nil {
				call := *chunk.ToolUse
				appendModelPart(&out, core.Part{Kind: core.PART_KIND_TOOL_USE, ToolUse: &call})
				out.StopReason = "tool_use"
			}
		}
	}
	if !sawDone {
		if err := ctx.Err(); err != nil {
			return core.ModelResult{}, fmt.Errorf("codex: stream interrupted: %w", err)
		}
		return core.ModelResult{}, fmt.Errorf("codex: stream closed before terminal chunk")
	}
	return out.NormalizeContent(), nil
}

// Stream implements core.StreamProvider. It returns a channel of
// core.ModelChunk that the runtime consumes incrementally.
//
// The HTTP body stays open for the entire stream; callers should
// drain the channel before returning from their handler.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	body, err := p.buildRequestBody(req)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	p.applyHeaders(httpReq, req.Auth)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("codex: http: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("codex: status %d: %s", resp.StatusCode, errBody)
	}
	return ParseStream(ctx, resp.Body)
}

// ---------------------------------------------------------------------------
// internal — body construction
// ---------------------------------------------------------------------------

// buildRequestBody applies the Codex-specific transformations to a
// core.ModelRequest, producing the final RequestBody shape. The
// transformations are:
//
//   - lift system/developer messages out of input[] into the
//     top-level Instructions field
//   - force Stream=true, Store=false
//   - STRIP max_output_tokens (Codex rejects it; we intentionally
//     drop req.MaxTokens — see buildRequestBody)
//   - for lite models (IsLiteModel), force parallel_tool_calls=false
//
// The function rejects provider-specific reasoning metadata that the
// Responses wire format cannot preserve.
func (p *Provider) buildRequestBody(req core.ModelRequest) (RequestBody, error) {
	body := RequestBody{
		Model:   p.model,
		Stream:  true,
		Store:   false,
		Include: []string{"reasoning.encrypted_content"},
	}
	instructions, input, err := liftInstructions(req.Messages)
	if err != nil {
		return RequestBody{}, err
	}
	body.Instructions = instructions
	body.Input = input
	body.Tools = translateTools(req.Tools)
	if IsLiteModel(p.model) {
		v := false
		body.ParallelToolCalls = &v
	}
	// intentionally NOT forwarding req.MaxTokens — Codex rejects
	// max_output_tokens on the wire, so the field is silently
	// dropped here.
	return body, nil
}

// liftInstructions extracts system/developer messages from
// msg.Parts and concatenates them into a single Instructions
// string. Non-instruction messages stay in the input list.
//
// Each text part is joined with "\n\n" so multi-part system
// prompts render as separate paragraphs in Codex's view.
func liftInstructions(msgs []core.Message) (string, []InputItem, error) {
	var instructions []string
	input := make([]InputItem, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == core.ROLE_SYSTEM {
			for _, c := range m.Parts {
				if c.Kind == core.PART_KIND_PLAIN_TEXT && c.Text != "" {
					instructions = append(instructions, c.Text)
				}
			}
			continue
		}
		role := "user"
		switch m.Role {
		case core.ROLE_ASSISTANT:
			role = "assistant"
		case core.ROLE_TOOL:
			role = "tool"
		}
		item := InputItem{Type: "message", Role: role}
		flushMessage := func() {
			if len(item.Content) == 0 {
				return
			}
			input = append(input, item)
			item = InputItem{Type: "message", Role: role}
		}
		for _, c := range m.Parts {
			switch c.Kind {
			case core.PART_KIND_PLAIN_TEXT:
				if c.Text != "" {
					item.Content = append(item.Content, ContentBlock{Type: "input_text", Text: c.Text})
				}
			case core.PART_KIND_REASONING:
				flushMessage()
				reasoningItem, err := toReasoningInput(c)
				if err != nil {
					return "", nil, err
				}
				input = append(input, reasoningItem)
			case core.PART_KIND_IMAGE:
				if len(c.Image) > 0 {
					mime := c.ImageMIME
					if mime == "" {
						mime = "image/png"
					}
					item.Content = append(item.Content, ContentBlock{
						Type: "input_image",
						ImageURL: &ImageURL{
							URL: fmt.Sprintf("data:%s;base64,%s", mime, string(c.Image)),
						},
					})
				}
			case core.PART_KIND_TOOL_USE:
				if c.ToolUse != nil {
					flushMessage()
					input = append(input, InputItem{
						Type: "message",
						Role: "assistant",
						Content: []ContentBlock{{
							Type: "input_text",
							Text: fmt.Sprintf("[assistant invoked tool %s with id=%s]", c.ToolUse.Name, c.ToolUse.ID),
						}},
					})
				}
			case core.PART_KIND_TOOL_RESULT:
				if c.ToolResult != nil {
					payload, _ := json.Marshal(c.ToolResult.Output)
					item.Content = append(item.Content, ContentBlock{
						Type: "input_text",
						Text: fmt.Sprintf("[tool %s returned ok=%v err=%q %s]", c.ToolResult.Name, c.ToolResult.OK, c.ToolResult.Error, string(payload)),
					})
				}
			}
		}
		flushMessage()
	}
	return strings.Join(instructions, "\n\n"), input, nil
}

func toReasoningInput(part core.Part) (InputItem, error) {
	item := InputItem{Type: "reasoning"}
	if part.Text != "" {
		item.Summary = []ContentBlock{{Type: "summary_text", Text: part.Text}}
	}
	if part.Reasoning == nil {
		return item, nil
	}
	if part.Reasoning.Signature != "" {
		return InputItem{}, fmt.Errorf("codex: reasoning part carries an Anthropic signature that Responses cannot represent")
	}
	item.ID = part.Reasoning.ID
	item.EncryptedContent = part.Reasoning.EncryptedContent
	return item, nil
}

func appendModelPart(result *core.ModelResult, part core.Part) {
	if part.Kind == core.PART_KIND_PLAIN_TEXT && len(result.Parts) > 0 {
		last := &result.Parts[len(result.Parts)-1]
		if last.Kind == core.PART_KIND_PLAIN_TEXT {
			last.Text += part.Text
			return
		}
	}
	result.Parts = append(result.Parts, part)
}

// translateTools converts core.ToolSpec → codex.Tool, copying the
// JSON Schema parameters verbatim. We accept either a json.RawMessage
// (most common — emitted by the schema generator) or any other type
// (we re-marshal it back to JSON so the wire shape is always raw).
func translateTools(specs []core.ToolSpec) []Tool {
	if len(specs) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(specs))
	for _, s := range specs {
		t := Tool{
			Type:        "function",
			Name:        s.Name,
			Description: s.Description,
		}
		if raw, ok := s.Parameters.(json.RawMessage); ok {
			t.Parameters = raw
		} else if s.Parameters != nil {
			raw, _ := json.Marshal(s.Parameters)
			t.Parameters = raw
		}
		out = append(out, t)
	}
	return out
}

// ---------------------------------------------------------------------------
// internal — HTTP transport
// ---------------------------------------------------------------------------

// endpoint is the upstream path appended to baseURL.
func (p *Provider) endpoint() string {
	return p.baseURL + "/codex/responses"
}

// applyHeaders sets the Codex identity headers plus auth. The set
// is fixed — Codex is picky about the header list:
//
//   - Content-Type
//   - originator: codex_cli_rs
//   - version:    0.125.0
//   - User-Agent: codex_cli_rs/0.125.0 (<platform>; <arch>)
//   - ChatGPT-Account-ID (when set)
//   - Authorization: Bearer <token>  (oauth or api key path)
//
// override is the per-call core.ModelRequest.Auth, merged on top of the
// credential bound at construction; a zero override changes nothing.
func (p *Provider) applyHeaders(req *http.Request, override core.Auth) {
	a := p.auth.Merge(override)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("originator", CodexOriginator)
	req.Header.Set("version", CodexVersion)
	req.Header.Set("User-Agent", CodexUserAgent())
	// Codex sends both credential classes in the same header.
	if tok := a.Token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	for k, v := range a.Headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
}
