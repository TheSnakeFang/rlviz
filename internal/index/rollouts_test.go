package index

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestQueryRolloutsFiltersPaginatesAndAggregatesSourceFacts(t *testing.T) {
	idx := openTestIndex(t)
	stream := []byte(`{"record_type":"run","id":"run-a","name":"nightly","metadata":{"checkpoint":"ckpt-42","model":"policy-small","environment_version":"env-v7"}}
{"record_type":"case","id":"case-a","run_id":"run-a","name":"checkout"}
{"record_type":"group","id":"group-a","case_id":"case-a","name":"sample-2"}
{"record_type":"trajectory","id":"good","group_id":"group-a","status":"completed","termination":"answer"}
{"record_type":"trajectory","id":"bad","group_id":"group-a","status":"failed","termination":"tool_error"}
{"record_type":"event","id":"good-tool","trajectory_id":"good","sequence":0,"kind":"tool","input":{"name":"shell"},"output":{"text":"ok"}}
{"record_type":"event","id":"good-answer","trajectory_id":"good","sequence":1,"kind":"generation","output":{"content":"done"}}
{"record_type":"event","id":"bad-tool","trajectory_id":"bad","sequence":0,"kind":"tool","input":{"name":"browser"}}
{"record_type":"event","id":"bad-error","trajectory_id":"bad","sequence":1,"kind":"error","data":{"message":"timeout"}}
{"record_type":"signal","id":"good-reward","trajectory_id":"good","name":"reward","value":1}
{"record_type":"signal","id":"good-pass","trajectory_id":"good","name":"pass","value":true}
{"record_type":"signal","id":"good-tokens","trajectory_id":"good","name":"token_count","value":100}
{"record_type":"signal","id":"good-cost","trajectory_id":"good","name":"cost_usd","value":0.3,"unit":"USD"}
{"record_type":"signal","id":"bad-reward","trajectory_id":"bad","name":"reward","value":0}
{"record_type":"signal","id":"bad-pass","trajectory_id":"bad","name":"pass","value":false}
{"record_type":"signal","id":"bad-tokens","trajectory_id":"bad","name":"token_count","value":200}
{"record_type":"signal","id":"bad-cost","trajectory_id":"bad","name":"cost_usd","value":0.7,"unit":"USD"}
{"record_type":"complete","records":17,"warnings":0}
`)
	if _, err := idx.Replace(t.Context(), Source{ID: "source-a", Path: "/traces/nightly.ndjson"}, bytes.NewReader(stream)); err != nil {
		t.Fatal(err)
	}

	first, err := idx.QueryRollouts(t.Context(), RolloutQuery{Limit: 1, Sort: "reward", Descending: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 2 || len(first.Items) != 1 || first.Items[0].Summary.Trajectory.Value.ID != "good" {
		t.Fatalf("first page = %#v", first)
	}
	if first.Aggregates.Success != 1 || first.Aggregates.Failure != 1 || first.Aggregates.Unknown != 0 || first.Aggregates.TotalCostUSD == nil || *first.Aggregates.TotalCostUSD != 1 {
		t.Fatalf("aggregates = %#v", first.Aggregates)
	}
	if first.Aggregates.ToolCallCount == nil || first.Aggregates.ToolCallCount.Min != 1 || first.Aggregates.ToolCallCount.Max != 1 {
		t.Fatalf("tool aggregate = %#v", first.Aggregates.ToolCallCount)
	}
	item := first.Items[0]
	if item.SourceName != "nightly.ndjson" || item.RunID != "run-a" || item.Checkpoint != "ckpt-42" || item.Model != "policy-small" || item.EnvironmentVersion != "env-v7" || item.Summary.CostUSD == nil || *item.Summary.CostUSD != 0.3 || item.Summary.ToolCallCount != 1 {
		t.Fatalf("summary = %#v", item)
	}

	failed := false
	filtered, err := idx.QueryRollouts(t.Context(), RolloutQuery{
		Checkpoint: "ckpt-42", Tool: "browser", Pass: &failed, RewardMax: float64Pointer(0), TokensMin: int64Pointer(150), Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].Summary.Trajectory.Value.ID != "bad" || filtered.Aggregates.TotalCostUSD == nil || *filtered.Aggregates.TotalCostUSD != 0.7 {
		t.Fatalf("filtered = %#v", filtered)
	}

	second, err := idx.QueryRollouts(t.Context(), RolloutQuery{Offset: 1, Limit: 1, Sort: "reward", Descending: true})
	if err != nil || len(second.Items) != 1 || second.Items[0].Summary.Trajectory.Value.ID != "bad" {
		t.Fatalf("second page = %#v err=%v", second, err)
	}
}

func TestQueryRolloutsRejectsNegativeUsageAndSortsUnknownLast(t *testing.T) {
	idx := openTestIndex(t)
	stream := []byte(`{"record_type":"run","id":"run","metadata":{"checkpoint":7,"model":false,"environment_version":{"bad":true}}}
{"record_type":"case","id":"case","run_id":"run"}
{"record_type":"group","id":"group","case_id":"case"}
{"record_type":"trajectory","id":"known","group_id":"group"}
{"record_type":"trajectory","id":"invalid","group_id":"group"}
{"record_type":"signal","id":"known-tokens","trajectory_id":"known","name":"token_count","value":10}
{"record_type":"signal","id":"invalid-tokens","trajectory_id":"invalid","name":"token_count","value":-2}
{"record_type":"signal","id":"invalid-cost","trajectory_id":"invalid","name":"cost_usd","value":-1}
{"record_type":"signal","id":"invalid-latency","trajectory_id":"invalid","name":"latency_ms","value":-3}
{"record_type":"complete","records":9,"warnings":0}
`)
	if _, err := idx.Replace(t.Context(), Source{ID: "negative", Path: "/traces/negative.ndjson"}, bytes.NewReader(stream)); err != nil {
		t.Fatal(err)
	}

	page, err := idx.QueryRollouts(t.Context(), RolloutQuery{Limit: 10, Sort: "tokens"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Summary.Trajectory.Value.ID != "known" || page.Items[1].Summary.Trajectory.Value.ID != "invalid" {
		t.Fatalf("token sort with unknown last = %#v", page.Items)
	}
	invalid := page.Items[1]
	if invalid.Summary.TokenCount != nil || invalid.Summary.CostUSD != nil || invalid.Summary.LatencyMS != nil {
		t.Fatalf("negative usage leaked into summary: %#v", invalid.Summary)
	}
	if invalid.Checkpoint != "" || invalid.Model != "" || invalid.EnvironmentVersion != "" {
		t.Fatalf("non-text dimensions leaked into summary: %#v", invalid)
	}
}

func TestQueryRolloutsBoundsSelectedSignals(t *testing.T) {
	idx := openTestIndex(t)
	var stream bytes.Buffer
	stream.WriteString("{\"record_type\":\"run\",\"id\":\"run\"}\n")
	stream.WriteString("{\"record_type\":\"case\",\"id\":\"case\",\"run_id\":\"run\"}\n")
	stream.WriteString("{\"record_type\":\"group\",\"id\":\"group\",\"case_id\":\"case\"}\n")
	stream.WriteString("{\"record_type\":\"trajectory\",\"id\":\"dense\",\"group_id\":\"group\"}\n")
	for index := range MaxGroupSummarySignals + 1 {
		fmt.Fprintf(&stream, "{\"record_type\":\"signal\",\"id\":\"signal-%d\",\"trajectory_id\":\"dense\",\"name\":\"custom-%d\",\"value\":%d}\n", index, index, index)
	}
	fmt.Fprintf(&stream, "{\"record_type\":\"complete\",\"records\":%d,\"warnings\":0}\n", MaxGroupSummarySignals+5)
	if _, err := idx.Replace(t.Context(), Source{ID: "dense", Path: "/traces/dense.ndjson"}, &stream); err != nil {
		t.Fatal(err)
	}

	_, err := idx.QueryRollouts(t.Context(), RolloutQuery{SourceID: "dense", Limit: 1})
	if !errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("error=%v, want ErrResultTooLarge", err)
	}
}

func float64Pointer(value float64) *float64 { return &value }
func int64Pointer(value int64) *int64       { return &value }
