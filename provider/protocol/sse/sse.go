// Package sse provides bounded Server-Sent Events frame decoding and encoding.
//
// It owns transport framing only. Provider-specific terminal events and JSON
// payload semantics belong to the protocol package that consumes Frame values.
package sse

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ErrUnexpectedEOF means an SSE frame ended without its blank-line boundary.
var ErrUnexpectedEOF = errors.New("unexpected EOF inside SSE frame")

// ErrLineTooLarge means one SSE line exceeded the configured byte limit.
var ErrLineTooLarge = errors.New("SSE line exceeds limit")

// ErrFrameTooLarge means one SSE frame exceeded the configured byte limit.
var ErrFrameTooLarge = errors.New("SSE frame exceeds limit")

// MAX_FRAME_BYTES is the default maximum size of one SSE line or frame.
const MAX_FRAME_BYTES int64 = 1 << 20

// Frame is one complete Server-Sent Events frame.
type Frame struct {
	Event       string
	ID          string
	RetryMillis *int
	Comments    []string
	Data        []byte
}

// Decoder reads complete SSE frames from an input stream.
type Decoder struct {
	reader        *bufio.Reader
	maxLineBytes  int64
	maxFrameBytes int64
	atStreamStart bool
}

// NewDecoder returns a full-frame decoder with the default byte limit.
func NewDecoder(reader io.Reader) *Decoder {
	return NewBoundedDecoder(reader, MAX_FRAME_BYTES)
}

// NewBoundedDecoder returns a decoder with per-line and per-frame limits.
func NewBoundedDecoder(reader io.Reader, maxBytes int64) *Decoder {
	var buffered *bufio.Reader
	if reader != nil {
		buffered = bufio.NewReader(reader)
	}
	return &Decoder{
		reader:        buffered,
		maxLineBytes:  maxBytes,
		maxFrameBytes: maxBytes,
		atStreamStart: true,
	}
}

// Next returns the next frame after its blank-line terminator.
func (d *Decoder) Next() (Frame, error) {
	if d == nil || d.reader == nil {
		return Frame{}, fmt.Errorf("decode SSE: nil reader")
	}
	if d.maxLineBytes <= 0 || d.maxFrameBytes <= 0 {
		return Frame{}, fmt.Errorf("decode SSE: byte limit must be positive")
	}

	var frame Frame
	var dataLines []string
	sawRecognizedLine := false
	var frameBytes int64

	for {
		lineBytes, err := d.readLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return Frame{}, fmt.Errorf("read SSE line: %w", err)
		}
		frameBytes += int64(len(lineBytes))
		if frameBytes > d.maxFrameBytes {
			return Frame{}, fmt.Errorf("%w: limit %d bytes", ErrFrameTooLarge, d.maxFrameBytes)
		}
		if d.atStreamStart {
			d.atStreamStart = false
			lineBytes = bytes.TrimPrefix(lineBytes, []byte{0xEF, 0xBB, 0xBF})
		}
		if len(lineBytes) > 0 {
			line := string(lineBytes)
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			if line == "" {
				if !sawRecognizedLine {
					if errors.Is(err, io.EOF) {
						return Frame{}, io.EOF
					}
					continue
				}
				frame.Data = []byte(strings.Join(dataLines, "\n"))
				return frame, nil
			}
			recognized, parseErr := parseLine(&frame, &dataLines, line)
			if parseErr != nil {
				return Frame{}, parseErr
			}
			sawRecognizedLine = sawRecognizedLine || recognized
		}
		if errors.Is(err, io.EOF) {
			if sawRecognizedLine {
				return Frame{}, ErrUnexpectedEOF
			}
			return Frame{}, io.EOF
		}
	}
}

func (d *Decoder) readLine() ([]byte, error) {
	line := make([]byte, 0, min(int64(d.reader.Size()), d.maxLineBytes))
	for {
		fragment, err := d.reader.ReadSlice('\n')
		if int64(len(line))+int64(len(fragment)) > d.maxLineBytes {
			return nil, fmt.Errorf("%w: limit %d bytes", ErrLineTooLarge, d.maxLineBytes)
		}
		line = append(line, fragment...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}

func parseLine(frame *Frame, dataLines *[]string, line string) (bool, error) {
	if strings.HasPrefix(line, ":") {
		comment := strings.TrimPrefix(line, ":")
		comment = strings.TrimPrefix(comment, " ")
		frame.Comments = append(frame.Comments, comment)
		return true, nil
	}
	field, value, found := strings.Cut(line, ":")
	if !found {
		value = ""
	}
	value = strings.TrimPrefix(value, " ")
	switch field {
	case "event":
		frame.Event = value
	case "id":
		frame.ID = value
	case "retry":
		retry, err := strconv.Atoi(value)
		if err != nil || retry < 0 {
			return true, fmt.Errorf("decode SSE retry %q: must be a non-negative integer", value)
		}
		frame.RetryMillis = &retry
	case "data":
		*dataLines = append(*dataLines, value)
	default:
		return false, nil
	}
	return true, nil
}

// Write writes one complete frame and its blank-line terminator.
func Write(writer io.Writer, frame Frame) error {
	if writer == nil {
		return fmt.Errorf("write SSE: nil writer")
	}
	for _, comment := range frame.Comments {
		if err := writeLine(writer, ": "+comment); err != nil {
			return err
		}
	}
	if frame.Event != "" {
		if err := writeLine(writer, "event: "+frame.Event); err != nil {
			return err
		}
	}
	if frame.ID != "" {
		if err := writeLine(writer, "id: "+frame.ID); err != nil {
			return err
		}
	}
	if frame.RetryMillis != nil {
		if *frame.RetryMillis < 0 {
			return fmt.Errorf("write SSE: retry must be non-negative")
		}
		if err := writeLine(writer, "retry: "+strconv.Itoa(*frame.RetryMillis)); err != nil {
			return err
		}
	}
	if frame.Data != nil {
		for _, line := range strings.Split(string(frame.Data), "\n") {
			if err := writeLine(writer, "data: "+line); err != nil {
				return err
			}
		}
	}
	if _, err := io.WriteString(writer, "\n"); err != nil {
		return fmt.Errorf("write SSE terminator: %w", err)
	}
	return nil
}

func writeLine(writer io.Writer, line string) error {
	if _, err := io.WriteString(writer, line+"\n"); err != nil {
		return fmt.Errorf("write SSE line: %w", err)
	}
	return nil
}
