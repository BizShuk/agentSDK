package sse

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecoderMultilineData(t *testing.T) {
	raw := "event: response.output_text.delta\r\nid: event_1\r\nretry: 1500\r\n: keepalive\r\ndata: {\"type\":\r\ndata: \"delta\"}\r\n\r\n"
	frame, err := NewDecoder(strings.NewReader(raw)).Next()
	require.NoError(t, err)
	assert.Equal(t, "response.output_text.delta", frame.Event)
	assert.Equal(t, "event_1", frame.ID)
	require.NotNil(t, frame.RetryMillis)
	assert.Equal(t, 1500, *frame.RetryMillis)
	assert.Equal(t, []string{"keepalive"}, frame.Comments)
	assert.Equal(t, "{\"type\":\n\"delta\"}", string(frame.Data))
}

func TestDecoderRejectsPartialFrameAtEOF(t *testing.T) {
	_, err := NewDecoder(strings.NewReader("event: message_start\ndata: {}\n")).Next()
	require.ErrorIs(t, err, ErrUnexpectedEOF)
}

func TestDecoderReturnsEOFWhenEmpty(t *testing.T) {
	_, err := NewDecoder(strings.NewReader("")).Next()
	require.ErrorIs(t, err, io.EOF)
}

func TestDecoderRejectsNilReader(t *testing.T) {
	_, err := NewDecoder(nil).Next()
	require.EqualError(t, err, "decode SSE: nil reader")
}

func TestDecoderStripsUTF8BOM(t *testing.T) {
	frame, err := NewDecoder(strings.NewReader("\xef\xbb\xbfevent: message_start\ndata: {}\n\n")).Next()
	require.NoError(t, err)
	assert.Equal(t, "message_start", frame.Event)
	assert.JSONEq(t, `{}`, string(frame.Data))
}

func TestDecoderRejectsLineOverLimit(t *testing.T) {
	raw := "data: " + strings.Repeat("x", 32) + "\n\n"
	_, err := NewBoundedDecoder(strings.NewReader(raw), 20).Next()
	require.ErrorIs(t, err, ErrLineTooLarge)
}

func TestDecoderRejectsFrameOverLimit(t *testing.T) {
	raw := "data: 12345\ndata: 67890\n\n"
	_, err := NewBoundedDecoder(strings.NewReader(raw), 20).Next()
	require.ErrorIs(t, err, ErrFrameTooLarge)
}

func TestDecoderRejectsInvalidRetry(t *testing.T) {
	_, err := NewDecoder(strings.NewReader("retry: later\n\n")).Next()
	require.ErrorContains(t, err, `decode SSE retry "later"`)
}

func TestWriteRoundTrip(t *testing.T) {
	retry := 250
	want := Frame{
		Event: "message_stop", ID: "event_2", RetryMillis: &retry,
		Comments: []string{"one", "two"}, Data: []byte("first\nsecond"),
	}
	var out bytes.Buffer
	require.NoError(t, Write(&out, want))

	got, err := NewDecoder(&out).Next()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestWritePropagatesWriterError(t *testing.T) {
	errBoom := errors.New("boom")
	err := Write(errorWriter{err: errBoom}, Frame{Data: []byte("data")})
	require.ErrorIs(t, err, errBoom)
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }
