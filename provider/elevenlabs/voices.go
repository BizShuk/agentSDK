package elevenlabs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: the speech surface carries the voice catalog, like ListModels.
var _ provider.VoiceLister = (*SpeechProvider)(nil)

// voicesPath is the v2 voice-search endpoint. Wire contract:
// https://elevenlabs.io/docs/api-reference/voices/search
const voicesPath = "/v2/voices"

// voiceListPageSizeMax is the vendor's documented page_size ceiling.
const voiceListPageSizeMax = 100

// voiceListResponse is the subset of the search response this adapter folds.
type voiceListResponse struct {
	Voices []struct {
		VoiceID       string            `json:"voice_id"`
		Name          string            `json:"name"`
		Category      string            `json:"category"`
		Labels        map[string]string `json:"labels"`
		Description   string            `json:"description"`
		PreviewURL    string            `json:"preview_url"`
		CreatedAtUnix int64             `json:"created_at_unix"`
	} `json:"voices"`
	HasMore       bool   `json:"has_more"`
	TotalCount    int    `json:"total_count"`
	NextPageToken string `json:"next_page_token"`
}

// ListVoices implements provider.VoiceLister against GET /v2/voices. Search,
// category, and pagination are all served vendor-side; the request maps onto
// query parameters one-to-one.
func (p *SpeechProvider) ListVoices(
	ctx context.Context,
	request provider.VoiceListRequest,
) (provider.VoiceListResult, error) {
	if err := request.Validate(); err != nil {
		return provider.VoiceListResult{}, err
	}
	if request.PageSize > voiceListPageSizeMax {
		return provider.VoiceListResult{}, fmt.Errorf(
			"elevenlabs voice page size %d exceeds limit of %d",
			request.PageSize,
			voiceListPageSizeMax,
		)
	}

	query := url.Values{}
	if search := strings.TrimSpace(request.Search); search != "" {
		query.Set("search", search)
	}
	if category := strings.TrimSpace(request.Category); category != "" {
		query.Set("category", category)
	}
	if request.PageSize > 0 {
		query.Set("page_size", strconv.Itoa(request.PageSize))
	}
	if token := strings.TrimSpace(request.PageToken); token != "" {
		query.Set("next_page_token", token)
	}
	endpoint := p.baseURL + voicesPath
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return provider.VoiceListResult{}, fmt.Errorf(
			"elevenlabs list voices: build request: %w",
			err,
		)
	}
	applyAuthHeaders(httpReq, p.auth.Merge(request.Auth))
	httpReq.Header.Set("Accept", "application/json")

	response, err := p.client.Do(httpReq)
	if err != nil {
		return provider.VoiceListResult{}, fmt.Errorf(
			"elevenlabs list voices: request: %w",
			err,
		)
	}
	defer response.Body.Close()

	id := requestID(response.Header)
	if response.StatusCode/100 != 2 {
		raw, readErr := readErrorResponse("list voices", response.Body, maxErrorResponseBytes)
		if readErr != nil {
			return provider.VoiceListResult{}, readErr
		}
		return provider.VoiceListResult{}, decodeAPIError(
			"list voices",
			response.StatusCode,
			id,
			raw,
		)
	}

	raw, err := readBoundedResponse("list voices", response.Body, maxTranscriptResponseBytes)
	if err != nil {
		return provider.VoiceListResult{}, err
	}
	var decoded voiceListResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return provider.VoiceListResult{}, fmt.Errorf(
			"elevenlabs list voices: decode response: %w",
			err,
		)
	}
	return foldVoiceListResponse(decoded), nil
}

func foldVoiceListResponse(decoded voiceListResponse) provider.VoiceListResult {
	result := provider.VoiceListResult{
		HasMore:       decoded.HasMore,
		TotalCount:    decoded.TotalCount,
		NextPageToken: decoded.NextPageToken,
	}
	for _, voice := range decoded.Voices {
		result.Voices = append(result.Voices, provider.Voice{
			ID:            voice.VoiceID,
			Name:          voice.Name,
			Category:      voice.Category,
			Description:   voice.Description,
			Labels:        voice.Labels,
			PreviewURL:    voice.PreviewURL,
			CreatedAtUnix: voice.CreatedAtUnix,
		})
	}
	return result
}
