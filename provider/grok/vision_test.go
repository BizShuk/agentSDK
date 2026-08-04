package grok

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
)

func TestToChatMessagesEncodesImageParts(t *testing.T) {
	payload := []byte{0xff, 0xd8, 0xff}
	messages, err := toChatMessages([]core.Message{
		{
			Role: core.ROLE_USER,
			Parts: []core.Part{
				{Kind: core.PART_KIND_PLAIN_TEXT, Text: "describe this"},
				{Kind: core.PART_KIND_IMAGE, Image: payload, ImageMIME: "image/png"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)

	parts, ok := messages[0].Content.([]ContentPart)
	require.True(t, ok, "image turn uses the multimodal array form")
	require.Len(t, parts, 2)
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "describe this", parts[0].Text)
	assert.Equal(t, "image_url", parts[1].Type)
	require.NotNil(t, parts[1].ImageURL)
	assert.Equal(t,
		"data:image/png;base64,"+base64.StdEncoding.EncodeToString(payload),
		parts[1].ImageURL.URL,
	)
}

func TestToChatMessagesTextOnlyStaysString(t *testing.T) {
	messages, err := toChatMessages([]core.Message{
		{Role: core.ROLE_USER, Parts: []core.Part{
			{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hello"},
		}},
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "hello", messages[0].Content)
}

func TestValidateContentUnion(t *testing.T) {
	valid := RequestBody{
		Model: "grok-3",
		Messages: []ChatMessage{
			{Role: "user", Content: multimodalContent("hi", []ImageURL{{URL: "data:image/jpeg;base64,aGk="}})},
		},
	}
	assert.NoError(t, valid.Validate())

	empty := RequestBody{
		Model:    "grok-3",
		Messages: []ChatMessage{{Role: "user", Content: []ContentPart{}}},
	}
	assert.ErrorContains(t, empty.Validate(), "empty content")

	wrongType := RequestBody{
		Model:    "grok-3",
		Messages: []ChatMessage{{Role: "user", Content: 42}},
	}
	assert.ErrorContains(t, wrongType.Validate(), "must be string or []ContentPart")
}
