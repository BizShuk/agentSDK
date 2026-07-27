package openaichat

import (
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
	body := requestBody{
		Model:     model,
		Messages:  toChatMessages(req.Messages),
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
			PromptTokens:     body.Usage.PromptTokens,
			CompletionTokens: body.Usage.CompletionTokens,
			TotalTokens:      body.Usage.TotalTokens,
		},
	}
	for _, choice := range body.Choices {
		out.Text += choice.Message.Content
		out.StopReason = choice.FinishReason
		for _, call := range choice.Message.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, core.ToolCall{
				ID:   call.ID,
				Name: call.Function.Name,
				Args: parseArgs(call.Function.Arguments),
			})
		}
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
	}
	return nil
}

func toChatMessages(messages []core.Message) []chatMessage {
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
		text, toolCalls, toolResults := flattenMessage(message)
		item := chatMessage{Role: role, Content: text, ToolCalls: toolCalls}
		if len(toolResults) > 0 {
			item.ToolCallID = toolResults[0].callID
			item.Content = toolResults[0].outputString()
			item.Name = toolResults[0].name
		}
		out = append(out, item)
	}
	return out
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

func flattenMessage(message core.Message) (string, []toolCall, []flatToolResult) {
	var text strings.Builder
	var toolCalls []toolCall
	var toolResults []flatToolResult
	for _, part := range message.Parts {
		switch part.Kind {
		case core.PART_KIND_PLAIN_TEXT:
			text.WriteString(part.Text)
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
	return text.String(), toolCalls, toolResults
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
