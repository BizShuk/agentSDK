package minimax

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"regexp"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// Wire contracts:
// https://platform.minimax.io/docs/api-reference/voice-design-design
// https://platform.minimax.io/docs/api-reference/voice-cloning-uploadcloneaudio
// https://platform.minimax.io/docs/api-reference/voice-cloning-uploadprompt
// https://platform.minimax.io/docs/api-reference/voice-cloning-clone
// https://platform.minimax.io/docs/api-reference/voice-management-delete
const (
	voiceDesignPath = "/v1/voice_design"
	voiceClonePath  = "/v1/voice_clone"
	voiceDeletePath = "/v1/delete_voice"
	fileUploadPath  = "/v1/files/upload"

	filePurposeVoiceClone  = "voice_clone"
	filePurposePromptAudio = "prompt_audio"

	defaultVoiceRequestTimeout = 5 * time.Minute
	maxVoiceSampleBytes        = 20 << 20
	maxVoicePreviewRunes       = 500
	maxCloneTextRunes          = 1000
	maxVoiceManageResponse     = 64 << 20
)

// cloneVoiceIDPattern is the vendor rule: 8–256 chars, starts with a letter,
// letters / digits / '-' / '_', and does not end with '-' or '_'.
var cloneVoiceIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{6,254}[A-Za-z0-9]$`)

// VoiceProvider implements provider.VoiceManager.
type VoiceProvider struct {
	rest restClient
	auth core.Auth
}

// NewVoice returns a MiniMax voice manager from registry-resolved config.
func NewVoice(cfg provider.ResolvedConfig) (*VoiceProvider, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultVoiceRequestTimeout
	}
	return &VoiceProvider{
		rest: restClient{baseURL: accountBaseURL(cfg.BaseURL), client: &http.Client{Timeout: timeout}},
		auth: cfg.Auth,
	}, nil
}

func (p *VoiceProvider) credential(override core.Auth) (core.Auth, error) {
	auth := p.auth.Merge(override)
	if auth.Token() == "" {
		return core.Auth{}, fmt.Errorf("minimax voice credential is required")
	}
	return auth, nil
}

// DesignVoice creates a voice from a prose description; the preview audio
// arrives hex-encoded and is decoded here.
func (p *VoiceProvider) DesignVoice(ctx context.Context, request provider.VoiceDesignRequest) (provider.VoiceDesignResult, error) {
	if err := request.Validate(); err != nil {
		return provider.VoiceDesignResult{}, err
	}
	if n := len([]rune(request.PreviewText)); n > maxVoicePreviewRunes {
		return provider.VoiceDesignResult{}, fmt.Errorf("minimax voice design preview text has %d runes, exceeds %d", n, maxVoicePreviewRunes)
	}
	auth, err := p.credential(request.Auth)
	if err != nil {
		return provider.VoiceDesignResult{}, err
	}
	raw, err := p.rest.postJSON(ctx, auth, "design voice", voiceDesignPath, map[string]string{
		"prompt":       request.Prompt,
		"preview_text": request.PreviewText,
		"voice_id":     strings.TrimSpace(request.VoiceID),
	}, maxVoiceManageResponse)
	if err != nil {
		return provider.VoiceDesignResult{}, err
	}
	var response struct {
		VoiceID    string `json:"voice_id"`
		TrialAudio string `json:"trial_audio"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return provider.VoiceDesignResult{}, fmt.Errorf("minimax design voice: decode response: %w", err)
	}
	audio, err := hex.DecodeString(strings.TrimSpace(response.TrialAudio))
	if err != nil {
		return provider.VoiceDesignResult{}, fmt.Errorf("minimax design voice: decode preview: %w", err)
	}
	return provider.VoiceDesignResult{
		VoiceID: response.VoiceID,
		Preview: provider.SpeechAsset{Bytes: audio, Format: defaultSpeechFormat},
	}, nil
}

type voiceClonePrompt struct {
	PromptAudio int64  `json:"prompt_audio"`
	PromptText  string `json:"prompt_text"`
}

type voiceCloneRequest struct {
	FileID                  int64             `json:"file_id"`
	VoiceID                 string            `json:"voice_id"`
	ClonePrompt             *voiceClonePrompt `json:"clone_prompt,omitempty"`
	Text                    string            `json:"text,omitempty"`
	Model                   string            `json:"model,omitempty"`
	NeedNoiseReduction      bool              `json:"need_noise_reduction,omitempty"`
	NeedVolumeNormalization bool              `json:"need_volume_normalization,omitempty"`
}

