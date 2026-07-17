package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxRecordBytes is the largest canonical NDJSON record accepted by Decoder.
// The bound applies to the JSON bytes and excludes the trailing CRLF or LF.
const MaxRecordBytes = 8 << 20

// ErrRecordTooLarge identifies a canonical record that exceeds MaxRecordBytes.
var ErrRecordTooLarge = errors.New("canonical record exceeds maximum size")

type Decoder struct {
	reader *bufio.Reader
	line   int64
	offset int64
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{reader: bufio.NewReaderSize(r, 64<<10)}
}

// Next decodes one NDJSON record without buffering the entire stream.
func (d *Decoder) Next() (*Record, error) {
	return d.NextContext(context.Background())
}

// NextContext decodes one NDJSON record and checks ctx while reading and before
// parsing it. Cancellation cannot interrupt an io.Reader whose Read is itself
// blocked; callers that require that behavior should use a context-aware reader.
func (d *Decoder) NextContext(ctx context.Context) (*Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	start := d.offset
	line, err := d.readLine(ctx)
	if err != nil {
		return nil, err
	}
	d.line++
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, fmt.Errorf("line %d: blank records are not allowed", d.line)
	}

	var envelope struct {
		RecordType RecordType `json:"record_type"`
	}
	if decodeErr := strictDecode(line, &envelope); decodeErr != nil {
		// The envelope intentionally accepts unknown fields. Decode it normally,
		// then apply strict decoding to the selected concrete record below.
		if jsonErr := json.Unmarshal(line, &envelope); jsonErr != nil {
			return nil, fmt.Errorf("line %d: invalid JSON: %w", d.line, jsonErr)
		}
	}
	if envelope.RecordType == "" {
		return nil, fmt.Errorf("line %d: record_type is required", d.line)
	}

	var value any
	switch envelope.RecordType {
	case RecordRun:
		value = &Run{}
	case RecordCase:
		value = &Case{}
	case RecordGroup:
		value = &Group{}
	case RecordTrajectory:
		value = &Trajectory{}
	case RecordEvent:
		value = &Event{}
	case RecordSignal:
		value = &Signal{}
	case RecordArtifact:
		value = &Artifact{}
	case RecordComplete:
		value = &Complete{}
	default:
		return nil, fmt.Errorf("line %d: unsupported record_type %q", d.line, envelope.RecordType)
	}
	if decodeErr := strictDecode(line, value); decodeErr != nil {
		return nil, fmt.Errorf("line %d: invalid %s record: %w", d.line, envelope.RecordType, decodeErr)
	}

	raw := append(json.RawMessage(nil), line...)
	return &Record{
		Type: envelope.RecordType, Value: value, Raw: raw, Line: d.line,
		ByteOffset: start, ByteLength: int64(len(line)),
	}, nil
}

func (d *Decoder) readLine(ctx context.Context) ([]byte, error) {
	var line []byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fragment, err := d.reader.ReadSlice('\n')
		d.offset += int64(len(fragment))
		line = append(line, fragment...)

		switch {
		case err == nil:
			line = line[:len(line)-1]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if len(line) > MaxRecordBytes {
				return nil, fmt.Errorf("line %d: %w (%d > %d bytes)", d.line+1, ErrRecordTooLarge, len(line), MaxRecordBytes)
			}
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			// One extra byte is permitted while reading because it may be the CR
			// in a CRLF terminator split across buffer reads.
			if len(line) > MaxRecordBytes+1 {
				return nil, fmt.Errorf("line %d: %w (> %d bytes)", d.line+1, ErrRecordTooLarge, MaxRecordBytes)
			}
			continue
		case errors.Is(err, io.EOF):
			if len(line) == 0 {
				return nil, io.EOF
			}
			// Preserve Decode's historical handling of a final CR-terminated
			// record even when the stream omits the following LF.
			if line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if len(line) > MaxRecordBytes {
				return nil, fmt.Errorf("line %d: %w (%d > %d bytes)", d.line+1, ErrRecordTooLarge, len(line), MaxRecordBytes)
			}
			return line, nil
		default:
			return nil, err
		}
	}
}

func strictDecode(data []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values in one record")
		}
		return err
	}
	return nil
}

// Decode validates and visits a stream record by record.
func Decode(r io.Reader, visit func(*Record) error) error {
	return DecodeContext(context.Background(), r, visit)
}

// DecodeContext validates and visits a stream record by record until completion
// or cancellation. Decode remains the context-free compatibility entry point.
func DecodeContext(ctx context.Context, r io.Reader, visit func(*Record) error) error {
	decoder := NewDecoder(r)
	validator := NewValidator()
	for {
		record, err := decoder.NextContext(ctx)
		if errors.Is(err, io.EOF) {
			return validator.Finish()
		}
		if err != nil {
			return err
		}
		if err := validator.Add(record); err != nil {
			recordError := &RecordValidationError{Line: record.Line, RecordType: record.Type, RecordID: RecordID(record), Err: err}
			var fieldError *FieldValidationError
			if errors.As(err, &fieldError) {
				recordError.Field = fieldError.Field
			}
			return recordError
		}
		if visit != nil {
			if err := visit(record); err != nil {
				return err
			}
		}
	}
}
