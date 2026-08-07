package openaichat

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// EncodeRequest projects a provider-neutral model request into the OpenAI Chat
// Completions JSON wire shape. To preserve the established adapter contract, an
// invalid raw tool schema produces an empty payload for upstream validation
// instead of introducing a new local error.
func EncodeRequest(req core.ModelRequest, model string, stream bool) ([]byte, error) {
	messages, err := toChatMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body := requestBody{
		Model:     model,
		Messages:  messages,
		MaxTokens: maxTokensOrDefault(req),
		Stream:    stream,
	}
	if len(req.Tools) > 0 {
		body.Tools = toToolDefs(req.Tools)
	}
	if err := body.validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, nil
	}
	return raw, nil
}

// DecodeResponse folds an OpenAI Chat Completions response into the canonical
// model result used by the runtime.
func DecodeResponse(raw []byte) (core.ModelResult, error) {
	var body response
	if err := json.Unmarshal(raw, &body); err != nil {
		return core.ModelResult{}, err
	}

	out := core.ModelResult{
		Usage: core.TokenUsage{
			InputTokens:          body.Usage.InputTokens,
			OutputTokens:         body.Usage.OutputTokens,
			InputCacheReadTokens: body.Usage.InputDetails.CachedTokens,
			TotalTokens:          body.Usage.TotalTokens,
		},
	}
	for _, choice := range body.Choices {
		if choice.Message.ReasoningContent != "" {
			out.Parts = append(out.Parts, core.Part{
				Kind: core.PART_KIND_REASONING,
				Text: choice.Message.ReasoningContent,
			})
		}
		if len(out.Parts) > 0 && choice.Message.Content != "" {
			out.Parts = append(out.Parts, core.Part{
				Kind: core.PART_KIND_PLAIN_TEXT,
				Text: choice.Message.Content,
			})
		}
		out.Text += choice.Message.Content
		out.StopReason = choice.FinishReason
		for _, call := range choice.Message.ToolCalls {
			toolCall := core.ToolCall{
				ID:   call.ID,
				Name: call.Function.Name,
				Args: parseArgs(call.Function.Arguments),
			}
			out.ToolCalls = append(out.ToolCalls, toolCall)
			if len(out.Parts) > 0 {
				out.Parts = append(out.Parts, core.Part{Kind: core.PART_KIND_TOOL_USE, ToolUse: &toolCall})
			}
		}
	}
	if len(out.Parts) > 0 {
		out = out.NormalizeContent()
	}
	return out, nil
}

func (r requestBody) validate() error {
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("at least one message is required")
	}
	for i, message := range r.Messages {
		switch message.Role {
		case "system", "user", "assistant", "tool":
		default:
			return fmt.Errorf("message[%d] role %q must be system|user|assistant|tool", i, message.Role)
		}
		switch message.Content.(type) {
		case nil, string, []contentPart:
		default:
			return fmt.Errorf("message[%d] content must be string or []contentPart, got %T", i, message.Content)
		}
	}
	return nil
}

func toChatMessages(messages []core.Message) ([]chatMessage, error) {
	out := make([]chatMessage, 0, len(messages))
	for _, message := range messages {
		role := "user"
		switch message.Role {
		case core.ROLE_SYSTEM:
			role = "system"
		case core.ROLE_ASSISTANT:
			role = "assistant"
		case core.ROLE_TOOL:
			role = "tool"
		}
		text, images, toolCalls, toolResults, err := flattenMessage(message)
		if err != nil {
			return nil, err
		}
		item := chatMessage{Role: role, Content: textContent(text), ToolCalls: toolCalls}
		if len(images) > 0 {
			item.Content = multimodalContent(text, images)
		}
		if len(toolResults) > 0 {
			item.ToolCallID = toolResults[0].callID
			item.Content = textContent(toolResults[0].outputString())
			item.Name = toolResults[0].name
		}
		out = append(out, item)
	}
	return out, nil
}

type flatToolResult struct {
	callID string
	name   string
	output any
}

func (r flatToolResult) outputString() string {
	if text, ok := r.output.(string); ok {
		return text
	}
	return marshalString(r.output)
}

// flattenMessage pulls the bits the wire shape carries out of one message:
// concatenated text, inline images as data URIs, assistant-side tool calls,
// and tool results.
//
// core.Part.Image holds raw decoded bytes, so they are base64-encoded here.
// An empty ImageMIME falls back to image/jpeg, the format every local
// vision model accepts.
func flattenMessage(message core.Message) (string, []imageURL, []toolCall, []flatToolResult, error) {
	var text strings.Builder
	var images []imageURL
	var toolCalls []toolCall
	var toolResults []flatToolResult
	for _, part := range message.Parts {
		switch part.Kind {
		case core.PART_KIND_PLAIN_TEXT:
			text.WriteString(part.Text)
		case core.PART_KIND_REASONING:
			return "", nil, nil, nil, fmt.Errorf("OpenAI Chat cannot preserve reasoning continuation metadata")
		case core.PART_KIND_IMAGE:
			if len(part.Image) > 0 {
				mime := part.ImageMIME
				if mime == "" {
					mime = "image/jpeg"
				}
				images = append(images, imageURL{
					URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(part.Image),
				})
			}
		case core.PART_KIND_TOOL_USE:
			if part.ToolUse != nil {
				call := toolCall{ID: part.ToolUse.ID, Type: "function"}
				call.Function.Name = part.ToolUse.Name
				call.Function.Arguments = marshalString(part.ToolUse.Args)
				toolCalls = append(toolCalls, call)
			}
		case core.PART_KIND_TOOL_RESULT:
			if part.ToolResult != nil {
				toolResults = append(toolResults, flatToolResult{
					callID: part.ToolResult.CallID,
					name:   part.ToolResult.Name,
					output: part.ToolResult.Output,
				})
			}
		}
	}
	return text.String(), images, toolCalls, toolResults, nil
}

func toToolDefs(specs []core.ToolSpec) []toolDef {
	out := make([]toolDef, 0, len(specs))
	for _, spec := range specs {
		definition := toolDef{Type: "function"}
		definition.Function.Name = spec.Name
		definition.Function.Description = spec.Description
		if raw, ok := spec.Parameters.(json.RawMessage); ok {
			definition.Function.Parameters = raw
		}
		out = append(out, definition)
	}
	return out
}

func maxTokensOrDefault(req core.ModelRequest) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return 4096
}

// marshalString deliberately preserves the adapters' previous lossy behavior:
// unsupported values become an empty string and are left for the upstream wire
// validator to reject.
func marshalString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
