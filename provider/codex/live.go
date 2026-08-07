package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/coder/websocket"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// MAX_LIVE_FRAME_BYTES bounds one websocket message, matching the google
// live transport: audio deltas overflow coder/websocket's 32 KiB default.
const MAX_LIVE_FRAME_BYTES = 16 << 20

// LiveProvider opens OpenAI Realtime API sessions over WebSocket. This
// surface takes standard API keys at api.openai.com — separate from the
// chat adapter's OAuth-only chatgpt.com backend.
type LiveProvider struct {
	baseURL string
	auth    core.Auth
}

// NewLive builds the Realtime API connector from registry-resolved options.
func NewLive(cfg provider.ResolvedConfig) (*LiveProvider, error) {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultLiveBaseURL
	}
	return &LiveProvider{baseURL: base, auth: cfg.Auth}, nil
}

// ConnectLive implements provider.LiveConnector: dial with the model in the
// query string, apply the session config, wait for session.updated.
//
// ThinkingLevel and Translation are rejected explicitly: gpt-realtime has no
// reasoning knob and no translation config, and silently ignoring a request
// field is worse than failing it.
func (p *LiveProvider) ConnectLive(
	ctx context.Context,
	req provider.LiveRequest,
) (provider.LiveSession, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.ThinkingLevel != "" {
		return nil, fmt.Errorf("codex live: thinking level is not supported by the realtime surface")
	}
	if req.Translation != nil {
		return nil, fmt.Errorf("codex live: translation is not supported by the realtime surface")
	}
	auth := p.auth.Merge(req.Auth)
	if auth.APIKey == "" {
		if auth.Bearer != "" {
			return nil, fmt.Errorf("codex live: requires api_key credential (%s); the realtime surface does not accept ChatGPT OAuth tokens", APIKeyEnvVar)
		}
		return nil, fmt.Errorf("codex live: credential is required")
	}

	model := req.Model
	if model == "" {
		model = DefaultLiveModel
	}
	endpoint, err := liveDialURL(p.baseURL, model)
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+auth.APIKey)

	conn, _, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		return nil, fmt.Errorf("codex live: dial: %w", err)
	}
	conn.SetReadLimit(MAX_LIVE_FRAME_BYTES)

	session := &realtimeSession{conn: conn}
	update := realtimeClientEvent{
		Type:    "session.update",
		Session: newRealtimeSessionConfig(req),
	}
	if err := session.write(ctx, update); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("codex live: send session.update: %w", err)
	}
	if err := session.awaitUpdated(ctx); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("codex live: setup: %w", err)
	}
	return session, nil
}

// liveDialURL appends the model query parameter, preserving any query an
// override base already carries.
func liveDialURL(base, model string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("codex live: base URL: %w", err)
	}
	q := u.Query()
	q.Set("model", model)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// newRealtimeSessionConfig projects the provider-neutral request onto the GA
// session shape. Only requested knobs are set; the endpoint's own defaults
// (24 kHz PCM16 both directions, server VAD) stay in place.
func newRealtimeSessionConfig(req provider.LiveRequest) *realtimeSessionConfig {
	session := &realtimeSessionConfig{
		Type:         "realtime",
		Instructions: req.System,
	}
	switch req.ResponseModality {
	case provider.LIVE_MODALITY_AUDIO:
		session.OutputModalities = []string{"audio"}
	default:
		session.OutputModalities = []string{"text"}
	}
	if req.TranscribeInput {
		session.audio().Input = &realtimeAudioInput{
			Transcription: &realtimeTranscriptionConfig{
				Model: DefaultInputTranscriptionModel,
			},
		}
	}
	if req.Voice != "" {
		session.audio().Output = &realtimeAudioOutput{Voice: req.Voice}
	}
	return session
}

// realtimeSession is one open Realtime API connection. Writes are serialized
// by mu; Receive follows the interface contract of a single reader.
type realtimeSession struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (s *realtimeSession) write(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.Write(ctx, websocket.MessageText, data)
}

// awaitUpdated consumes events until the server acknowledges the session
// config. session.created arrives first and is skipped; an error event fails
// the handshake with the server's own message.
func (s *realtimeSession) awaitUpdated(ctx context.Context) error {
	for {
		event, err := s.read(ctx)
		if err != nil {
			return err
		}
		switch event.Type {
		case "session.updated":
			return nil
		case "error":
			return event.Error.err()
		}
	}
}

func (s *realtimeSession) read(ctx context.Context) (realtimeServerEvent, error) {
	_, data, err := s.conn.Read(ctx)
	if err != nil {
		return realtimeServerEvent{}, err
	}
	var event realtimeServerEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return realtimeServerEvent{}, fmt.Errorf("decode server event: %w", err)
	}
	return event, nil
}

