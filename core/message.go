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

// ChunkKind is the discriminator for multimodal chunks.
type ChunkKind string

const (
	CHUNK_KIND_TEXT       ChunkKind = "text"
	CHUNK_KIND_AUDIO      ChunkKind = "audio"
	CHUNK_KIND_IMAGE      ChunkKind = "image"
	CHUNK_KIND_TOOL_USE   ChunkKind = "tool_use"
	CHUNK_KIND_TOOL_RESULT ChunkKind = "tool_result"
)

// ToolResultChunk is an embedded tool result inside an assistant-style message.
type ToolResultChunk struct {
	CallID string `json:"call_id"`
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Output any    `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Chunk is one fragment of a Message — text, audio, image, tool_use, or tool_result.
// Step does not inspect Chunks directly; the LLM provider deals with conversion.
// runtime passes Chunks through unchanged so providers see a faithful transcript.
type Chunk struct {
	Kind       ChunkKind       `json:"kind"`
	Text       string          `json:"text,omitempty"`
	Audio      []byte          `json:"audio,omitempty"`
	AudioMIME  string          `json:"audio_mime,omitempty"`
	Image      []byte          `json:"image,omitempty"`
	ImageMIME  string          `json:"image_mime,omitempty"`
	ToolUse    *ToolUseChunk   `json:"tool_use,omitempty"`
	ToolResult *ToolResultChunk `json:"tool_result,omitempty"`
}

// Message is a turn of the conversation — one role, many chunks.
type Message struct {
	Role Role       `json:"role"`
	Chunks []Chunk  `json:"chunks"`
	Ts     time.Time `json:"ts"`
}

// AppendText returns a message with a new Text chunk added (helper for tests/sample).
func (m Message) AppendText(s string) Message {
	out := m
	out.Chunks = append(append([]Chunk(nil), m.Chunks...), Chunk{Kind: CHUNK_KIND_TEXT, Text: s})
	return out
}
