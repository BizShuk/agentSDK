package minimax

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

const videoFormatMP4 = "mp4"

func validateVideoFormat(format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", videoFormatMP4:
		return nil
	default:
		return fmt.Errorf("minimax video output format %q is not supported", format)
	}
}

func (p *VideoProvider) downloadVideo(
	ctx context.Context,
	auth core.Auth,
	rawURL string,
	outputPath string,
) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("minimax video: parse download URL: %w", err)
	}
	if !parsed.IsAbs() {
		return fmt.Errorf("minimax video: download URL must be absolute")
	}

	downloadCtx := ctx
	cancel := func() {}
	if p.downloadTimeout > 0 {
		downloadCtx, cancel = context.WithTimeout(ctx, p.downloadTimeout)
	}
	defer cancel()

	request, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("minimax video: build download request: %w", err)
	}
	applyVideoAuthHeaders(request, auth)

	client := *p.client
	client.Timeout = p.downloadTimeout
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("minimax video: download: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, maxVideoErrorBytes))
		return fmt.Errorf(
			"minimax video: download: status=%d body=%q",
			response.StatusCode,
			string(snippet),
		)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("minimax video: create output directory: %w", err)
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("minimax video: create output: %w", err)
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		_ = file.Close()
		return fmt.Errorf("minimax video: write output: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("minimax video: close output: %w", err)
	}
	return nil
}

func verifyVideoFile(path string, format string) error {
	if err := validateVideoFormat(format); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("minimax video: stat output: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("minimax video: output is empty")
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("minimax video: open output: %w", err)
	}
	defer file.Close()
	header := make([]byte, 8)
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("minimax video: read output header: %w", err)
	}
	if string(header[4:8]) != "ftyp" {
		return fmt.Errorf("minimax video: output is not an MP4 (missing ftyp box)")
	}
	return nil
}
