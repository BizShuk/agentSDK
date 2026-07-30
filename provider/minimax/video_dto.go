package minimax

import (
	"fmt"

	"github.com/bizshuk/agentsdk/provider"
)

type textVideoRequest struct {
	Prompt     string `json:"prompt"`
	Model      string `json:"model,omitempty"`
	Duration   int    `json:"duration,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

type imageVideoRequest struct {
	Prompt          string `json:"prompt"`
	FirstFrameImage string `json:"first_frame_image"`
	Model           string `json:"model,omitempty"`
	Duration        int    `json:"duration,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
}

type startEndVideoRequest struct {
	Prompt          string `json:"prompt"`
	FirstFrameImage string `json:"first_frame_image"`
	LastFrameImage  string `json:"last_frame_image"`
	Model           string `json:"model,omitempty"`
	Duration        int    `json:"duration,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
}

type videoSubjectReference struct {
	Type  string   `json:"type"`
	Image []string `json:"image"`
}

type subjectVideoRequest struct {
	Prompt           string                  `json:"prompt"`
	SubjectReference []videoSubjectReference `json:"subject_reference"`
	Model            string                  `json:"model,omitempty"`
	Duration         int                     `json:"duration,omitempty"`
	Resolution       string                  `json:"resolution,omitempty"`
}

type videoBaseResponse struct {
	StatusCode int    `json:"status_code,omitempty"`
	StatusMsg  string `json:"status_msg,omitempty"`
}

func (r videoBaseResponse) Err() error {
	if r.StatusCode == 0 {
		return nil
	}
	return fmt.Errorf("minimax video API error (code %d): %s", r.StatusCode, r.StatusMsg)
}

type createVideoTaskResponse struct {
	TaskID   string            `json:"task_id"`
	BaseResp videoBaseResponse `json:"base_resp"`
}

type queryVideoTaskResponse struct {
	BaseResp     videoBaseResponse `json:"base_resp"`
	Status       string            `json:"status"`
	FileID       string            `json:"file_id,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
}

type retrieveVideoFileResponse struct {
	BaseResp videoBaseResponse `json:"base_resp"`
	File     struct {
		DownloadURL string `json:"download_url"`
	} `json:"file"`
}

func buildVideoPayload(request provider.VideoRequest, model string) (any, error) {
	switch request.Mode {
	case provider.VIDEO_MODE_TEXT:
		return &textVideoRequest{
			Prompt:     request.Prompt,
			Model:      model,
			Duration:   request.Duration,
			Resolution: request.Resolution,
		}, nil
	case provider.VIDEO_MODE_IMAGE:
		return &imageVideoRequest{
			Prompt:          request.Prompt,
			FirstFrameImage: request.FirstFrameURL,
			Model:           model,
			Duration:        request.Duration,
			Resolution:      request.Resolution,
		}, nil
	case provider.VIDEO_MODE_START_END:
		return &startEndVideoRequest{
			Prompt:          request.Prompt,
			FirstFrameImage: request.FirstFrameURL,
			LastFrameImage:  request.LastFrameURL,
			Model:           model,
			Duration:        request.Duration,
			Resolution:      request.Resolution,
		}, nil
	case provider.VIDEO_MODE_SUBJECT:
		return &subjectVideoRequest{
			Prompt: request.Prompt,
			SubjectReference: []videoSubjectReference{{
				Type:  "character",
				Image: request.SubjectImageURLs,
			}},
			Model:      model,
			Duration:   request.Duration,
			Resolution: request.Resolution,
		}, nil
	default:
		return nil, fmt.Errorf("minimax video: unsupported mode %q", request.Mode)
	}
}