// SendText submits one complete user text turn: the conversation item plus
// the explicit response.create the text path requires.
func (s *realtimeSession) SendText(ctx context.Context, text string) error {
	item := realtimeClientEvent{
		Type: "conversation.item.create",
		Item: &realtimeItem{
			Type: "message",
			Role: "user",
			Content: []realtimeContent{
				{Type: "input_text", Text: text},
			},
		},
	}
	if err := s.write(ctx, item); err != nil {
		return err
	}
	return s.write(ctx, realtimeClientEvent{Type: "response.create"})
}

// SendAudio streams one chunk of 24 kHz 16-bit mono little-endian PCM into
// the input audio buffer; server VAD commits turns.
func (s *realtimeSession) SendAudio(ctx context.Context, pcm []byte) error {
	return s.write(ctx, realtimeClientEvent{
		Type:  "input_audio_buffer.append",
		Audio: base64.StdEncoding.EncodeToString(pcm),
	})
}

// Receive blocks for the next mapped server event. Unmapped lifecycle events
// are skipped; a normal server close surfaces as io.EOF; a server error
// event fails the read with the server's message.
func (s *realtimeSession) Receive(ctx context.Context) (provider.LiveEvent, error) {
	for {
		event, err := s.read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return provider.LiveEvent{}, io.EOF
			}
			return provider.LiveEvent{}, err
		}
		switch event.Type {
		case "response.output_text.delta":
			return provider.LiveEvent{Text: event.Delta}, nil
		case "response.output_audio.delta":
			audio, err := base64.StdEncoding.DecodeString(event.Delta)
			if err != nil {
				return provider.LiveEvent{}, fmt.Errorf("decode audio delta: %w", err)
			}
			return provider.LiveEvent{
				Audio:     audio,
				AudioMIME: "audio/pcm;rate=24000",
			}, nil
		case "response.output_audio_transcript.delta":
			return provider.LiveEvent{OutputTranscript: event.Delta}, nil
		case "conversation.item.input_audio_transcription.completed":
			// The completed transcript, not the deltas: mapping both would
			// hand callers every utterance twice.
			return provider.LiveEvent{InputTranscript: event.Transcript}, nil
		case "input_audio_buffer.speech_started":
			return provider.LiveEvent{Interrupted: true}, nil
		case "response.done":
			return provider.LiveEvent{TurnComplete: true, Cost: core.UnpricedCost()}, nil
		case "error":
			return provider.LiveEvent{}, event.Error.err()
		}
	}
}

// Close performs the closing handshake; a session the server already closed
// is a completed shutdown, not a failure.
func (s *realtimeSession) Close() error {
	err := s.conn.Close(websocket.StatusNormalClosure, "")
	if err != nil && websocket.CloseStatus(err) != -1 {
		return nil
	}
	return err
}

// ----------------------------------------------------------------------------
// Realtime API wire DTOs (GA session shape). Local to this adapter: no other
// registered provider speaks this protocol.
// ----------------------------------------------------------------------------

type realtimeClientEvent struct {
	Type    string                 `json:"type"`
	Session *realtimeSessionConfig `json:"session,omitempty"`
	Item    *realtimeItem          `json:"item,omitempty"`
	Audio   string                 `json:"audio,omitempty"`
}

type realtimeSessionConfig struct {
	Type             string               `json:"type"`
	OutputModalities []string             `json:"output_modalities,omitempty"`
	Instructions     string               `json:"instructions,omitempty"`
	Audio            *realtimeAudioConfig `json:"audio,omitempty"`
}

// audio returns the audio config block, allocating it on first use so an
// all-default session omits the field entirely.
func (c *realtimeSessionConfig) audio() *realtimeAudioConfig {
	if c.Audio == nil {
		c.Audio = &realtimeAudioConfig{}
	}
	return c.Audio
}

type realtimeAudioConfig struct {
	Input  *realtimeAudioInput  `json:"input,omitempty"`
	Output *realtimeAudioOutput `json:"output,omitempty"`
}

type realtimeAudioInput struct {
	Transcription *realtimeTranscriptionConfig `json:"transcription,omitempty"`
}

type realtimeAudioOutput struct {
	Voice string `json:"voice,omitempty"`
}

type realtimeTranscriptionConfig struct {
	Model string `json:"model"`
}

type realtimeItem struct {
	Type    string            `json:"type"`
	Role    string            `json:"role"`
	Content []realtimeContent `json:"content"`
}

type realtimeContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type realtimeServerEvent struct {
	Type       string                 `json:"type"`
	Delta      string                 `json:"delta,omitempty"`
	Transcript string                 `json:"transcript,omitempty"`
	Error      *realtimeAPIError      `json:"error,omitempty"`
	Session    *realtimeSessionConfig `json:"session,omitempty"`
}

type realtimeAPIError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func (e *realtimeAPIError) err() error {
	if e == nil {
		return fmt.Errorf("realtime error event without payload")
	}
	return fmt.Errorf("realtime error: %s", e.Message)
}
