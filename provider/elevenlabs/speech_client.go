package elevenlabs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func speechPath(voice string) string {
	return "/v1/text-to-speech/" + url.PathEscape(resolveVoiceID(voice))
}

func speechStreamPath(voice string) string {
	return speechPath(voice) + "/stream"
}

// postSpeech performs one blocking synthesis call. A 200 body is raw audio,
// not JSON, so the success path never decodes it.
func (p *SpeechProvider) postSpeech(
	ctx context.Context,
	call speechCall,
	path string,
) ([]byte, error) {
	response, err := p.send(ctx, call, path)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if err := p.checkStatus("generate speech", response); err != nil {
		return nil, err
	}
	return readBoundedResponse("generate speech", response.Body, maxAudioResponseBytes)
}

// postSpeechStream hands the live body to the caller. The status check runs
// first so a failed request is drained and closed here rather than surfacing
// as an error document the caller would try to play.
func (p *SpeechProvider) postSpeechStream(
	ctx context.Context,
	call speechCall,
	path string,
) (io.ReadCloser, error) {
	response, err := p.send(ctx, call, path)
	if err != nil {
		return nil, err
	}
	if err := p.checkStatus("stream speech", response); err != nil {
		response.Body.Close()
		return nil, err
	}
	return response.Body, nil
}

func (p *SpeechProvider) send(
	ctx context.Context,
	call speechCall,
	path string,
) (*http.Response, error) {
	endpoint := p.baseURL + path
	if call.outputQuery != "" {
		endpoint += "?" + url.Values{"output_format": {call.outputQuery}}.Encode()
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(call.payload),
	)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs speech: build request: %w", err)
	}
	applyAuthHeaders(request, call.auth)
	request.Header.Set("Accept", "audio/mpeg")
	request.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs speech: request: %w", err)
	}
	return response, nil
}

func (p *SpeechProvider) checkStatus(operation string, response *http.Response) error {
	if response.StatusCode/100 == 2 {
		return nil
	}
	id := requestID(response.Header)
	raw, err := readErrorResponse(operation, response.Body, maxErrorResponseBytes)
	if err != nil {
		return err
	}
	return decodeAPIError(operation, response.StatusCode, id, raw)
}
