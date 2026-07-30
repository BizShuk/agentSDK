package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

const (
	videoCreateTaskPath   = "/v1/video_generation"
	videoQueryTaskPath    = "/v1/query/video_generation"
	videoRetrieveFilePath = "/v1/files/retrieve"
	maxVideoResponseBytes = 1 << 20
	maxVideoErrorBytes    = 4 << 10
)

func (p *VideoProvider) createVideoTask(
	ctx context.Context,
	auth core.Auth,
	payload any,
) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("minimax video: marshal request: %w", err)
	}
	var response createVideoTaskResponse
	if err := p.doVideoJSON(
		ctx,
		auth,
		http.MethodPost,
		videoCreateTaskPath,
		body,
		&response,
	); err != nil {
		return "", err
	}
	if err := response.BaseResp.Err(); err != nil {
		return "", err
	}
	if response.TaskID == "" {
		return "", fmt.Errorf("minimax video: create returned an empty task id")
	}
	return response.TaskID, nil
}

func (p *VideoProvider) queryVideoTask(
	ctx context.Context,
	auth core.Auth,
	taskID string,
) (queryVideoTaskResponse, error) {
	query := url.Values{"task_id": {taskID}}
	path := videoQueryTaskPath + "?" + query.Encode()
	var response queryVideoTaskResponse
	if err := p.doVideoJSON(ctx, auth, http.MethodGet, path, nil, &response); err != nil {
		return queryVideoTaskResponse{}, err
	}
	if err := response.BaseResp.Err(); err != nil {
		return queryVideoTaskResponse{}, err
	}
	return response, nil
}

func (p *VideoProvider) pollVideoTask(
	ctx context.Context,
	auth core.Auth,
	taskID string,
	interval time.Duration,
) (string, error) {
	if interval <= 0 {
		interval = defaultVideoPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		response, err := p.queryVideoTask(ctx, auth, taskID)
		if err != nil {
			if ctx.Err() != nil {
				return "", videoPollingCancellation(ctx.Err())
			}
		} else {
			switch response.Status {
			case videoStatusSuccess:
				if response.FileID == "" {
					return "", fmt.Errorf("minimax video: successful task %s returned an empty file id", taskID)
				}
				return response.FileID, nil
			case videoStatusFail:
				return "", fmt.Errorf("%w: %s", ErrVideoTaskFailed, response.ErrorMessage)
			case videoStatusQueueing, videoStatusPreparing, videoStatusProcessing, "":
			default:
				return "", fmt.Errorf(
					"minimax video: task %s returned unknown status %q",
					taskID,
					response.Status,
				)
			}
		}

		select {
		case <-ctx.Done():
			return "", videoPollingCancellation(ctx.Err())
		case <-ticker.C:
		}
	}
}

func videoPollingCancellation(cause error) error {
	return fmt.Errorf("%w: %w", ErrVideoPollingCancelled, cause)
}

func (p *VideoProvider) retrieveVideoFile(
	ctx context.Context,
	auth core.Auth,
	fileID string,
) (string, error) {
	query := url.Values{"file_id": {fileID}}
	path := videoRetrieveFilePath + "?" + query.Encode()
	var response retrieveVideoFileResponse
	if err := p.doVideoJSON(ctx, auth, http.MethodGet, path, nil, &response); err != nil {
		return "", err
	}
	if err := response.BaseResp.Err(); err != nil {
		return "", err
	}
	if response.File.DownloadURL == "" {
		return "", fmt.Errorf("minimax video: file %s has no download URL", fileID)
	}
	return response.File.DownloadURL, nil
}

func (p *VideoProvider) doVideoJSON(
	ctx context.Context,
	auth core.Auth,
	method string,
	path string,
	body []byte,
	target any,
) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("minimax video: build %s %s: %w", method, path, err)
	}
	applyVideoAuthHeaders(request, auth)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("minimax video: %s %s: %w", method, path, err)
	}
	defer response.Body.Close()

	if response.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, maxVideoErrorBytes))
		return fmt.Errorf(
			"minimax video: %s %s: status=%d body=%q",
			method,
			path,
			response.StatusCode,
			string(snippet),
		)
	}
	if target == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxVideoResponseBytes))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxVideoResponseBytes)).Decode(target); err != nil {
		return fmt.Errorf("minimax video: decode %s %s: %w", method, path, err)
	}
	return nil
}

func applyVideoAuthHeaders(request *http.Request, auth core.Auth) {
	if token := auth.Token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range auth.Headers {
		if value != "" {
			request.Header.Set(key, value)
		}
	}
}
