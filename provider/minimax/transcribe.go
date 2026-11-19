package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// Wire contract: https://platform.minimax.io/docs/api-reference/speech-to-text
const (
	transcribePath                  = "/v1/speech_to_text"
	DefaultTranscribeModel          = "asr-1.0"
	defaultTranscribeRequestTimeout = 10 * time.Minute
	maxTranscribeAudioBytes         = 50 << 20
	maxTranscribeAudioMilliseconds  = 500_000
	maxTranscribeResponseBytes      = 16 << 20
)

// TranscribeProvider implements provider.Transcriber against MiniMax's
// speech_to_text endpoint. Every request asks for verbose_json: it is the
// only response shape carrying sentence timing and speaker labels, which fold
// into TranscribeResult.Words so callers can render subtitles themselves.
type TranscribeProvider struct {
	rest  restClient
	model string
	auth  core.Auth
}

// NewTranscriber returns a MiniMax transcriber from registry-resolved config.
func NewTranscriber(cfg provider.ResolvedConfig) (*TranscribeProvider, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTranscribeRequestTimeout
	}
	return &TranscribeProvider{
		rest:  restClient{baseURL: accountBaseURL(cfg.BaseURL), client: &http.Client{Timeout: timeout}},
		model: strings.TrimSpace(cfg.Model),
		auth:  cfg.Auth,
	}, nil
}

type transcribeSegment struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker any     `json:"speaker"`
	Text    string  `json:"text"`
}

type transcribeResponse struct {
	Text     string              `json:"text"`
	Duration float64             `json:"duration"`
	Segments []transcribeSegment `json:"segments"`
}

// Transcribe converts one recording into text.
func (p *TranscribeProvider) Transcribe(
	ctx context.Context,
	request provider.TranscribeRequest,
) (provider.TranscribeResult, error) {
	if err := request.Validate(); err != nil {
		return provider.TranscribeResult{}, err
	}
	if request.Audio.URL != "" {
		return provider.TranscribeResult{}, fmt.Errorf("minimax transcribe: audio URL is not supported; send bytes")
	}
	if len(request.Audio.Bytes) > maxTranscribeAudioBytes {
		return provider.TranscribeResult{}, fmt.Errorf("minimax transcribe: audio exceeds %d bytes", maxTranscribeAudioBytes)
	}
	if request.Audio.DurationMilliseconds > maxTranscribeAudioMilliseconds {
		return provider.TranscribeResult{}, fmt.Errorf("minimax transcribe: audio exceeds %d seconds", maxTranscribeAudioMilliseconds/1000)
	}
	auth := p.auth.Merge(request.Auth)
	if auth.Token() == "" {
		return provider.TranscribeResult{}, fmt.Errorf("minimax transcribe credential is required")
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = DefaultTranscribeModel
	}

	body, contentType, err := encodeTranscribeRequest(request, model)
	if err != nil {
		return provider.TranscribeResult{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.rest.baseURL+transcribePath, bytes.NewReader(body))
	if err != nil {
		return provider.TranscribeResult{}, fmt.Errorf("minimax transcribe: build request: %w", err)
	}
	applySpeechAuthHeaders(httpRequest, auth)
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", contentType)
	if language := strings.TrimSpace(request.Language); language != "" {
		httpRequest.Header.Set("language", language)
	}
	raw, err := p.rest.do(httpRequest, "transcribe", maxTranscribeResponseBytes)
	if err != nil {
		return provider.TranscribeResult{}, err
	}
	var decoded transcribeResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return provider.TranscribeResult{}, fmt.Errorf("minimax transcribe: decode response: %w", err)
	}
	return foldTranscribeResponse(decoded, request.Language), nil
}

func encodeTranscribeRequest(request provider.TranscribeRequest, model string) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	format := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(request.Audio.Format)), ".")
	if format == "" {
		format = "mp3"
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="audio.%s"`, format))
	header.Set("Content-Type", "application/octet-stream")
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", fmt.Errorf("minimax transcribe: encode file: %w", err)
	}
	if _, err := part.Write(request.Audio.Bytes); err != nil {
		return nil, "", fmt.Errorf("minimax transcribe: encode file: %w", err)
	}
	for key, value := range map[string]string{"model": model, "response_format": "verbose_json"} {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", fmt.Errorf("minimax transcribe: encode %s: %w", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("minimax transcribe: encode request: %w", err)
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

// foldTranscribeResponse maps sentence segments into Words: MiniMax's
// verbose_json is sentence-level, so each "word" is one timed sentence.
func foldTranscribeResponse(response transcribeResponse, language string) provider.TranscribeResult {
	words := make([]provider.TranscribedWord, 0, len(response.Segments))
	for _, segment := range response.Segments {
		words = append(words, provider.TranscribedWord{
			Text:    segment.Text,
			StartMs: int64(segment.Start * 1000),
			EndMs:   int64(segment.End * 1000),
			Speaker: speakerLabel(segment.Speaker),
		})
	}
	return provider.TranscribeResult{
		Text:     response.Text,
		Language: language,
		Words:    words,
		Usage:    provider.TranscribeUsage{AudioDurationMilliseconds: int64(response.Duration * 1000)},
	}
}

// speakerLabel normalizes the vendor's speaker field, which arrives as a
// number in examples and may be a string; empty means unlabeled.
func speakerLabel(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return fmt.Sprintf("%d", int(v))
	default:
		return fmt.Sprint(v)
	}
}
