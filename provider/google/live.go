package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// MAX_LIVE_FRAME_BYTES bounds one websocket message. Audio-modality turns
// carry inline PCM chunks well past coder/websocket's 32 KiB default read
// limit, mirroring the oversized-frame lesson from the antigravity SSE path.
const MAX_LIVE_FRAME_BYTES = 16 << 20

// LiveProvider opens Gemini Live API sessions over the BidiGenerateContent
// websocket. The same surface serves dialogue (DefaultLiveModel) and realtime
// translation (DefaultTranslateModel with a translationConfig).
type LiveProvider struct {
	baseURL string
	auth    core.Auth
}

// NewLive builds the Live API connector from registry-resolved options.
func NewLive(cfg provider.ResolvedConfig) (*LiveProvider, error) {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultLiveBaseURL
	}
	return &LiveProvider{baseURL: base, auth: cfg.Auth}, nil
}

// ConnectLive implements provider.LiveConnector: dial, send setup, wait for
// setupComplete, hand the session to the caller.
func (p *LiveProvider) ConnectLive(
	ctx context.Context,
	req provider.LiveRequest,
) (provider.LiveSession, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	auth := p.auth.Merge(req.Auth)
	if auth.Token() == "" {
		return nil, fmt.Errorf("google live: credential is required")
	}
	endpoint, header, err := liveDialTarget(p.baseURL, auth)
	if err != nil {
		return nil, err
	}
	conn, _, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		return nil, fmt.Errorf("google live: dial: %w", err)
	}
	conn.SetReadLimit(MAX_LIVE_FRAME_BYTES)

	session := &liveSession{conn: conn}
	if err := session.write(ctx, liveClientMessage{Setup: newLiveSetup(req)}); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("google live: send setup: %w", err)
	}
	if err := session.awaitSetup(ctx); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("google live: setup: %w", err)
	}
	return session, nil
}

// liveDialTarget resolves the websocket URL and handshake headers. An API key
// authenticates via the ?key= query parameter, an OAuth token via the
// Authorization header — matching the two schemes the gateway accepts.
func liveDialTarget(base string, auth core.Auth) (string, http.Header, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", nil, fmt.Errorf("google live: base URL: %w", err)
	}
	header := http.Header{}
	if auth.Bearer != "" {
		header.Set("Authorization", "Bearer "+auth.Bearer)
	} else {
		q := u.Query()
		q.Set("key", auth.APIKey)
		u.RawQuery = q.Encode()
	}
	return u.String(), header, nil
}

// newLiveSetup projects the provider-neutral request into the wire setup
// payload. Text modality is the default; the model id is normalized to the
// "models/<id>" resource name the endpoint expects.
func newLiveSetup(req provider.LiveRequest) *liveSetup {
	model := req.Model
	if model == "" {
		model = DefaultLiveModel
	}
	if !strings.Contains(model, "/") {
		model = "models/" + model
	}

	generation := &liveGenerationConfig{}
	switch req.ResponseModality {
	case provider.LIVE_MODALITY_AUDIO:
		generation.ResponseModalities = []string{"AUDIO"}
	default:
		generation.ResponseModalities = []string{"TEXT"}
	}
	if req.ThinkingLevel != "" {
		generation.ThinkingConfig = &liveThinkingConfig{ThinkingLevel: req.ThinkingLevel}
	}
	if req.Voice != "" {
		generation.SpeechConfig = &liveSpeechConfig{
			VoiceConfig: &liveVoiceConfig{
				PrebuiltVoiceConfig: &livePrebuiltVoice{VoiceName: req.Voice},
			},
		}
	}
	if req.Translation != nil {
		generation.TranslationConfig = &liveTranslationConfig{
			TargetLanguageCode: req.Translation.TargetLanguage,
			EchoTargetLanguage: req.Translation.EchoTarget,
		}
	}

	setup := &liveSetup{Model: model, GenerationConfig: generation}
	if req.System != "" {
		setup.SystemInstruction = &liveContent{Parts: []livePart{{Text: req.System}}}
	}
	if req.TranscribeInput {
		setup.InputAudioTranscription = &struct{}{}
	}
	if req.TranscribeOutput {
		setup.OutputAudioTranscription = &struct{}{}
	}
	return setup
}

// liveSession is one open BidiGenerateContent connection. Writes are
// serialized by mu so SendText and SendAudio may race from different
// goroutines; Receive follows the interface contract of a single reader.
type liveSession struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (s *liveSession) write(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.Write(ctx, websocket.MessageText, data)
}

// awaitSetup consumes frames until the server acknowledges the setup.
func (s *liveSession) awaitSetup(ctx context.Context) error {
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			return err
		}
		var msg liveServerMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("decode server frame: %w", err)
		}
		if msg.SetupComplete != nil {
			return nil
		}
	}
}

// SendText submits one complete user text turn via clientContent.
func (s *liveSession) SendText(ctx context.Context, text string) error {
	return s.write(ctx, liveClientMessage{
		ClientContent: &liveClientContent{
			Turns: []liveContent{
				{Role: "user", Parts: []livePart{{Text: text}}},
			},
			TurnComplete: true,
		},
	})
}

// SendAudio streams one chunk of 16 kHz 16-bit mono little-endian PCM via
// realtimeInput.
func (s *liveSession) SendAudio(ctx context.Context, pcm []byte) error {
	return s.write(ctx, liveClientMessage{
		RealtimeInput: &liveRealtimeInput{
			Audio: &liveBlob{
				MIMEType: "audio/pcm;rate=16000",
				Data:     base64.StdEncoding.EncodeToString(pcm),
			},
		},
	})
}

