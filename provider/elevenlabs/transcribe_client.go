package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bizshuk/agentsdk/core"
)

const transcribePath = "/v1/speech-to-text"

func (p *TranscribeProvider) postTranscribe(
	ctx context.Context,
	auth core.Auth,
	payload []byte,
	contentType string,
) (transcribeResponse, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+transcribePath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return transcribeResponse{}, fmt.Errorf(
			"elevenlabs transcribe: build request: %w",
			err,
		)
	}
	applyAuthHeaders(request, auth)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", contentType)

	response, err := p.client.Do(request)
	if err != nil {
		return transcribeResponse{}, fmt.Errorf("elevenlabs transcribe: request: %w", err)
	}
	defer response.Body.Close()

	id := requestID(response.Header)
	if response.StatusCode/100 != 2 {
		raw, readErr := readErrorResponse("transcribe", response.Body, maxErrorResponseBytes)
		if readErr != nil {
			return transcribeResponse{}, readErr
		}
		return transcribeResponse{}, decodeAPIError(
			"transcribe",
			response.StatusCode,
			id,
			raw,
		)
	}

	raw, err := readBoundedResponse("transcribe", response.Body, maxTranscriptResponseBytes)
	if err != nil {
		return transcribeResponse{}, err
	}
	var decoded transcribeResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return transcribeResponse{}, fmt.Errorf(
			"elevenlabs transcribe: decode response: %w",
			err,
		)
	}
	return decoded, nil
}
