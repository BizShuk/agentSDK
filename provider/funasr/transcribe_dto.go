package funasr

import (
	"bytes"
	"fmt"
	"math"
	"mime/multipart"
	"strconv"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

// transcribeResponse is the server's verbose_json shape. `duration` is the
// server-side processing time, NOT the audio length, so it is deliberately
// not decoded — Usage falls back to the request-side duration instead.
type transcribeResponse struct {
	Text     string              `json:"text"`
	Language string              `json:"language"`
	Segments []transcribeSegment `json:"segments"`
}

// transcribeSegment carries seconds as float and a nullable speaker index.
type transcribeSegment struct {
	Text    string  `json:"text"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker *int    `json:"speaker"`
}

// encodeTranscribeRequest builds the multipart body. Only bytes input
// reaches this point: Transcribe rejects URL input up front.
func encodeTranscribeRequest(
	request provider.TranscribeRequest,
	model string,
) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", transcribeFileName(request.Audio.Format))
	if err != nil {
		return nil, "", fmt.Errorf("funasr transcribe: encode audio: %w", err)
	}
	if _, err := part.Write(request.Audio.Bytes); err != nil {
		return nil, "", fmt.Errorf("funasr transcribe: write audio: %w", err)
	}

	fields := [][2]string{
		{"model", model},
		// verbose_json is requested unconditionally: it is the only shape
		// that carries segments and detected language.
		{"response_format", "verbose_json"},
	}
	if language := strings.TrimSpace(request.Language); language != "" {
		fields = append(fields, [2]string{"language", language})
	}
	for _, field := range fields {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return nil, "", fmt.Errorf("funasr transcribe: encode %s: %w", field[0], err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("funasr transcribe: close request body: %w", err)
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

// transcribeFileName gives the upload a name the server reads a container
// format from. AudioSource.Format may be a composite label such as
// "pcm_16000", so only the leading token becomes the extension.
func transcribeFileName(format string) string {
	extension, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(format)), "_")
	if extension == "" {
		return "audio"
	}
	return "audio." + extension
}

// foldTranscribeResponse maps sentence-level segments into Words. FunASR
// segments are sentences, not words — coarser than ElevenLabs word timing
// but the same canonical shape.
func foldTranscribeResponse(response transcribeResponse) provider.TranscribeResult {
	result := provider.TranscribeResult{
		Text:     response.Text,
		Language: normalizeLanguage(response.Language),
	}
	if len(response.Segments) == 0 {
		return result
	}
	result.Words = make([]provider.TranscribedWord, 0, len(response.Segments))
	for _, segment := range response.Segments {
		word := provider.TranscribedWord{
			Text:    segment.Text,
			StartMs: secondsToMilliseconds(segment.Start),
			EndMs:   secondsToMilliseconds(segment.End),
		}
		if segment.Speaker != nil {
			word.Speaker = strconv.Itoa(*segment.Speaker)
		}
		result.Words = append(result.Words, word)
	}
	return result
}

// normalizeLanguage drops the server's "auto" placeholder: it means the
// language was not detected, and the canonical contract uses empty for that.
func normalizeLanguage(language string) string {
	if strings.EqualFold(strings.TrimSpace(language), "auto") {
		return ""
	}
	return language
}

func secondsToMilliseconds(seconds float64) int64 {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0
	}
	return int64(math.Round(seconds * 1000))
}