// Receive blocks for the next serverContent frame and folds it into one
// provider.LiveEvent. Frames without content (goAway, usage metadata) are
// skipped; a normal server close surfaces as io.EOF.
func (s *liveSession) Receive(ctx context.Context) (provider.LiveEvent, error) {
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return provider.LiveEvent{}, io.EOF
			}
			return provider.LiveEvent{}, err
		}
		var msg liveServerMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return provider.LiveEvent{}, fmt.Errorf("decode server frame: %w", err)
		}
		if msg.ServerContent == nil {
			continue
		}
		event, err := foldServerContent(msg.ServerContent)
		if err != nil {
			return provider.LiveEvent{}, err
		}
		return event, nil
	}
}

// Close performs the closing handshake. A session the server already closed
// reports its close status through err; that is a completed shutdown, not a
// failure, so it folds to nil.
func (s *liveSession) Close() error {
	err := s.conn.Close(websocket.StatusNormalClosure, "")
	if err != nil && websocket.CloseStatus(err) != -1 {
		return nil
	}
	return err
}

// foldServerContent maps one serverContent frame onto the neutral event. A
// single frame can carry text, audio, and transcripts at once, so every part
// is folded rather than the first match winning.
func foldServerContent(sc *liveServerContent) (provider.LiveEvent, error) {
	event := provider.LiveEvent{
		Interrupted:  sc.Interrupted,
		TurnComplete: sc.TurnComplete,
	}
	if event.TurnComplete {
		event.Cost = core.UnpricedCost()
	}
	if sc.InputTranscription != nil {
		event.InputTranscript = sc.InputTranscription.Text
	}
	if sc.OutputTranscription != nil {
		event.OutputTranscript = sc.OutputTranscription.Text
	}
	if sc.ModelTurn == nil {
		return event, nil
	}
	for _, part := range sc.ModelTurn.Parts {
		event.Text += part.Text
		if part.InlineData == nil {
			continue
		}
		audio, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
		if err != nil {
			return provider.LiveEvent{}, fmt.Errorf("decode audio part: %w", err)
		}
		event.Audio = append(event.Audio, audio...)
		event.AudioMIME = part.InlineData.MIMEType
	}
	return event, nil
}

// ----------------------------------------------------------------------------
// BidiGenerateContent wire DTOs. Local to this adapter on purpose: no other
// registered provider speaks this protocol, so there is nothing to share.
// ----------------------------------------------------------------------------

type liveClientMessage struct {
	Setup         *liveSetup         `json:"setup,omitempty"`
	ClientContent *liveClientContent `json:"clientContent,omitempty"`
	RealtimeInput *liveRealtimeInput `json:"realtimeInput,omitempty"`
}

type liveSetup struct {
	Model                    string                `json:"model"`
	GenerationConfig         *liveGenerationConfig `json:"generationConfig,omitempty"`
	SystemInstruction        *liveContent          `json:"systemInstruction,omitempty"`
	InputAudioTranscription  *struct{}             `json:"inputAudioTranscription,omitempty"`
	OutputAudioTranscription *struct{}             `json:"outputAudioTranscription,omitempty"`
}

type liveGenerationConfig struct {
	ResponseModalities []string               `json:"responseModalities,omitempty"`
	ThinkingConfig     *liveThinkingConfig    `json:"thinkingConfig,omitempty"`
	SpeechConfig       *liveSpeechConfig      `json:"speechConfig,omitempty"`
	TranslationConfig  *liveTranslationConfig `json:"translationConfig,omitempty"`
}

type liveThinkingConfig struct {
	ThinkingLevel string `json:"thinkingLevel"`
}

type liveSpeechConfig struct {
	VoiceConfig *liveVoiceConfig `json:"voiceConfig,omitempty"`
}

type liveVoiceConfig struct {
	PrebuiltVoiceConfig *livePrebuiltVoice `json:"prebuiltVoiceConfig,omitempty"`
}

type livePrebuiltVoice struct {
	VoiceName string `json:"voiceName"`
}

type liveTranslationConfig struct {
	TargetLanguageCode string `json:"targetLanguageCode"`
	EchoTargetLanguage bool   `json:"echoTargetLanguage,omitempty"`
}

type liveContent struct {
	Role  string     `json:"role,omitempty"`
	Parts []livePart `json:"parts"`
}

type livePart struct {
	Text       string    `json:"text,omitempty"`
	InlineData *liveBlob `json:"inlineData,omitempty"`
}

type liveBlob struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type liveClientContent struct {
	Turns        []liveContent `json:"turns"`
	TurnComplete bool          `json:"turnComplete"`
}

type liveRealtimeInput struct {
	Text  string    `json:"text,omitempty"`
	Audio *liveBlob `json:"audio,omitempty"`
}

type liveServerMessage struct {
	SetupComplete *struct{}          `json:"setupComplete,omitempty"`
	ServerContent *liveServerContent `json:"serverContent,omitempty"`
}

type liveServerContent struct {
	ModelTurn           *liveContent       `json:"modelTurn,omitempty"`
	TurnComplete        bool               `json:"turnComplete,omitempty"`
	GenerationComplete  bool               `json:"generationComplete,omitempty"`
	Interrupted         bool               `json:"interrupted,omitempty"`
	InputTranscription  *liveTranscription `json:"inputTranscription,omitempty"`
	OutputTranscription *liveTranscription `json:"outputTranscription,omitempty"`
}

type liveTranscription struct {
	Text string `json:"text,omitempty"`
}
