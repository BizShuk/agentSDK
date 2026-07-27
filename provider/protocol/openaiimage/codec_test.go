package openaiimage_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/protocol/openaiimage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeRequestUsesDefaultsAndExtras(t *testing.T) {
	raw, err := openaiimage.EncodeRequest(provider.ImageRequest{
		Prompt: "draw a fox",
		Extra: map[string]json.RawMessage{
			"resolution": json.RawMessage(`"2k"`),
		},
	}, "image-default")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"image-default",
		"prompt":"draw a fox",
		"resolution":"2k"
	}`, string(raw))
}

func TestEncodeRequestRejectsInvalidInputs(t *testing.T) {
	invalidCompression := 101
	cases := []struct {
		name    string
		request provider.ImageRequest
		model   string
		want    string
	}{
		{
			name:    "prompt",
			request: provider.ImageRequest{},
			model:   "image-model",
			want:    "image prompt is required",
		},
		{
			name:    "model",
			request: provider.ImageRequest{Prompt: "draw"},
			want:    "image model is required",
		},
		{
			name: "compression",
			request: provider.ImageRequest{
				Prompt:            "draw",
				OutputCompression: &invalidCompression,
			},
			model: "image-model",
			want:  "between 0 and 100",
		},
		{
			name: "reserved extra",
			request: provider.ImageRequest{
				Prompt: "draw",
				Extra: map[string]json.RawMessage{
					"model": json.RawMessage(`"shadow-model"`),
				},
			},
			model: "image-model",
			want:  "conflicts with a standard field",
		},
		{
			name: "invalid extra",
			request: provider.ImageRequest{
				Prompt: "draw",
				Extra: map[string]json.RawMessage{
					"resolution": json.RawMessage(`not-json`),
				},
			},
			model: "image-model",
			want:  "is not valid JSON",
		},
		{
			name: "streaming extra",
			request: provider.ImageRequest{
				Prompt: "draw",
				Extra: map[string]json.RawMessage{
					"stream": json.RawMessage(`true`),
				},
			},
			model: "image-model",
			want:  "requires unsupported streaming behavior",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := openaiimage.EncodeRequest(tc.request, tc.model)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestDecodeResponseSupportsBase64AndURL(t *testing.T) {
	result, err := openaiimage.DecodeResponse([]byte(`{
		"created":1713833628,
		"data":[
			{"b64_json":"aW1hZ2U=","mime_type":"image/png","revised_prompt":"revised"},
			{"url":"https://example.test/image.png","mime_type":"image/png"}
		],
		"usage":{
			"total_tokens":100,
			"input_tokens":50,
			"output_tokens":50,
			"input_tokens_details":{"text_tokens":10,"image_tokens":40},
			"cost_in_usd_ticks":200000000
		}
	}`))
	require.NoError(t, err)
	assert.Equal(t, provider.ImageResult{
		Created: 1713833628,
		Images: []provider.Image{
			{Base64: "aW1hZ2U=", MIMEType: "image/png", RevisedPrompt: "revised"},
			{URL: "https://example.test/image.png", MIMEType: "image/png"},
		},
		Usage: provider.ImageUsage{
			TotalTokens:  100,
			InputTokens:  50,
			OutputTokens: 50,
			InputTokenDetails: provider.ImageInputTokenDetails{
				TextTokens:  10,
				ImageTokens: 40,
			},
			CostInUSDTicks: 200000000,
		},
	}, result)
}

func TestDecodeResponseRejectsMissingImagePayload(t *testing.T) {
	_, err := openaiimage.DecodeResponse([]byte(`{"data":[{}]}`))
	require.Error(t, err)
	assert.ErrorContains(t, err, "neither url nor b64_json")
}

func TestDecodeAPIErrorIsStructuredAndBounded(t *testing.T) {
	longMessage := strings.Repeat("x", openaiimage.MAX_ERROR_MESSAGE_RUNES+20)
	raw, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": longMessage,
			"type":    "image_generation_user_error",
			"code":    "moderation_blocked",
			"moderation_details": map[string]any{
				"moderation_stage": "input",
			},
			"secret": "must-not-leak",
		},
	})
	require.NoError(t, err)

	err = openaiimage.DecodeAPIError("grok", "generate image", 400, "req-1", raw)
	var apiErr *provider.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.Equal(t, "moderation_blocked", apiErr.Code)
	assert.Equal(t, "image_generation_user_error", apiErr.Type)
	assert.Equal(t, "req-1", apiErr.RequestID)
	assert.Len(t, []rune(apiErr.Message), openaiimage.MAX_ERROR_MESSAGE_RUNES)
	assert.NotContains(t, err.Error(), "must-not-leak")
	assert.JSONEq(t, `{"moderation_stage":"input"}`, string(apiErr.Details))
}

func TestDecodeAPIErrorOmitsOversizedDetails(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": "blocked",
			"moderation_details": map[string]any{
				"payload": strings.Repeat("x", openaiimage.MAX_ERROR_DETAILS_BYTES),
			},
		},
	})
	require.NoError(t, err)

	err = openaiimage.DecodeAPIError("google", "generate image", 400, "", raw)
	var apiErr *provider.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Empty(t, apiErr.Details)
}
