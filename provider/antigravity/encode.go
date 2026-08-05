package antigravity

// Encoding core.ModelRequest onto the Cloud Code wire.
//
// Two rules shape everything below:
//
//   - System text is hoisted out of the transcript. Gemini has no system
//     role; it takes a separate systemInstruction message.
//   - Reasoning only travels with its signature. The gateway validates
//     the signature on every thought part and rejects the request when it
//     is missing or foreign, so an unsigned thought is dropped rather
//     than sent — it is the model's own scratch text, and losing it costs
//     continuity, whereas sending it costs the whole turn.

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// GEMINI_SKIP_SIGNATURE tells Gemini 3+ to skip thought-signature
// validation on a functionCall part. Gemini requires a signature on every
// tool call; when the transcript carries no reasoning to source one from,
// this sentinel is the documented way to proceed instead of failing.
//
// See https://ai.google.dev/gemini-api/docs/thought-signatures
const GEMINI_SKIP_SIGNATURE = "skip_thought_signature_validator"

// MAX_TOOL_NAME_BYTES is the gateway's limit on a function name.
const MAX_TOOL_NAME_BYTES = 64

// buildRequest assembles the full v1internal envelope for one model call.
func buildRequest(req core.ModelRequest, model, project, sessionID, requestID string) (CloudCodeRequest, error) {
	contents, err := toContents(req.Messages, model)
	if err != nil {
		return CloudCodeRequest{}, err
	}

	inner := GenerateRequest{
		Contents:          contents,
		SystemInstruction: systemInstruction(req.Messages),
		GenerationConfig:  generationConfig(req, model),
		SessionID:         sessionID,
	}
	if len(req.Tools) > 0 {
		inner.Tools = []Tool{{FunctionDeclarations: toDeclarations(req.Tools)}}
		if isClaudeModel(model) {
			// Claude on this gateway validates arguments only when asked
			// to; without it, malformed tool args reach the tool.
			inner.ToolConfig = &ToolConfig{FunctionCallingConfig: FunctionCallingConfig{Mode: "VALIDATED"}}
		}
	}

	return CloudCodeRequest{
		Project:     project,
		Model:       model,
		Request:     inner,
		UserAgent:   CLIENT_NAME,
		RequestType: "agent",
		RequestID:   requestID,
	}, nil
}

// systemInstruction hoists the caller's system messages into the
// separate message Gemini takes for them.
//
// Both reference proxies additionally prepend an "You are Antigravity…"
// persona and then a second copy wrapped in an ignore marker to cancel
// it. That is not replicated here, and the omission was verified against
// the live gateway: Gemini 2.5, Gemini 3.6 and Claude Sonnet 4.6 all
// answer normally without it. It costs ~150 prompt tokens per request
// and makes the model introduce itself as Antigravity — with the prelude
// in place, `provider "ping"` came back "Pong! I am Antigravity, your
// agentic coding assistant". The proxies need it because they impersonate
// the IDE for Claude Code; an SDK adapter has no such obligation.
func systemInstruction(msgs []core.Message) *Content {
	var parts []Part
	for _, m := range msgs {
		if m.Role != core.ROLE_SYSTEM {
			continue
		}
		for _, p := range m.Parts {
			if p.Kind == core.PART_KIND_PLAIN_TEXT && strings.TrimSpace(p.Text) != "" {
				parts = append(parts, Part{Text: p.Text})
			}
		}
	}
	if len(parts) == 0 {
		return nil
	}
	// The gateway wants role "user" here, not "system" — the field name
	// carries the meaning, the role field is vestigial.
	return &Content{Role: "user", Parts: parts}
}

// toContents converts the transcript, minus system messages, into Gemini
// contents.
func toContents(msgs []core.Message, model string) ([]Content, error) {
	gemini := isGeminiModel(model)
	out := make([]Content, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == core.ROLE_SYSTEM {
			continue
		}
		role := "user"
		if m.Role == core.ROLE_ASSISTANT {
			role = "model"
		}
		parts, err := toParts(m.Parts, gemini)
		if err != nil {
			return nil, err
		}
		if len(parts) == 0 {
			// Every content entry must carry at least one part. This
			// happens when a message held nothing but unsigned
			// reasoning; a lone period is invisible in practice and
			// keeps the turn structure intact.
			parts = []Part{{Text: "."}}
		}
		out = append(out, Content{Role: role, Parts: parts})
	}
	return out, nil
}

