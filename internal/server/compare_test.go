package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	rolloutindex "github.com/unlatch-ai/rolloutviz/internal/index"
	"github.com/unlatch-ai/rolloutviz/internal/model"
)

func comparisonHandler(t *testing.T, eventsPerLeft int) http.Handler {
	right := 1
	if eventsPerLeft == 3 {
		right = 3
	}
	return comparisonHandlerCounts(t, eventsPerLeft, right, eventsPerLeft == 3)
}

func comparisonHandlerCounts(t *testing.T, eventsPerLeft, eventsPerRight int, divergent bool) http.Handler {
	t.Helper()
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "comparison.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	records := []any{
		&model.Run{RecordType: model.RecordRun, ID: "run"},
		&model.Case{RecordType: model.RecordCase, ID: "case", RunID: "run"},
		&model.Group{RecordType: model.RecordGroup, ID: "group", CaseID: "case"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "left", GroupID: "group", Status: "failed", Termination: "timeout"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "right", GroupID: "group", Status: "completed", Termination: "answer"},
	}
	if divergent && eventsPerLeft == 3 && eventsPerRight == 3 {
		records = append(records,
			&model.Event{RecordType: model.RecordEvent, ID: "left-a", TrajectoryID: "left", Sequence: 0, Kind: "tool", AlignmentKey: "a"},
			&model.Event{RecordType: model.RecordEvent, ID: "left-b", TrajectoryID: "left", Sequence: 1, Kind: "tool", AlignmentKey: "b"},
			&model.Event{RecordType: model.RecordEvent, ID: "left-c", TrajectoryID: "left", Sequence: 2, Kind: "tool", AlignmentKey: "c"},
			&model.Event{RecordType: model.RecordEvent, ID: "right-a", TrajectoryID: "right", Sequence: 0, Kind: "tool", AlignmentKey: "a"},
			&model.Event{RecordType: model.RecordEvent, ID: "right-x", TrajectoryID: "right", Sequence: 1, Kind: "error", AlignmentKey: "x"},
			&model.Event{RecordType: model.RecordEvent, ID: "right-c", TrajectoryID: "right", Sequence: 2, Kind: "tool", AlignmentKey: "c"},
		)
	} else if divergent {
		for sequence := 0; sequence < eventsPerLeft; sequence++ {
			records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("left-%d", sequence), TrajectoryID: "left", Sequence: int64(sequence), Kind: "tool", AlignmentKey: "left"})
		}
		for sequence := 0; sequence < eventsPerRight; sequence++ {
			records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("right-%d", sequence), TrajectoryID: "right", Sequence: int64(sequence), Kind: "tool", AlignmentKey: "right"})
		}
	} else {
		for sequence := 0; sequence < eventsPerLeft; sequence++ {
			records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("left-%d", sequence), TrajectoryID: "left", Sequence: int64(sequence), Kind: "message"})
		}
		for sequence := 0; sequence < eventsPerRight; sequence++ {
			records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("right-%d", sequence), TrajectoryID: "right", Sequence: int64(sequence), Kind: "message"})
		}
	}
	records = append(records,
		&model.Signal{RecordType: model.RecordSignal, ID: "left-reward", TrajectoryID: "left", Name: "reward", Value: 0.0},
		&model.Signal{RecordType: model.RecordSignal, ID: "right-reward", TrajectoryID: "right", Name: "reward", Value: 1.0},
		&model.Artifact{RecordType: model.RecordArtifact, ID: "right-log", TrajectoryID: "right", Name: "log", MediaType: "text/plain", Text: "done"},
	)
	records = append(records, &model.Complete{RecordType: model.RecordComplete, Records: int64(len(records))})
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.Replace(t.Context(), rolloutindex.Source{ID: "source", Path: "fixture.ndjson", Fingerprint: "fixture", Size: int64(input.Len()), ModTime: time.Unix(1, 0)}, bytes.NewReader(input.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return NewIndexedHandler(store, "secret")
}

func TestIndexedCompareLoadsEveryPaginatedEvent(t *testing.T) {
	handler := comparisonHandlerCounts(t, 401, 401, false)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	payload := decodeIndexedResponse(t, response)
	if got := len(payload["left"].(map[string]any)["events"].([]any)); got != 401 {
		t.Fatalf("left event count = %d", got)
	}
	if got := len(payload["right"].(map[string]any)["events"].([]any)); got != 401 {
		t.Fatalf("right event count = %d", got)
	}
	if got := len(payload["alignment"].(map[string]any)["steps"].([]any)); got != 401 {
		t.Fatalf("alignment step count = %d", got)
	}
}

type bytePagedComparisonReader struct{ IndexedReader }

func (bytePagedComparisonReader) Events(_ context.Context, query rolloutindex.EventQuery) (rolloutindex.EventPage, error) {
	sequence := int64(0)
	if query.AfterSequence != nil {
		sequence = *query.AfterSequence + 1
	}
	page := rolloutindex.EventPage{
		Events:   []rolloutindex.IndexedRecord[*model.Event]{{Value: &model.Event{ID: fmt.Sprintf("event-%d", sequence), TrajectoryID: query.TrajectoryID, Sequence: sequence, Kind: "tool"}}},
		Total:    3,
		RawBytes: MaxComparisonRawBytes / 2,
	}
	if sequence < 2 {
		page.NextSequence = &sequence
	} else {
		page.RawBytes = 1
	}
	return page, nil
}

func TestComparisonRejectsCumulativeRawByteMaxPlusOne(t *testing.T) {
	api := indexedAPI{reader: bytePagedComparisonReader{}}
	_, _, _, err := api.allComparisonEvents(t.Context(), "source", "trajectory")
	if !errors.Is(err, errComparisonTooLarge) {
		t.Fatalf("error = %v, want errComparisonTooLarge", err)
	}
}

func TestIndexedCompareReturnsDivergenceRealignmentAndDifferences(t *testing.T) {
	handler := comparisonHandler(t, 3)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	payload := decodeIndexedResponse(t, response)
	aligned := payload["alignment"].(map[string]any)
	if aligned["first_meaningful_divergence"] != float64(1) || aligned["later_realignment"] != float64(2) {
		t.Fatalf("alignment = %#v", aligned)
	}
	if len(aligned["steps"].([]any)) != 3 {
		t.Fatalf("steps = %#v", aligned["steps"])
	}
	left := payload["left"].(map[string]any)
	right := payload["right"].(map[string]any)
	if left["trajectory"].(map[string]any)["id"] != "left" || len(left["events"].([]any)) != 3 || len(right["artifacts"].([]any)) != 1 {
		t.Fatalf("sides = left:%#v right:%#v", left, right)
	}
	differences := payload["differences"].(map[string]any)
	if differences["status"].(map[string]any)["changed"] != true || differences["termination"].(map[string]any)["changed"] != true {
		t.Fatalf("differences = %#v", differences)
	}
	reward := differences["reward"].(map[string]any)
	if reward["left"] != float64(0) || reward["right"] != float64(1) || reward["changed"] != true {
		t.Fatalf("reward = %#v", reward)
	}
}

func TestIndexedCompareRequiresAuthenticationAndStrictQuery(t *testing.T) {
	handler := comparisonHandler(t, 3)
	target := "/api/v1/indexed/compare?trajectory=source&left=left&right=right"
	unauthorized := indexedRequest(t, handler, http.MethodGet, target, false)
	if unauthorized.Code != http.StatusUnauthorized || decodeIndexedResponse(t, unauthorized)["code"] != "unauthorized" {
		t.Fatalf("unauthorized = %d %s", unauthorized.Code, unauthorized.Body.String())
	}
	for _, invalid := range []string{
		"/api/v1/indexed/compare?trajectory=source&left=left",
		"/api/v1/indexed/compare?trajectory=source&left=left&right=left",
		"/api/v1/indexed/compare?trajectory=source&left=left&right=right&extra=x",
		"/api/v1/indexed/compare?trajectory=source&left=left&left=other&right=right",
	} {
		response := indexedRequest(t, handler, http.MethodGet, invalid, true)
		if response.Code != http.StatusBadRequest || decodeIndexedResponse(t, response)["code"] != "invalid_query" {
			t.Errorf("%s = %d %s", invalid, response.Code, response.Body.String())
		}
	}
}

func TestIndexedCompareReportsMissingSide(t *testing.T) {
	handler := comparisonHandler(t, 3)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=missing&right=right", true)
	if response.Code != http.StatusNotFound || decodeIndexedResponse(t, response)["code"] != "left_trajectory_not_found" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestIndexedCompareEnforcesEventLimitFromRealIndex(t *testing.T) {
	handler := comparisonHandler(t, MaxComparisonEvents+1)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusRequestEntityTooLarge || decodeIndexedResponse(t, response)["code"] != "comparison_too_large" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestIndexedCompareAllowsLongComparableTraces(t *testing.T) {
	handler := comparisonHandlerCounts(t, 5_000, 5_000, false)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if got := len(decodeIndexedResponse(t, response)["alignment"].(map[string]any)["steps"].([]any)); got != 5_000 {
		t.Fatalf("alignment steps = %d", got)
	}
}

func TestIndexedCompareBoundsPathologicalDivergentWork(t *testing.T) {
	handler := comparisonHandlerCounts(t, 5_000, 5_000, true)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusRequestEntityTooLarge || decodeIndexedResponse(t, response)["code"] != "comparison_too_large" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}
