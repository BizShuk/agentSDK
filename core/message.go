package core

import "time"

// Role identifies the speaker of a Message.
type Role string

const (
	ROLE_SYSTEM    Role = "system"
	ROLE_USER      Role = "user"
	ROLE_ASSISTANT Role = "assistant"
	ROLE_TOOL      Role = "tool"
)

// PartKind is the discriminator for multimodal parts.
type PartKind string

const (
	PART_KIND_PLAIN_TEXT  PartKind = "plain_text"
	PART_KIND_AUDIO       PartKind = "audio"
	PART_KIND_IMAGE       PartKind = "image"
	PART_KIND_TOOL_USE    PartKind = "tool_use"
	PART_KIND_TOOL_RESULT PartKind = "tool_result"
)

// Part is one fragment of a Message — text, audio, image, tool_use, or tool_result.
// Decide does not inspect Parts directly; the LLM provider deals with conversion.
// runtime passes Parts through unchanged so providers see a faithful transcript.
type Part struct {
	Kind       PartKind    `json:"kind"`
	Text       string      `json:"text,omitempty"`
	Audio      []byte      `json:"audio,omitempty"`
	AudioMIME  string      `json:"audio_mime,omitempty"`
	Image      []byte      `json:"image,omitempty"`
	ImageMIME  string      `json:"image_mime,omitempty"`
	ToolUse    *ToolCall   `json:"tool_use,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}

// Message is a turn of the conversation — one role, many parts.
type Message struct {
	Role  Role      `json:"role"`
	Parts []Part    `json:"parts"`
	Ts    time.Time `json:"ts"`
}

// AppendText returns a message with a new plain-text Part added (helper for tests/sample).
func (m Message) AppendText(s string) Message {
	out := m
	out.Parts = append(append([]Part(nil), m.Parts...), Part{Kind: PART_KIND_PLAIN_TEXT, Text: s})
	return out
}
