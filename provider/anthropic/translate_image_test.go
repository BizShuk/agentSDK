package anthropic

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
)

func TestToMessageParamsEncodesImageParts(t *testing.T) {
	payload := []byte{0x89, 0x50, 0x4e, 0x47}
	params, err := toMessageParams([]core.Message{
		{
			Role: core.ROLE_USER,
			Parts: []core.Part{
				{Kind: core.PART_KIND_PLAIN_TEXT, Text: "what is in this picture?"},
				{Kind: core.PART_KIND_IMAGE, Image: payload, ImageMIME: "image/png"},
				{Kind: core.PART_KIND_IMAGE, Image: payload},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, params, 1)
	require.Len(t, params[0].Content, 3)

	image := params[0].Content[1]
	assert.Equal(t, "image", image.Type)
	require.NotNil(t, image.Source)
	assert.Equal(t, "base64", image.Source.Type)
	assert.Equal(t, "image/png", image.Source.MediaType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(payload), image.Source.Data)

	assert.Equal(t, "image/jpeg", params[0].Content[2].Source.MediaType,
		"missing MIME falls back to image/jpeg")
}

func TestToMessageParamsSkipsEmptyImagePart(t *testing.T) {
	params, err := toMessageParams([]core.Message{
		{
			Role: core.ROLE_USER,
			Parts: []core.Part{
				{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"},
				{Kind: core.PART_KIND_IMAGE},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, params, 1)
	assert.Len(t, params[0].Content, 1)
}
