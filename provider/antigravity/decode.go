package antigravity

// Decoding one Cloud Code response into core vocabulary.

import (
	"github.com/bizshuk/agentsdk/core"
)

// FromResponse folds a blocking GenerateResponse into a ModelResult.
func FromResponse(r GenerateResponse) core.ModelResult {
	out := core.ModelResult{Usage: toUsage(r.UsageMetadata)}
	if len(r.Candidates) == 0 {
		return out.NormalizeContent()
	}
	candidate := r.Candidates[0]
	out.Parts = toCoreParts(candidate.Content.Parts)
	out.StopReason = StopReason(candidate.FinishReason, out.Parts)
	return out.NormalizeContent()
}

// toCoreParts converts Gemini parts into ordered core parts.
func toCoreParts(parts []Part) []core.Part {
	var out []core.Part
	for _, p := range parts {
		switch {
		case p.FunctionCall != nil:
			call := core.ToolCall{
				ID:   p.FunctionCall.ID,
				Name: p.FunctionCall.Name,
				Args: p.FunctionCall.Args,
			}
			if call.ID == "" {
				// Claude supplies an id; Gemini does not, and the
				// runtime pairs results to calls by id. The function
				// name is unique within one response, which is the
				// scope that matters.
				call.ID = p.FunctionCall.Name
			}
			out = append(out, core.Part{Kind: core.PART_KIND_TOOL_USE, ToolUse: &call})

		case p.Thought:
			out = append(out, core.Part{
				Kind:      core.PART_KIND_REASONING,
				Text:      p.Text,
				Reasoning: &core.ReasoningState{Signature: p.ThoughtSignature},
			})

		case p.InlineData != nil:
			out = append(out, core.Part{
				Kind:      core.PART_KIND_IMAGE,
				Image:     decodeInline(p.InlineData.Data),
				ImageMIME: p.InlineData.MIMEType,
			})

		case p.Text != "":
			out = append(out, core.Part{Kind: core.PART_KIND_PLAIN_TEXT, Text: p.Text})
		}
	}
	return out
}

// StopReason maps a Gemini finishReason onto the Anthropic-flavoured
// vocabulary the rest of this repo's adapters emit, so a caller switching
// providers does not have to learn a second set of strings.
//
// A response carrying tool calls reports "tool_use" regardless of the
// finish reason: Gemini reports STOP for a turn that ends in a tool call,
// which reads as "done" to anything inspecting the field.
func StopReason(finish string, parts []core.Part) string {
	for _, p := range parts {
		if p.Kind == core.PART_KIND_TOOL_USE {
			return "tool_use"
		}
	}
	switch finish {
	case "", "STOP":
		return "end_turn"
	case "MAX_TOKENS":
		return "max_tokens"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return "refusal"
	default:
		return finish
	}
}

// toUsage converts token accounting. Thinking tokens are billed output
// that Gemini reports separately from candidate tokens, so they are added
// back into the completion count rather than dropped.
func toUsage(u *UsageMetadata) core.TokenUsage {
	if u == nil {
		return core.TokenUsage{}
	}
	completion := u.CandidatesTokenCount + u.ThoughtsTokenCount
	total := u.TotalTokenCount
	if total == 0 {
		total = u.PromptTokenCount + completion
	}
	return core.TokenUsage{
		PromptTokens:     u.PromptTokenCount,
		CompletionTokens: completion,
		TotalTokens:      total,
	}
}
