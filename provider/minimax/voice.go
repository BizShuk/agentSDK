package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: the speech surface carries the voice catalog.
var _ provider.VoiceLister = (*SpeechProvider)(nil)

// voiceListPath is the voice-management endpoint. Wire contract:
// https://platform.minimax.io/docs/api-reference/voice-management-get
const voiceListPath = "/v1/get_voice"

const (
	maxVoiceResponseBytes      int64 = 32 << 20
	maxVoiceErrorResponseBytes int64 = 1 << 20
)

// voiceCategoryAll is the vendor's combined-categories filter and the default
// when the neutral request names no category.
const voiceCategoryAll = "all"

// voiceListRequest is the POST body. voice_type is the only parameter the
// endpoint takes.
type voiceListRequest struct {
	VoiceType string `json:"voice_type"`
}

// voiceListEntry is one voice in any of the response arrays. system_voice
// carries voice_name; cloned and generated voices only have an id.
type voiceListEntry struct {
	VoiceID     string   `json:"voice_id"`
	VoiceName   string   `json:"voice_name"`
	Description []string `json:"description"`
	CreatedTime string   `json:"created_time"`
}

// voiceBaseResponse mirrors the vendor's shared base_resp envelope.
type voiceBaseResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

type voiceListResponse struct {
	SystemVoice     []voiceListEntry  `json:"system_voice"`
	VoiceCloning    []voiceListEntry  `json:"voice_cloning"`
	VoiceGeneration []voiceListEntry  `json:"voice_generation"`
	BaseResp        voiceBaseResponse `json:"base_resp"`
}

// ListVoices implements provider.VoiceLister against POST /v1/get_voice.
//
// The endpoint has no server-side search or pagination: Search filters the
// full list locally, PageSize truncates it locally (HasMore reports the
// truncation, NextPageToken stays empty because there is nothing to resume
// from), and a non-empty PageToken is rejected rather than silently starting
// over.
func (p *SpeechProvider) ListVoices(
	ctx context.Context,
	request provider.VoiceListRequest,
) (provider.VoiceListResult, error) {
	if err := request.Validate(); err != nil {
		return provider.VoiceListResult{}, err
	}
	if strings.TrimSpace(request.PageToken) != "" {
		return provider.VoiceListResult{}, fmt.Errorf(
			"minimax voice list does not support pagination tokens",
		)
	}
	category, err := normalizeVoiceCategory(request.Category)
	if err != nil {
		return provider.VoiceListResult{}, err
	}
	auth := p.auth.Merge(request.Auth)
	if auth.Token() == "" {
		return provider.VoiceListResult{}, fmt.Errorf("minimax voice credential is required")
	}

	payload, err := json.Marshal(voiceListRequest{VoiceType: category})
	if err != nil {
		return provider.VoiceListResult{}, fmt.Errorf(
			"minimax voice list: encode request: %w",
			err,
		)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+voiceListPath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return provider.VoiceListResult{}, fmt.Errorf(
			"minimax voice list: build request: %w",
			err,
		)
	}
	applySpeechAuthHeaders(httpReq, auth)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(httpReq)
	if err != nil {
		return provider.VoiceListResult{}, fmt.Errorf(
			"minimax voice list: request: %w",
			err,
		)
	}
	defer response.Body.Close()

	id := speechRequestID(response.Header)
	if response.StatusCode/100 != 2 {
		raw, readErr := readVoiceErrorResponse(response.Body)
		if readErr != nil {
			return provider.VoiceListResult{}, readErr
		}
		return provider.VoiceListResult{}, decodeVoiceAPIError(response.StatusCode, id, raw)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxVoiceResponseBytes+1))
	if err != nil {
		return provider.VoiceListResult{}, fmt.Errorf(
			"minimax voice list: read response: %w",
			err,
		)
	}
	if int64(len(raw)) > maxVoiceResponseBytes {
		return provider.VoiceListResult{}, fmt.Errorf(
			"minimax voice list: read response: exceeds %d bytes",
			maxVoiceResponseBytes,
		)
	}
	var decoded voiceListResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return provider.VoiceListResult{}, fmt.Errorf(
			"minimax voice list: decode response: %w",
			err,
		)
	}
	if decoded.BaseResp.StatusCode != 0 {
		return provider.VoiceListResult{}, newVoiceAPIError(http.StatusOK, id, decoded.BaseResp)
	}
	return foldVoiceListResponse(decoded, request), nil
}

