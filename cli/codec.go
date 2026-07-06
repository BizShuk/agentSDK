package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Codec translates between Envelopes and JSONL bytes on the wire.
type Codec interface {
	Write(env Envelope) error
	Read() (Envelope, error)
	Flush() error
}

// NewJSONLCodec returns a JSON Lines codec. Each Envelope is one line;
// Read returns one envelope per call (blocks until one is available).
func NewJSONLCodec(r io.Reader, w io.Writer) Codec {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	bw, ok := w.(*bufio.Writer)
	if !ok {
		bw = bufio.NewWriter(w)
	}
	return &jsonlCodec{br: br, bw: bw, enc: json.NewEncoder(bw)}
}

type jsonlCodec struct {
	br  *bufio.Reader
	bw  *bufio.Writer
	enc *json.Encoder
}

// Write encodes env as JSON and writes it plus a newline.
func (c *jsonlCodec) Write(env Envelope) error {
	if err := c.enc.Encode(env); err != nil {
		return fmt.Errorf("cli: encode: %w", err)
	}
	return nil
}

// Flush flushes the underlying writer.
func (c *jsonlCodec) Flush() error { return c.bw.Flush() }

// Read blocks on one JSONL line and returns the parsed envelope.
func (c *jsonlCodec) Read() (Envelope, error) {
	line, err := c.br.ReadBytes('\n')
	if err != nil {
		return Envelope{}, err
	}
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Envelope{}, fmt.Errorf("cli: decode: %w", err)
	}
	return env, nil
}

// WriteError is sugar — emit an error envelope and flush.
func WriteError(c Codec, runID, kind, msg string) error {
	return c.Write(Envelope{
		Type:  MSG_TYPE_ERROR,
		RunID: runID,
		Error: &ErrorPayload{Message: msg, Kind: kind},
	})
}

// WriteResult is sugar — emit a terminal result envelope.
func WriteResult(c Codec, runID, status string, turn int) error {
	return c.Write(Envelope{
		Type:   MSG_TYPE_RESULT,
		RunID:  runID,
		Turn:   turn,
		Result: &ResultPayload{Status: status, Turn: turn},
	})
}