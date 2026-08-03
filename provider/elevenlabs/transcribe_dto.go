package elevenlabs

import (
	"bytes"
	"fmt"
	"math"
	"mime/multipart"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

type transcribeResponse struct {
	LanguageCode        string           `json:"language_code"`
	LanguageProbability float64          `json:"language_probability"`
	Text                string           `json:"text"`
	Words               []transcribeWord `json:"words"`
}

// transcribeWord carries seconds as float; provider.TranscribedWord is
// integer milliseconds so callers never compare floats to find a boundary.
type transcribeWord struct {
	Text      string  `json:"text"`
	Start     float64 `json:"start"`
	End       float64 `json:"end"`
	Type      string  `json:"type"`
	SpeakerID string  `json:"speaker_id"`
}

// encodeTranscribeRequest builds the multipart body. Bytes go in the `file`
// part and a URL goes in `cloud_storage_url`; TranscribeRequest.Validate has
// already established that exactly one of them is set.
func encodeTranscribeRequest(
	request provider.TranscribeRequest,
	model string,
) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if len(request.Audio.Bytes) > 0 {
		part, err := writer.CreateFormFile("file", transcribeFileName(request.Audio.Format))
		if err != nil {
			return nil, "", fmt.Errorf("elevenlabs transcribe: encode audio: %w", err)
		}
		if _, err := part.Write(request.Audio.Bytes); err != nil {
			return nil, "", fmt.Errorf("elevenlabs transcribe: write audio: %w", err)
		}
	} else if err := writer.WriteField(
		"cloud_storage_url",
		strings.TrimSpace(request.Audio.URL),
	); err != nil {
		return nil, "", fmt.Errorf("elevenlabs transcribe: encode audio URL: %w", err)
	}

	fields := [][2]string{{"model_id", model}}
	if language := strings.TrimSpace(request.Language); language != "" {
		fields = append(fields, [2]string{"language_code", language})
	}
	if request.Diarize {
		fields = append(fields, [2]string{"diarize", "true"})
	}
	for _, field := range fields {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return nil, "", fmt.Errorf(
				"elevenlabs transcribe: encode %s: %w",
				field[0],
				err,
			)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("elevenlabs transcribe: close request body: %w", err)
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

// transcribeFileName gives the upload a name the vendor can read a container
// from. AudioSource.Format may be a composite label such as "pcm_16000", so
// only the leading token becomes the extension.
func transcribeFileName(format string) string {
	extension, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(format)), "_")
	if extension == "" {
		return "audio"
	}
	return "audio." + extension
}

func foldTranscribeResponse(response transcribeResponse) provider.TranscribeResult {
	result := provider.TranscribeResult{
		Text:     response.Text,
		Language: response.LanguageCode,
	}
	if len(response.Words) == 0 {
		return result
	}
	// Words are kept verbatim, including the vendor's "spacing" entries: a
	// caller aligning a transcript to audio needs the gaps, and one that does
	// not can filter on Text.
	result.Words = make([]provider.TranscribedWord, 0, len(response.Words))
	for _, word := range response.Words {
		result.Words = append(result.Words, provider.TranscribedWord{
			Text:    word.Text,
			StartMs: secondsToMilliseconds(word.Start),
			EndMs:   secondsToMilliseconds(word.End),
			Speaker: word.SpeakerID,
		})
	}
	return result
}

func secondsToMilliseconds(seconds float64) int64 {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0
	}
	return int64(math.Round(seconds * 1000))
}