// toParts converts one message's parts. gemini selects the thought-
// signature handling Gemini 3+ requires on tool calls.
func toParts(in []core.Part, gemini bool) ([]Part, error) {
	var out []Part
	// signature carries the most recent reasoning signature within this
	// message so a following functionCall can be signed with it.
	signature := ""

	for _, p := range in {
		switch p.Kind {
		case core.PART_KIND_PLAIN_TEXT:
			if p.Text != "" {
				out = append(out, Part{Text: p.Text})
			}

		case core.PART_KIND_REASONING:
			if p.Reasoning != nil && (p.Reasoning.ID != "" || p.Reasoning.EncryptedContent != "") {
				return nil, fmt.Errorf("antigravity: reasoning part carries Responses continuation metadata; encode it into a thought signature before calling the adapter")
			}
			if p.Reasoning == nil || p.Reasoning.Signature == "" {
				continue // unsigned thought — see the file comment
			}
			signature = p.Reasoning.Signature
			out = append(out, Part{
				Text:             p.Text,
				Thought:          true,
				ThoughtSignature: signature,
			})

		case core.PART_KIND_IMAGE:
			if len(p.Image) > 0 {
				out = append(out, Part{InlineData: &InlineData{
					MIMEType: mimeOrDefault(p.ImageMIME, "image/jpeg"),
					Data:     base64.StdEncoding.EncodeToString(p.Image),
				}})
			}

		case core.PART_KIND_AUDIO:
			if len(p.Audio) > 0 {
				out = append(out, Part{InlineData: &InlineData{
					MIMEType: mimeOrDefault(p.AudioMIME, "audio/mpeg"),
					Data:     base64.StdEncoding.EncodeToString(p.Audio),
				}})
			}

		case core.PART_KIND_TOOL_USE:
			if p.ToolUse == nil {
				continue
			}
			args := p.ToolUse.Args
			if args == nil {
				args = map[string]any{}
			}
			part := Part{FunctionCall: &FunctionCall{
				ID:   p.ToolUse.ID,
				Name: sanitizeToolName(p.ToolUse.Name),
				Args: args,
			}}
			if gemini {
				part.ThoughtSignature = signature
				if part.ThoughtSignature == "" {
					part.ThoughtSignature = GEMINI_SKIP_SIGNATURE
				}
			}
			out = append(out, part)

		case core.PART_KIND_TOOL_RESULT:
			if p.ToolResult == nil {
				continue
			}
			// Gemini matches a response to its declaration by NAME, so
			// the tool's own name is what goes in Name — the reference
			// proxies put the call id there only because the Anthropic
			// payload they translate has no name to use. The id is kept
			// in ID, which is what Claude matches on.
			name := p.ToolResult.Name
			if name == "" {
				name = p.ToolResult.CallID
			}
			out = append(out, Part{FunctionResponse: &FunctionResponse{
				ID:       p.ToolResult.CallID,
				Name:     sanitizeToolName(name),
				Response: toolResponsePayload(*p.ToolResult),
			}})
		}
	}
	return out, nil
}

// toolResponsePayload shapes a tool outcome into the object Gemini
// expects. Errors are reported in-band: the model has to see that the
// call failed to decide what to do next.
func toolResponsePayload(r core.ToolResult) map[string]any {
	if r.Error != "" {
		return map[string]any{"error": r.Error}
	}
	if r.Output == nil {
		return map[string]any{"result": ""}
	}
	return map[string]any{"result": r.Output}
}

// toDeclarations converts tool specs into function declarations.
func toDeclarations(specs []core.ToolSpec) []FunctionDeclaration {
	out := make([]FunctionDeclaration, 0, len(specs))
	for _, s := range specs {
		out = append(out, FunctionDeclaration{
			Name:        sanitizeToolName(s.Name),
			Description: s.Description,
			Parameters:  encodeSchema(s.Parameters),
		})
	}
	return out
}

// generationConfig assembles sampling and thinking controls for one call.
func generationConfig(req core.ModelRequest, model string) *GenerationConfig {
	cfg := &GenerationConfig{
		MaxOutputTokens: req.MaxTokens,
		StopSequences:   req.StopReasons,
	}
	if cfg.MaxOutputTokens <= 0 {
		cfg.MaxOutputTokens = DEFAULT_MAX_TOKENS
	}

	switch {
	case !isThinkingModel(model):
		// no thinking config at all

	case isClaudeModel(model):
		cfg.ThinkingConfig = ClaudeThinkingConfig{
			IncludeThoughts: true,
			ThinkingBudget:  CLAUDE_THINKING_BUDGET,
		}
		if cfg.MaxOutputTokens <= CLAUDE_THINKING_BUDGET {
			cfg.MaxOutputTokens = CLAUDE_THINKING_BUDGET + CLAUDE_THINKING_HEADROOM
		}

	default:
		// No thinkingBudget, deliberately. The reference proxies always
		// send one, but against the live gateway it changes nothing for
		// the better: gemini-3.6-flash-high returns no `thought:true`
		// parts with a budget of -1, 8192, or none at all — thoughts are
		// billed (thoughtsTokenCount) but not surfaced. Setting 8192 made
		// it worse, pushing the reasoning into VISIBLE text ("Here is the
		// step-by-step thinking: 1. …"), which is exactly what keeping
		// reasoning in its own part is meant to prevent.
		//
		// IncludeThoughts stays so the parts flow through if Google
		// starts returning them. Claude models on this gateway DO return
		// signed thoughts today — see the case above.
		cfg.ThinkingConfig = GeminiThinkingConfig{IncludeThoughts: true}
	}

	if isGeminiModel(model) && cfg.MaxOutputTokens > GEMINI_MAX_OUTPUT_TOKENS {
		cfg.MaxOutputTokens = GEMINI_MAX_OUTPUT_TOKENS
	}
	return cfg
}

// sanitizeToolName drops characters the gateway's function-name grammar
// rejects and enforces its length limit. MCP servers routinely produce
// names with dots and colons that would otherwise 400 the whole request.
func sanitizeToolName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > MAX_TOOL_NAME_BYTES {
		out = out[:MAX_TOOL_NAME_BYTES]
	}
	return out
}

func mimeOrDefault(mime, fallback string) string {
	if mime == "" {
		return fallback
	}
	return mime
}
