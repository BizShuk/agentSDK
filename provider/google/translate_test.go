package google

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	genai "google.golang.org/genai"
)

// TestFromGenaiResponseParsesTextAndFunctionCall verifies the response
// translation: text content + a function call must round-trip into
// core.ModelResult, and the tool call's ID must come from
// FunctionCall.ID (not the name) when present — so downstream
// tool-result pairing works.
func TestFromGenaiResponseParsesTextAndFunctionCall(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{
				genai.NewPartFromText("I'll read the log."),
				{FunctionCall: &genai.FunctionCall{
					ID: "fc-42", Name: "read_log_tail", Args: map[string]any{"n": float64(5)},
				}},
			}},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 8,
			TotalTokenCount:      18,
		},
	}
	mr := fromGenaiResponse(resp)
	assertContains(t, mr.Text, "read the log")
	assertEqual(t, "STOP", mr.StopReason)
	assertEqual(t, 18, mr.Usage.TotalTokens)
	if len(mr.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(mr.ToolCalls))
	}
	tc := mr.ToolCalls[0]
	assertEqual(t, "fc-42", tc.ID)
	assertEqual(t, "read_log_tail", tc.Name)
	if n, ok := tc.Args["n"].(float64); !ok || n != 5 {
		t.Errorf("expected args[n]=5 (float64), got %v", tc.Args["n"])
	}
}

// TestFromGenaiResponseFallsBackToNameForID covers the legacy / single-
// call case where Gemini omits the function-call id: the call id falls
// back to the name so pairing still works.
func TestFromGenaiResponseFallsBackToNameForID(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{Name: "notify", Args: map[string]any{}}},
			}},
		}},
	}
	mr := fromGenaiResponse(resp)
	if len(mr.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(mr.ToolCalls))
	}
	assertEqual(t, "notify", mr.ToolCalls[0].ID)
	assertEqual(t, "notify", mr.ToolCalls[0].Name)
}

// TestToGenaiPartsForwardsTextAndImage verifies the multimodal forward
// path: text parts + an image part (with MIME) survive into genai parts.
// This is the provider-side half of the "Image chunk survives" contract.
func TestToGenaiPartsForwardsTextAndImage(t *testing.T) {
	msgs := []core.Message{{
		Role: core.ROLE_USER,
		Parts: []core.Part{
			{Kind: core.PART_KIND_PLAIN_TEXT, Text: "look at this"},
			{Kind: core.PART_KIND_IMAGE, ImageMIME: "image/png", Image: []byte{0x89, 0x50}},
		},
	}}
	parts := toGenaiParts(msgs)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].Text != "look at this" {
		t.Errorf("text part lost: %q", parts[0].Text)
	}
	if parts[1].InlineData == nil {
		t.Error("image part lost its inline data")
	}
}

// TestToGenaiToolsCarriesNameAndDescription verifies tool declarations
// carry the name + description; the parameters schema is best-effort.
func TestToGenaiToolsCarriesNameAndDescription(t *testing.T) {
	specs := []core.ToolSpec{{
		Name:        "read_log_tail",
		Description: "read the log tail",
		Parameters:  []byte(`{"type":"object","properties":{"n":{"type":"integer"}}}`),
		Risk:        core.RISK_LEVEL_LOW,
	}}
	tools := toGenaiTools(specs)
	if len(tools) != 1 || len(tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 function declaration, got %v", tools)
	}
	fd := tools[0].FunctionDeclarations[0]
	assertEqual(t, "read_log_tail", fd.Name)
	assertEqual(t, "read the log tail", fd.Description)
}

// assertEqual / assertContains are tiny local helpers so this internal
// test file does not pull testify (keeps the genai mock tests dependency-
// free, matching the provider's stdlib lean).
func assertEqual[T comparable](t *testing.T, want, got T) {
	t.Helper()
	if want != got {
		t.Errorf("want %v, got %v", want, got)
	}
}

func assertContains(t *testing.T, s, sub string) {
	t.Helper()
	if !contains(s, sub) {
		t.Errorf("want %q to contain %q", s, sub)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
