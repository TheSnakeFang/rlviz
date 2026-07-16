package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecoderByteProvenanceCRLFAndNoFinalNewline(t *testing.T) {
	t.Parallel()
	first := `{"record_type":"run","id":"run-offsets"}`
	second := `{"record_type":"complete","records":1,"warnings":0}`
	decoder := NewDecoder(strings.NewReader(first + "\r\n" + second))

	record, err := decoder.Next()
	if err != nil {
		t.Fatal(err)
	}
	if record.Line != 1 || record.ByteOffset != 0 || record.ByteLength != int64(len(first)) {
		t.Fatalf("first provenance = line %d offset %d length %d", record.Line, record.ByteOffset, record.ByteLength)
	}
	if string(record.Raw) != first {
		t.Fatalf("first raw = %q, want %q", record.Raw, first)
	}

	record, err = decoder.Next()
	if err != nil {
		t.Fatal(err)
	}
	if record.Line != 2 || record.ByteOffset != int64(len(first)+2) || record.ByteLength != int64(len(second)) {
		t.Fatalf("second provenance = line %d offset %d length %d", record.Line, record.ByteOffset, record.ByteLength)
	}
	if string(record.Raw) != second {
		t.Fatalf("second raw = %q, want %q", record.Raw, second)
	}
	if _, err := decoder.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("final error = %v, want EOF", err)
	}
}

func TestDecoderRejectsOversizedRecord(t *testing.T) {
	t.Parallel()
	stream := `{"record_type":"run","id":"run-large","metadata":{"payload":"` +
		strings.Repeat("x", MaxRecordBytes) + `"}}` + "\n"
	_, err := NewDecoder(strings.NewReader(stream)).Next()
	if !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("error = %v, want ErrRecordTooLarge", err)
	}
}

func TestDecodeContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	visited := 0
	err := DecodeContext(ctx, bytes.NewReader(tenThousandEventStream()), func(*Record) error {
		visited++
		if visited == 100 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if visited != 100 {
		t.Fatalf("visited = %d, want 100", visited)
	}
}

func TestDecodeTenThousandEvents(t *testing.T) {
	t.Parallel()
	visited := 0
	if err := Decode(bytes.NewReader(tenThousandEventStream()), func(*Record) error {
		visited++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if visited != 10_005 {
		t.Fatalf("visited = %d, want 10005", visited)
	}
}

func TestCanonicalFixtures(t *testing.T) {
	t.Parallel()
	patterns := []string{
		filepath.Join("..", "..", "fixtures", "canonical", "*.ndjson"),
		filepath.Join("..", "..", "fixtures", "adversarial", "*.ndjson"),
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 0 {
			t.Fatalf("no fixtures matched %s", pattern)
		}
		for _, path := range files {
			path := path
			t.Run(filepath.Base(path), func(t *testing.T) {
				t.Parallel()
				file, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				var records int
				if err := Decode(file, func(record *Record) error {
					records++
					if len(record.Raw) == 0 {
						t.Fatal("raw record was not retained")
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if records < 2 {
					t.Fatalf("decoded only %d records", records)
				}
			})
		}
	}
}

func TestMalformedFixtures(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"unknown-parent.ndjson":     "unknown or later parent",
		"out-of-order.ndjson":       "not greater than prior sequence",
		"duplicate-id.ndjson":       "duplicate id",
		"complete-not-final.ndjson": "complete must be the final record",
		"unknown-field.ndjson":      "unknown field",
		"invalid-json.ndjson":       "invalid JSON",
	}
	for name, want := range tests {
		name, want := name, want
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			file, err := os.Open(filepath.Join("..", "..", "fixtures", "malformed", name))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			err = Decode(file, nil)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("Decode() error = %v, want substring %q", err, want)
			}
		})
	}
}

func TestDecoderStreamsLargeRecords(t *testing.T) {
	t.Parallel()
	payload := strings.Repeat("x", 256*1024)
	stream := strings.Join([]string{
		`{"record_type":"run","id":"run-large","metadata":{"payload":"` + payload + `"}}`,
		`{"record_type":"complete","records":1,"warnings":0}`,
		"",
	}, "\n")
	decoder := NewDecoder(strings.NewReader(stream))
	record, err := decoder.Next()
	if err != nil {
		t.Fatal(err)
	}
	if record.Type != RecordRun {
		t.Fatalf("type = %q, want run", record.Type)
	}
	if len(record.Raw) < len(payload) {
		t.Fatalf("raw record unexpectedly short: %d", len(record.Raw))
	}
	if _, err := decoder.Next(); err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("final error = %v, want EOF", err)
	}
}

func TestDecodeRequiresComplete(t *testing.T) {
	t.Parallel()
	err := Decode(strings.NewReader(`{"record_type":"run","id":"run-incomplete"}`+"\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "without a complete") {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodePropagatesVisitorError(t *testing.T) {
	t.Parallel()
	want := errors.New("stop")
	err := Decode(strings.NewReader(`{"record_type":"run","id":"run-stop"}`+"\n"), func(*Record) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func BenchmarkDecodeTenThousandEvents(b *testing.B) {
	stream := tenThousandEventStream()
	b.ReportAllocs()
	b.SetBytes(int64(len(stream)))
	b.ResetTimer()
	for range b.N {
		if err := Decode(bytes.NewReader(stream), nil); err != nil {
			b.Fatal(err)
		}
	}
}

func tenThousandEventStream() []byte {
	var stream bytes.Buffer
	stream.Grow(1 << 20)
	stream.WriteString("{\"record_type\":\"run\",\"id\":\"run-10k\"}\n")
	stream.WriteString("{\"record_type\":\"case\",\"id\":\"case-10k\",\"run_id\":\"run-10k\"}\n")
	stream.WriteString("{\"record_type\":\"group\",\"id\":\"group-10k\",\"case_id\":\"case-10k\"}\n")
	stream.WriteString("{\"record_type\":\"trajectory\",\"id\":\"trajectory-10k\",\"group_id\":\"group-10k\"}\n")
	for i := range 10_000 {
		fmt.Fprintf(&stream, "{\"record_type\":\"event\",\"id\":\"event-%d\",\"trajectory_id\":\"trajectory-10k\",\"sequence\":%d,\"kind\":\"log\"}\n", i, i)
	}
	stream.WriteString("{\"record_type\":\"complete\",\"records\":10004,\"warnings\":0}")
	return stream.Bytes()
}
