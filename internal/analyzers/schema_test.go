package analyzers

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestAnalyzerProtocolFixturesConformToSchemasAndRuntime(t *testing.T) {
	t.Parallel()
	inputData := readProtocolFixture(t, "analyzer-input.json")
	outputData := readProtocolFixture(t, "analyzer-output.json")
	validateSchemaInstance(t, "analyzer-input.schema.json", inputData)
	validateSchemaInstance(t, "analyzer-output.schema.json", outputData)

	var input Input
	decodeStrictJSON(t, inputData, &input)
	if err := ValidateInput(input); err != nil {
		t.Fatalf("fixture input failed runtime validation: %v", err)
	}
	var output Output
	decodeStrictJSON(t, outputData, &output)
	digest, err := InputDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Provenance.InputDigest != digest {
		t.Fatalf("fixture input_digest = %q, want %q", output.Provenance.InputDigest, digest)
	}
	if err := ValidateOutput(output, input, output.Provenance); err != nil {
		t.Fatalf("fixture output failed runtime validation: %v", err)
	}
}

func TestAnalyzerSchemasRejectInvalidProtocolDocuments(t *testing.T) {
	t.Parallel()
	inputSchema := compileAnalyzerSchema(t, "analyzer-input.schema.json")
	outputSchema := compileAnalyzerSchema(t, "analyzer-output.schema.json")
	validInput := decodeSchemaJSON(t, readProtocolFixture(t, "analyzer-input.json")).(map[string]any)
	validOutput := decodeSchemaJSON(t, readProtocolFixture(t, "analyzer-output.json")).(map[string]any)

	for name, mutate := range map[string]func(map[string]any){
		"input unknown field": func(value map[string]any) { value["surprise"] = true },
		"input operation":     func(value map[string]any) { value["operation"] = "probe" },
		"input event kind": func(value map[string]any) {
			value["events"].([]any)[0].(map[string]any)["kind"] = "made_up"
		},
		"input signal value": func(value map[string]any) {
			value["signals"].([]any)[0].(map[string]any)["value"] = []any{true}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneSchemaValue(t, validInput)
			mutate(candidate)
			if err := inputSchema.Validate(candidate); err == nil {
				t.Fatal("schema accepted invalid analyzer input")
			}
		})
	}

	for name, mutate := range map[string]func(map[string]any){
		"output unknown field": func(value map[string]any) { value["surprise"] = true },
		"output digest": func(value map[string]any) {
			value["provenance"].(map[string]any)["digest"] = "SHA256:not-lowercase"
		},
		"output severity": func(value map[string]any) {
			value["findings"].([]any)[0].(map[string]any)["severity"] = "critical"
		},
		"output repeated event": func(value map[string]any) {
			value["findings"].([]any)[0].(map[string]any)["event_ids"] = []any{"event-1", "event-1"}
		},
		"output signal value": func(value map[string]any) {
			value["signals"].([]any)[0].(map[string]any)["value"] = map[string]any{"bad": true}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneSchemaValue(t, validOutput)
			mutate(candidate)
			if err := outputSchema.Validate(candidate); err == nil {
				t.Fatal("schema accepted invalid analyzer output")
			}
		})
	}
}

func validateSchemaInstance(t *testing.T, schemaName string, data []byte) {
	t.Helper()
	if err := compileAnalyzerSchema(t, schemaName).Validate(decodeSchemaJSON(t, data)); err != nil {
		t.Fatalf("%s: %v", schemaName, err)
	}
}

func compileAnalyzerSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "schemas", "v1alpha1", name))
	if err != nil {
		t.Fatal(err)
	}
	document := decodeSchemaJSON(t, data)
	compiler := jsonschema.NewCompiler()
	location := "https://rolloutviz.dev/schemas/v1alpha1/" + name
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func readProtocolFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "protocol", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeSchemaJSON(t *testing.T, data []byte) any {
	t.Helper()
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func decodeStrictJSON(t *testing.T, data []byte, value any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("fixture has trailing JSON: %v", err)
	}
}

func cloneSchemaValue(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	cloned := decodeSchemaJSON(t, data)
	result, ok := cloned.(map[string]any)
	if !ok {
		t.Fatalf("clone type = %T", cloned)
	}
	return result
}

func TestAnalyzerSchemaBoundsMatchProtocolConstants(t *testing.T) {
	t.Parallel()
	type property struct {
		MaxItems  int    `json:"maxItems"`
		MaxLength int    `json:"maxLength"`
		Pattern   string `json:"pattern"`
	}
	type definition struct {
		Properties map[string]property `json:"properties"`
		Pattern    string              `json:"pattern"`
	}
	type document struct {
		Properties map[string]property        `json:"properties"`
		Defs       map[string]json.RawMessage `json:"$defs"`
	}
	read := func(name string) document {
		data, err := os.ReadFile(filepath.Join("..", "..", "schemas", "v1alpha1", name))
		if err != nil {
			t.Fatal(err)
		}
		var schema document
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatal(err)
		}
		return schema
	}
	input := read("analyzer-input.schema.json")
	if input.Properties["events"].MaxItems != MaxInputEvents || input.Properties["signals"].MaxItems != MaxInputSignals {
		t.Fatalf("input array bounds = events %d signals %d", input.Properties["events"].MaxItems, input.Properties["signals"].MaxItems)
	}
	output := read("analyzer-output.schema.json")
	if output.Properties["findings"].MaxItems != MaxFindings || output.Properties["signals"].MaxItems != MaxOutputSignals {
		t.Fatalf("output array bounds = findings %d signals %d", output.Properties["findings"].MaxItems, output.Properties["signals"].MaxItems)
	}
	var finding, digest definition
	if err := json.Unmarshal(output.Defs["finding"], &finding); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Defs["digest"], &digest); err != nil {
		t.Fatal(err)
	}
	if finding.Properties["title"].MaxLength != MaxTextBytes || finding.Properties["summary"].MaxLength != MaxTextBytes {
		t.Fatalf("finding text bounds do not match %d", MaxTextBytes)
	}
	if pattern := digest.Pattern; pattern != `^sha256:[0-9a-f]{64}$` {
		t.Fatalf("digest pattern = %q", pattern)
	}
}