// CloneVoice uploads the sample (and the optional prompt clip), then clones.
// A cloned voice the vendor sees unused for 7 days is deleted by MiniMax.
func (p *VoiceProvider) CloneVoice(ctx context.Context, request provider.VoiceCloneRequest) (provider.VoiceCloneResult, error) {
	if err := request.Validate(); err != nil {
		return provider.VoiceCloneResult{}, err
	}
	if !cloneVoiceIDPattern.MatchString(request.VoiceID) {
		return provider.VoiceCloneResult{}, fmt.Errorf("minimax voice id %q must be 8-256 chars, start with a letter, use letters/digits/-/_, and not end with - or _", request.VoiceID)
	}
	if n := len([]rune(request.PreviewText)); n > maxCloneTextRunes {
		return provider.VoiceCloneResult{}, fmt.Errorf("minimax voice clone preview text has %d runes, exceeds %d", n, maxCloneTextRunes)
	}
	auth, err := p.credential(request.Auth)
	if err != nil {
		return provider.VoiceCloneResult{}, err
	}

	wire := voiceCloneRequest{
		VoiceID:                 request.VoiceID,
		NeedNoiseReduction:      request.NoiseReduction,
		NeedVolumeNormalization: request.VolumeNormalization,
	}
	if wire.FileID, err = p.upload(ctx, auth, filePurposeVoiceClone, request.Sample); err != nil {
		return provider.VoiceCloneResult{}, err
	}
	if request.PromptAudio != nil {
		id, err := p.upload(ctx, auth, filePurposePromptAudio, *request.PromptAudio)
		if err != nil {
			return provider.VoiceCloneResult{}, err
		}
		wire.ClonePrompt = &voiceClonePrompt{PromptAudio: id, PromptText: request.PromptText}
	}
	if text := strings.TrimSpace(request.PreviewText); text != "" {
		wire.Text = text
		wire.Model = strings.TrimSpace(request.Model)
		if wire.Model == "" {
			wire.Model = defaultSpeechModel
		}
	}
	raw, err := p.rest.postJSON(ctx, auth, "clone voice", voiceClonePath, wire, maxVoiceManageResponse)
	if err != nil {
		return provider.VoiceCloneResult{}, err
	}
	var response struct {
		DemoAudio string `json:"demo_audio"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return provider.VoiceCloneResult{}, fmt.Errorf("minimax clone voice: decode response: %w", err)
	}
	return provider.VoiceCloneResult{VoiceID: request.VoiceID, PreviewURL: response.DemoAudio}, nil
}

// upload sends one audio file to /v1/files/upload and returns its file_id.
func (p *VoiceProvider) upload(ctx context.Context, auth core.Auth, purpose string, audio provider.AudioSource) (int64, error) {
	if len(audio.Bytes) == 0 {
		return 0, fmt.Errorf("minimax upload %s: audio bytes are required", purpose)
	}
	if len(audio.Bytes) > maxVoiceSampleBytes {
		return 0, fmt.Errorf("minimax upload %s: audio exceeds %d bytes", purpose, maxVoiceSampleBytes)
	}
	format := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(audio.Format)), ".")
	switch format {
	case "mp3", "m4a", "wav":
	case "":
		format = "mp3"
	default:
		return 0, fmt.Errorf("minimax upload %s: format %q is not supported; use mp3, m4a or wav", purpose, audio.Format)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("purpose", purpose); err != nil {
		return 0, fmt.Errorf("minimax upload %s: encode: %w", purpose, err)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s.%s"`, purpose, format))
	header.Set("Content-Type", "application/octet-stream")
	part, err := writer.CreatePart(header)
	if err == nil {
		_, err = part.Write(audio.Bytes)
	}
	if err == nil {
		err = writer.Close()
	}
	if err != nil {
		return 0, fmt.Errorf("minimax upload %s: encode: %w", purpose, err)
	}

	raw, err := p.rest.post(ctx, auth, "upload "+purpose, fileUploadPath, writer.FormDataContentType(), body.Bytes(), maxVoiceManageResponse)
	if err != nil {
		return 0, err
	}
	var response struct {
		File struct {
			FileID json.Number `json:"file_id"`
		} `json:"file"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return 0, fmt.Errorf("minimax upload %s: decode response: %w", purpose, err)
	}
	id, err := response.File.FileID.Int64()
	if err != nil || id == 0 {
		return 0, fmt.Errorf("minimax upload %s: response has no file_id", purpose)
	}
	return id, nil
}

// DeleteVoice deletes one cloned or designed voice. Deletion is permanent
// and the id cannot be reused.
func (p *VoiceProvider) DeleteVoice(ctx context.Context, request provider.VoiceDeleteRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	category := strings.TrimSpace(request.Category)
	switch category {
	case voiceCategoryCloning, voiceCategoryGeneration:
	default:
		return fmt.Errorf("minimax voice delete category must be %q or %q", voiceCategoryCloning, voiceCategoryGeneration)
	}
	auth, err := p.credential(request.Auth)
	if err != nil {
		return err
	}
	_, err = p.rest.postJSON(ctx, auth, "delete voice", voiceDeletePath, map[string]string{
		"voice_type": category,
		"voice_id":   request.VoiceID,
	}, maxVoiceManageResponse)
	return err
}
