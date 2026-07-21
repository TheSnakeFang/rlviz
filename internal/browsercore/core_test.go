package browsercore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestParseBuiltInBrowserFormats(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path         string
		format       string
		trajectories int
		firstEvents  int
	}{
		{"../../examples/gallery/coding-agent-bugfix.ndjson", "canonical-ndjson", 1, 300},
		{"../../examples/traces/inspect-ai-eval.json", "inspect-ai-eval-log-json-v2", 2, 5},
		{"../../examples/traces/verifiers-generate.json", "prime-verifiers-generate-outputs", 1, 3},
	}
	for _, test := range cases {
		test := test
		t.Run(filepath.Base(test.path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := Parse(data, filepath.Base(test.path))
			if err != nil {
				t.Fatal(err)
			}
			collection, err := DecodeCollection(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if collection.Source.Format != test.format {
				t.Fatalf("format = %q", collection.Source.Format)
			}
			repeated, err := Parse(data, filepath.Base(test.path))
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != string(repeated) {
				t.Fatal("browser normalization is not deterministic")
			}
			if got := len(collection.Trajectories); got != test.trajectories {
				t.Fatalf("trajectories = %d, want %d", got, test.trajectories)
			}
			for _, trajectory := range collection.Trajectories {
				if len(trajectory.Events) == test.firstEvents {
					return
				}
			}
			t.Fatalf("no trajectory with %d events", test.firstEvents)
		})
	}
}

func TestParseRejectsTraceAboveBrowserCeiling(t *testing.T) {
	_, err := Parse(make([]byte, MaxRecommendedBytes+1), "large.ndjson")
	if err == nil || !strings.Contains(err.Error(), "browser maximum") {
		t.Fatalf("error = %v", err)
	}
	_, err = ParseCanonical(make([]byte, MaxRecommendedBytes+1), "expanded.ndjson", "browser-wasm-adapter", 1)
	if err == nil || !strings.Contains(err.Error(), "canonical trace") {
		t.Fatalf("canonical error = %v", err)
	}
}

func TestParseCanonicalPreservesTrajectoryRecordOrder(t *testing.T) {
	canonical := strings.Join([]string{
		`{"record_type":"run","id":"run"}`,
		`{"record_type":"case","id":"case","run_id":"run"}`,
		`{"record_type":"group","id":"group","case_id":"case"}`,
		`{"record_type":"trajectory","id":"z-first","group_id":"group"}`,
		`{"record_type":"trajectory","id":"a-second","group_id":"group"}`,
		`{"record_type":"event","id":"z-event","trajectory_id":"z-first","sequence":0,"kind":"message"}`,
		`{"record_type":"event","id":"a-event","trajectory_id":"a-second","sequence":0,"kind":"message"}`,
		`{"record_type":"complete","records":7,"warnings":0}`,
	}, "\n") + "\n"
	collection, err := ParseCanonical([]byte(canonical), "ordered.ndjson", "canonical-ndjson", len(canonical))
	if err != nil {
		t.Fatal(err)
	}
	rows := collection.Browse["trajectories"].([]BrowseRow)
	if rows[0].Trajectory.ID != "z-first" || rows[1].Trajectory.ID != "a-second" {
		t.Fatalf("browse order = %q, %q", rows[0].Trajectory.ID, rows[1].Trajectory.ID)
	}
}

func TestCompareReturnsDaemonErrorCodeAboveAlignmentCap(t *testing.T) {
	left := make([]model.Event, 5_001)
	right := make([]model.Event, 5_001)
	for index := range left {
		left[index] = model.Event{ID: "left", Sequence: int64(index), Kind: "message", AlignmentKey: "left"}
		right[index] = model.Event{ID: "right", Sequence: int64(index), Kind: "tool", AlignmentKey: "right"}
	}
	collection := Collection{Trajectories: map[string]TrajectoryData{
		"left":  {Trajectory: model.Trajectory{ID: "left"}, Events: left},
		"right": {Trajectory: model.Trajectory{ID: "right"}, Events: right},
	}}
	_, err := Compare(collection, "left", "right")
	coded, ok := err.(interface{ ErrorCode() string })
	if !ok || coded.ErrorCode() != "comparison_too_large" {
		t.Fatalf("error = %#v", err)
	}
}

func TestCompareMatchesDaemonResearchFields(t *testing.T) {
	contextTokens := int64(10)
	collection := Collection{Source: Source{ID: "source"}, Trajectories: map[string]TrajectoryData{
		"left": {
			Run: &model.Run{ID: "run-left"}, Case: &model.Case{ID: "case-left"}, Group: &model.Group{ID: "group-left"}, Trajectory: model.Trajectory{ID: "left", Status: "failed"},
			Events:  []model.Event{{ID: "left-context", Kind: "state", Context: &model.Context{Operation: "compaction", InputTokens: &contextTokens, Provenance: "source_native"}}, {ID: "left-grader", Sequence: 1, Kind: "grader", AlignmentKey: "grader:final", Output: map[string]any{"verdict": "fail"}}},
			Signals: []model.Signal{{Name: "reward", Value: json.Number("0")}, {Name: "pass", Value: false}, {Name: "token_count", Value: json.Number("10")}},
		},
		"right": {
			Run: &model.Run{ID: "run-right"}, Case: &model.Case{ID: "case-right"}, Group: &model.Group{ID: "group-right"}, Trajectory: model.Trajectory{ID: "right", Status: "completed"},
			Events:  []model.Event{{ID: "right-grader", Kind: "grader", AlignmentKey: "grader:final", Output: map[string]any{"verdict": "pass"}}},
			Signals: []model.Signal{{Name: "reward", Value: json.Number("1")}, {Name: "pass", Value: true}, {Name: "token_count", Value: json.Number("12")}},
		},
	}}
	result, err := Compare(collection, "left", "right")
	if err != nil {
		t.Fatal(err)
	}
	left := result["left"].(map[string]any)
	if left["run"].(*model.Run).ID != "run-left" || left["context"] == nil {
		t.Fatalf("left side = %#v", left)
	}
	differences := result["differences"].(map[string]any)
	for _, field := range []string{"success", "token_count", "context_event_count", "compaction_count", "verifier_results"} {
		if _, ok := differences[field]; !ok {
			t.Fatalf("missing difference %q", field)
		}
	}
	if differences["token_count"].(map[string]any)["delta"] != int64(2) {
		t.Fatalf("token difference = %#v", differences["token_count"])
	}
}

func TestRecommendedSizeCeilingExceedsGallery(t *testing.T) {
	data, err := os.ReadFile("../../examples/gallery/coding-agent-bugfix.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) >= MaxRecommendedBytes {
		t.Fatalf("300-event gallery fixture is %d bytes", len(data))
	}
}
