package provider

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

// Image executes an image generation request against the target provider.
func Image(ctx context.Context, req Request, out io.Writer) error {
	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	generator, err := provider.NewImage(req.Provider, req.Options)
	if err != nil {
		return err
	}
	result, err := generator.GenerateImage(ctx, provider.ImageRequest{Prompt: req.Prompt})
	if err != nil {
		return fmt.Errorf("image: %w", err)
	}
	if req.JSON {
		return WriteJSON(out, result)
	}
	for index, image := range result.Images {
		if image.URL != "" {
			if _, err := fmt.Fprintf(out, "image[%d].url=%s\n", index, image.URL); err != nil {
				return fmt.Errorf("write image URL: %w", err)
			}
		}
		if image.Base64 != "" {
			if _, err := fmt.Fprintf(
				out,
				"image[%d].base64_chars=%d mime=%s\n",
				index,
				len(image.Base64),
				image.MIMEType,
			); err != nil {
				return fmt.Errorf("write image metadata: %w", err)
			}
		}
		if image.RevisedPrompt != "" {
			if _, err := fmt.Fprintf(
				out,
				"image[%d].revised_prompt=%s\n",
				index,
				image.RevisedPrompt,
			); err != nil {
				return fmt.Errorf("write revised prompt: %w", err)
			}
		}
	}
	if _, err := fmt.Fprintf(
		out,
		"[images=%d tokens=%d cost_usd=%s cost_status=%s]\n",
		len(result.Images),
		result.Usage.TotalTokens,
		result.Cost.AmountUSD,
		result.Cost.Status,
	); err != nil {
		return fmt.Errorf("write image usage: %w", err)
	}
	return nil
}
