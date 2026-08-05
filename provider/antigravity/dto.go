package antigravity

// Wire format is Google Cloud Code v1internal: a thin envelope carrying a
// Gemini GenerateContent request. Claude models are served through the
// same envelope — the gateway translates them internally — so there is one
// request shape here, not two.
//
//	POST /v1internal:generateContent
//	POST /v1internal:streamGenerateContent?alt=sse
//	{
//	  "project": "...",
//	  "model": "gemini-3.6-flash-high",
//	  "request": { "contents": [...], "systemInstruction": {...}, ... },
//	  "userAgent": "antigravity",
//	  "requestType": "agent",
//	  "requestId": "agent-<uuid>"
//	}
//
// Responses arrive wrapped the same way: {"response": {"candidates": ...}}.

import "encoding/json"

// CloudCodeRequest is the v1internal envelope.
type CloudCodeRequest struct {
	Project     string          `json:"project"`
	Model       string          `json:"model"`
	Request     GenerateRequest `json:"request"`
	UserAgent   string          `json:"userAgent,omitempty"`
	RequestType string          `json:"requestType,omitempty"`
	RequestID   string          `json:"requestId,omitempty"`
}

// GenerateRequest is the Gemini GenerateContent body inside the envelope.
type GenerateRequest struct {
	Contents          []Content         `json:"contents"`
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
	Tools             []Tool            `json:"tools,omitempty"`
	ToolConfig        *ToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`

	// SessionID gives the gateway a prompt-cache key. It is stable for
	// the lifetime of one Provider, matching the IDE's per-launch id.
	SessionID string `json:"sessionId,omitempty"`
}

// Content is one turn: a role plus ordered parts. Gemini names the
// assistant role "model"; system text does not live here at all, it goes
// to GenerateRequest.SystemInstruction.
type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}

// Part is one fragment of a Content. Exactly one payload field is set per
// part; Thought and ThoughtSignature are modifiers on a text or
// functionCall part rather than payloads of their own.
type Part struct {
	Text             string            `json:"text,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	InlineData       *InlineData       `json:"inlineData,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
}

// InlineData carries base64 bytes with their media type — images, audio,
// and documents all ride this one shape.
type InlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

// FunctionCall is the model asking for a tool. ID is populated for Claude
// models, which match results to calls by id; Gemini matches by name.
type FunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// FunctionResponse is the tool's answer going back up.
type FunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// Tool wraps the declarations; the gateway takes a one-element array.
type Tool struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations"`
}

// FunctionDeclaration is one tool's contract. Parameters is a Google-
// dialect schema — see schema.go for the translation from JSON Schema.
type FunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolConfig pins the function-calling mode.
type ToolConfig struct {
	FunctionCallingConfig FunctionCallingConfig `json:"functionCallingConfig"`
}

// FunctionCallingConfig selects how strictly arguments are validated.
// "VALIDATED" is what the reference clients send for Claude models.
type FunctionCallingConfig struct {
	Mode string `json:"mode"`
}

// GenerationConfig holds sampling and thinking controls.
//
// ThinkingConfig is typed `any` because the gateway takes two different
// schemas behind one field name: snake_case for Claude models, camelCase
// for Gemini. They are distinct upstream messages, not a casing
// preference, so a single struct carrying both spellings would send each
// family fields the other side rejects.
type GenerationConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
	ThinkingConfig  any      `json:"thinkingConfig,omitempty"`
}

// ClaudeThinkingConfig is the Claude-family thinking control.
type ClaudeThinkingConfig struct {
	IncludeThoughts bool `json:"include_thoughts"`
	ThinkingBudget  int  `json:"thinking_budget"`
}

// GeminiThinkingConfig is the Gemini-family thinking control.
type GeminiThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts"`
	ThinkingBudget  int  `json:"thinkingBudget,omitempty"`
}

// ---------------------------------------------------------------------------
// responses
// ---------------------------------------------------------------------------

// Envelope is the response wrapper. The streaming endpoint always wraps;
// the blocking one sometimes returns the payload bare, which is why
// Unwrap falls back to decoding the same bytes as a GenerateResponse.
type Envelope struct {
	Response *GenerateResponse `json:"response,omitempty"`
	Error    *APIError         `json:"error,omitempty"`
}

// GenerateResponse is the Gemini response body.
type GenerateResponse struct {
	Candidates    []Candidate    `json:"candidates"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
}

// Candidate is one completion.
type Candidate struct {
	Content      Content `json:"content"`
	FinishReason string  `json:"finishReason,omitempty"`
	Index        int     `json:"index,omitempty"`
}

// UsageMetadata is the token accounting. ThoughtsTokenCount is billed
// output that does not appear in CandidatesTokenCount.
type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// APIError is the google.rpc.Status body returned on failure.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Unwrap decodes one response payload, accepting both the wrapped and the
// bare shape.
func Unwrap(raw []byte) (GenerateResponse, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Response != nil {
		return *env.Response, nil
	}
	var bare GenerateResponse
	if err := json.Unmarshal(raw, &bare); err != nil {
		return GenerateResponse{}, err
	}
	return bare, nil
}
