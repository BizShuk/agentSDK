// Package wire implements the headless presentation surfaces shared by
// claude-code (-p / stream-json), codex (exec --json), and pi (--mode
// json / rpc): a stable JSONL envelope over core.StreamEvent, an RPC
// framing for process embedding, and a plain-text formatter for print
// mode.
//
// This package revives the removed cli/ envelope idea with an actual
// caller path: runtime → core.EventSink → wire.Sink → JSONL out.
// Envelope fields must stay JSON round-trip stable — they are external API.
package wire

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// Type discriminates Envelope payloads.
type Type string

const (
	TYPE_EVENT            Type = "event"
	TYPE_RESULT           Type = "result"
	TYPE_ERROR            Type = "error"
	TYPE_APPROVAL_REQUEST Type = "approval_request"
	TYPE_HUMAN_DECISION   Type = "human_decision"
)

// MAX_LINE_BYTES bounds one JSONL line on decode (1 MiB).
const MAX_LINE_BYTES = 1 << 20

// Result is the terminal summary of a run.
type Result struct {
	RunID  string          `json:"run_id"`
	Status core.RunStatus  `json:"status"`
	Text   string          `json:"text,omitempty"` // final assistant text
	Usage  core.TokenUsage `json:"usage"`
	Cost   core.Cost       `json:"cost"`
}

// ErrorPayload carries a terminal error. Messages must already be
// credential/prompt-free — wire does not redact.
type ErrorPayload struct {
	Message string `json:"message"`
}

// Envelope is one JSONL line. Exactly one payload field matches Type.
type Envelope struct {
	Type     Type                   `json:"type"`
	Stream   *core.StreamEvent      `json:"stream,omitempty"`
	Result   *Result                `json:"result,omitempty"`
	Error    *ErrorPayload          `json:"error,omitempty"`
	Approval *core.PendingApproval  `json:"approval,omitempty"`
	Decision *core.ApprovalDecision `json:"decision,omitempty"`
	Ts       time.Time              `json:"ts,omitzero"`
}

// Encoder writes one Envelope per line; safe for concurrent use.
type Encoder struct {
	w  io.Writer
	mu sync.Mutex
}

// NewEncoder wraps a writer.
func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

// Encode writes env as a single JSON line.
func (e *Encoder) Encode(env Envelope) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("wire encode: %w", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.w.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("wire write: %w", err)
	}
	return nil
}

// Decoder reads one Envelope per line.
type Decoder struct {
	sc *bufio.Scanner
}

// NewDecoder wraps a reader; lines up to MAX_LINE_BYTES are accepted.
func NewDecoder(r io.Reader) *Decoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MAX_LINE_BYTES)
	return &Decoder{sc: sc}
}

// Next returns the next Envelope, io.EOF at end of stream.
func (d *Decoder) Next() (Envelope, error) {
	for d.sc.Scan() {
		line := strings.TrimSpace(d.sc.Text())
		if line == "" {
			continue
		}
		var env Envelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			return Envelope{}, fmt.Errorf("wire decode: %w", err)
		}
		return env, nil
	}
	if err := d.sc.Err(); err != nil {
		return Envelope{}, err
	}
	return Envelope{}, io.EOF
}

// Sink adapts an Encoder to core.EventSink — the stream-json mode wiring.
type Sink struct {
	Enc *Encoder
	Now func() time.Time // nil → time.Now
}

// NewSink builds a Sink writing to w.
func NewSink(w io.Writer) *Sink { return &Sink{Enc: NewEncoder(w)} }

// OnStreamEvent implements core.EventSink. Encode errors are dropped —
// a broken pipe must not fail the run.
func (s *Sink) OnStreamEvent(ev core.StreamEvent) {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	_ = s.Enc.Encode(Envelope{Type: TYPE_EVENT, Stream: &ev, Ts: now().UTC()})
}

// Request is one RPC call (LF-delimited JSONL, pi's --mode rpc shape).
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response answers one Request by ID.
type Response struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// ReadRequest reads the next request line; io.EOF at end of stream.
func ReadRequest(sc *bufio.Scanner) (Request, error) {
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			return Request{}, fmt.Errorf("wire rpc decode: %w", err)
		}
		return req, nil
	}
	if err := sc.Err(); err != nil {
		return Request{}, err
	}
	return Request{}, io.EOF
}

// WriteResponse writes one response line.
func WriteResponse(w io.Writer, resp Response) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("wire rpc encode: %w", err)
	}
	if _, err := w.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("wire rpc write: %w", err)
	}
	return nil
}

// FormatStream renders one StreamEvent for print mode; "" = not printed.
func FormatStream(ev core.StreamEvent) string {
	switch ev.Kind {
	case core.STREAM_MESSAGE:
		return ev.Text
	case core.STREAM_TOOL_START:
		if ev.ToolCall != nil {
			return fmt.Sprintf("→ %s", ev.ToolCall.Name)
		}
	case core.STREAM_TOOL_RESULT:
		if ev.ToolResult != nil {
			if ev.ToolResult.OK {
				return fmt.Sprintf("← %s ok", ev.ToolResult.Name)
			}
			return fmt.Sprintf("← %s error: %s", ev.ToolResult.Name, ev.ToolResult.Error)
		}
	}
	return ""
}
