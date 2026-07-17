package model

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestContextFixtureDecodesStructuredObservations(t *testing.T) {
	t.Parallel()
	data := readContextFixture(t)

	events := make(map[string]*Event)
	if err := Decode(bytes.NewReader(data), func(record *Record) error {
		if event, ok := record.Value.(*Event); ok && event.Context != nil {
			events[event.ID] = event
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(events) != 5 {
		t.Fatalf("structured context events = %d, want 5", len(events))
	}
	compaction := events["ctx-compaction"].Context
	if compaction.Operation != "compaction" || compaction.Provenance != "source_native" {
		t.Fatalf("compaction identity = %#v", compaction)
	}
	if compaction.InputTokens == nil || *compaction.InputTokens != 256 ||
		compaction.InputTokensBefore == nil || *compaction.InputTokensBefore != 900 ||
		compaction.Capacity == nil || *compaction.Capacity != 1024 {
		t.Fatalf("compaction token observation = %#v", compaction)
	}
	if got := strings.Join(compaction.RetainedEventIDs, ","); got != "ctx-tool" {
		t.Errorf("retained ids = %q, want ctx-tool", got)
	}
	if got := strings.Join(compaction.SummarizedEventIDs, ","); got != "ctx-user,ctx-observation" {
		t.Errorf("summarized ids = %q", got)
	}
	if compaction.Summary == "" {
		t.Error("compaction summary is empty")
	}

	derived := events["ctx-derived"].Context
	if derived.Provenance != "adapter_derived" || derived.Derivation == "" {
		t.Fatalf("derived context provenance = %#v", derived)
	}
	truncation := events["ctx-truncation"].Context
	if truncation.Operation != "truncation" || strings.Join(truncation.DroppedEventIDs, ",") != "ctx-observation" {
		t.Fatalf("truncation context = %#v", truncation)
	}
	restore := events["ctx-restore"].Context
	if restore.Operation != "restore" || strings.Join(restore.RetainedEventIDs, ",") != "ctx-user,ctx-tool" {
		t.Fatalf("restore context = %#v", restore)
	}
}

func TestContextFixtureMatchesCanonicalSchema(t *testing.T) {
	t.Parallel()
	schemaData, err := os.ReadFile(filepath.Join("..", "..", "schemas", "v1alpha1", "canonical-record.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	location := "https://rlviz.dev/schemas/v1alpha1/canonical-record.schema.json"
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(readContextFixture(t)))
	for line := 1; scanner.Scan(); line++ {
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(scanner.Bytes()))
		if err != nil {
			t.Fatalf("line %d: %v", line, err)
		}
		if err := schema.Validate(value); err != nil {
			t.Fatalf("line %d: %v", line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestContextValidationFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		context string
		prior   []string
		field   string
	}{
		{name: "missing provenance", context: `{"input_tokens":1}`, field: "context.provenance"},
		{name: "unsupported provenance", context: `{"input_tokens":1,"provenance":"manual"}`, field: "context.provenance"},
		{name: "derived without derivation", context: `{"input_tokens":1,"provenance":"adapter_derived"}`, field: "context.derivation"},
		{name: "unsupported operation", context: `{"operation":"eviction","provenance":"source_native"}`, field: "context.operation"},
		{name: "before without operation", context: `{"input_tokens_before":2,"input_tokens":1,"provenance":"source_native"}`, field: "context.input_tokens_before"},
		{name: "negative input tokens", context: `{"input_tokens":-1,"provenance":"source_native"}`, field: "context.input_tokens"},
		{name: "negative input tokens before", context: `{"operation":"compaction","input_tokens_before":-1,"provenance":"source_native"}`, field: "context.input_tokens_before"},
		{name: "nonpositive capacity", context: `{"capacity":0,"provenance":"source_native"}`, field: "context.capacity"},
		{name: "no observation", context: `{"provenance":"source_native"}`, field: "context"},
		{name: "empty reference", context: `{"retained_event_ids":[""],"provenance":"source_native"}`, field: "context.retained_event_ids"},
		{name: "unknown reference", context: `{"retained_event_ids":["later"],"provenance":"source_native"}`, field: "context.retained_event_ids"},
		{
			name:    "reference in multiple groups",
			context: `{"retained_event_ids":["prior"],"dropped_event_ids":["prior"],"provenance":"source_native"}`,
			prior:   []string{`{"record_type":"event","id":"prior","trajectory_id":"a","sequence":0,"kind":"message"}`},
			field:   "context.dropped_event_ids",
		},
		{
			name:    "reference from another trajectory",
			context: `{"summarized_event_ids":["foreign"],"provenance":"source_native"}`,
			prior:   []string{`{"record_type":"event","id":"foreign","trajectory_id":"b","sequence":0,"kind":"message"}`},
			field:   "context.summarized_event_ids",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream := contextValidationStream(tt.context, tt.prior...)
			err := Decode(strings.NewReader(stream), nil)
			var recordError *RecordValidationError
			if !errors.As(err, &recordError) {
				t.Fatalf("error = %v, want RecordValidationError", err)
			}
			if recordError.RecordType != RecordEvent || recordError.RecordID != "target" || recordError.Field != tt.field {
				t.Fatalf("record error = %#v, want event target field %q", recordError, tt.field)
			}
		})
	}
}

func contextValidationStream(context string, prior ...string) string {
	records := []string{
		`{"record_type":"run","id":"run"}`,
		`{"record_type":"case","id":"case","run_id":"run"}`,
		`{"record_type":"group","id":"group","case_id":"case"}`,
		`{"record_type":"trajectory","id":"a","group_id":"group"}`,
		`{"record_type":"trajectory","id":"b","group_id":"group"}`,
	}
	records = append(records, prior...)
	records = append(records, `{"record_type":"event","id":"target","trajectory_id":"a","sequence":10,"kind":"state","context":`+context+`}`)
	return lines(records...)
}

func readContextFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "context.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