// normalizeVoiceCategory maps the neutral category onto the vendor's
// voice_type vocabulary; empty selects every class.
func normalizeVoiceCategory(value string) (string, error) {
	category := strings.ToLower(strings.TrimSpace(value))
	switch category {
	case "":
		return voiceCategoryAll, nil
	case "system", "voice_cloning", "voice_generation", voiceCategoryAll:
		return category, nil
	default:
		return "", fmt.Errorf(
			"minimax voice category %q must be system, voice_cloning, voice_generation, or all",
			value,
		)
	}
}

func foldVoiceListResponse(
	decoded voiceListResponse,
	request provider.VoiceListRequest,
) provider.VoiceListResult {
	var voices []provider.Voice
	for _, group := range []struct {
		category string
		entries  []voiceListEntry
	}{
		{"system", decoded.SystemVoice},
		{"voice_cloning", decoded.VoiceCloning},
		{"voice_generation", decoded.VoiceGeneration},
	} {
		for _, entry := range group.entries {
			voice := provider.Voice{
				ID:            entry.VoiceID,
				Name:          entry.VoiceName,
				Category:      group.category,
				Description:   strings.Join(entry.Description, "; "),
				CreatedAtUnix: parseVoiceCreatedTime(entry.CreatedTime),
			}
			if matchesVoiceSearch(voice, request.Search) {
				voices = append(voices, voice)
			}
		}
	}
	result := provider.VoiceListResult{Voices: voices, TotalCount: len(voices)}
	if request.PageSize > 0 && len(voices) > request.PageSize {
		result.Voices = voices[:request.PageSize]
		result.HasMore = true
	}
	return result
}

// matchesVoiceSearch mirrors the vendor-side search of richer catalogs:
// case-insensitive substring across id, name, and description.
func matchesVoiceSearch(voice provider.Voice, search string) bool {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return true
	}
	for _, haystack := range []string{voice.ID, voice.Name, voice.Description} {
		if strings.Contains(strings.ToLower(haystack), needle) {
			return true
		}
	}
	return false
}

// parseVoiceCreatedTime resolves the vendor's yyyy-mm-dd date to UTC
// midnight; an absent or unparseable date stays zero.
func parseVoiceCreatedTime(value string) int64 {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed.Unix()
}

func readVoiceErrorResponse(reader io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxVoiceErrorResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("minimax voice list: read error response: %w", err)
	}
	return raw, nil
}

func decodeVoiceAPIError(statusCode int, id string, raw []byte) error {
	var envelope struct {
		BaseResp voiceBaseResponse `json:"base_resp"`
		Message  string            `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return &provider.APIError{
			Provider:   "minimax",
			Operation:  "list voices",
			StatusCode: statusCode,
			Message:    "upstream request failed",
			RequestID:  id,
		}
	}
	if envelope.BaseResp.StatusMsg == "" {
		envelope.BaseResp.StatusMsg = envelope.Message
	}
	return newVoiceAPIError(statusCode, id, envelope.BaseResp)
}

func newVoiceAPIError(statusCode int, id string, baseResp voiceBaseResponse) error {
	message := boundVoiceErrorMessage(baseResp.StatusMsg)
	if message == "" {
		message = "upstream request failed"
	}
	code := ""
	if baseResp.StatusCode != 0 {
		code = strconv.Itoa(baseResp.StatusCode)
	}
	return &provider.APIError{
		Provider:   "minimax",
		Operation:  "list voices",
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		RequestID:  id,
	}
}

func boundVoiceErrorMessage(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) <= maxSpeechErrorMessageRunes {
		return string(runes)
	}
	return string(runes[:maxSpeechErrorMessageRunes])
}
