// Package browsercore builds an in-memory viewer collection from trace bytes.
// It contains no filesystem, database, process, or network access and is safe
// to compile for GOOS=js GOARCH=wasm.
package browsercore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/TheSnakeFang/rlviz/internal/alignment"
	"github.com/TheSnakeFang/rlviz/internal/analyzers"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

const MaxRecommendedBytes = 32 << 20

type CodedError struct {
	Code    string
	Message string
}

func (err *CodedError) Error() string     { return err.Message }
func (err *CodedError) ErrorCode() string { return err.Code }

type Source struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Format     string `json:"format"`
	Size       int    `json:"size"`
	IndexState string `json:"index_state"`
}

type BrowseRow struct {
	SourceID   string           `json:"source_id"`
	SourceName string           `json:"source_name"`
	RunName    string           `json:"run_name,omitempty"`
	CaseName   string           `json:"case_name,omitempty"`
	GroupName  string           `json:"group_name,omitempty"`
	Trajectory model.Trajectory `json:"trajectory"`
	Metrics    map[string]any   `json:"metrics"`
}

type TrajectoryData struct {
	Trajectory model.Trajectory `json:"trajectory"`
	Events     []model.Event    `json:"events"`
	Signals    []model.Signal   `json:"signals"`
	Artifacts  []model.Artifact `json:"artifacts"`
	Run        *model.Run       `json:"run,omitempty"`
	Case       *model.Case      `json:"case,omitempty"`
	Group      *model.Group     `json:"group,omitempty"`
	Source     Source           `json:"source"`
	Page       map[string]any   `json:"page"`
}

type Collection struct {
	Source       Source                    `json:"source"`
	Browse       map[string]any            `json:"browse"`
	Trajectories map[string]TrajectoryData `json:"trajectories"`
}

// Parse detects a built-in browser format, normalizes it to canonical NDJSON,
// validates the stream, and builds the complete in-memory collection.
func Parse(source []byte, name string) ([]byte, error) {
	collection, err := ParseCollection(source, name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(collection)
}

// ParseCollection returns the decoded collection without an intermediate JSON
// round trip. The WASM bridge caches this value for analysis and comparison.
func ParseCollection(source []byte, name string) (Collection, error) {
	if len(source) > MaxRecommendedBytes {
		return Collection{}, fmt.Errorf("trace is %d bytes; browser maximum is %d bytes", len(source), MaxRecommendedBytes)
	}
	canonical, format, err := Normalize(source, name)
	if err != nil {
		return Collection{}, err
	}
	return ParseCanonical(canonical, name, format, len(source))
}

// Normalize returns validated canonical input for built-in formats.
func Normalize(source []byte, name string) ([]byte, string, error) {
	trimmed := bytes.TrimSpace(source)
	if len(trimmed) == 0 {
		return nil, "", errors.New("trace is empty")
	}
	if bytes.Contains(firstLine(trimmed), []byte(`"record_type"`)) {
		return source, "canonical-ndjson", nil
	}
	var document map[string]any
	if err := json.Unmarshal(trimmed, &document); err != nil {
		return nil, "", errors.New("unsupported trace: expected canonical NDJSON, Inspect AI EvalLog JSON, or Verifiers GenerateOutputs JSON")
	}
	if version, ok := number(document["version"]); ok && version == 2 && object(document["eval"]) != nil && array(document["samples"]) != nil {
		out, err := normalizeInspect(document, name)
		return out, "inspect-ai-eval-log-json-v2", err
	}
	if object(document["metadata"]) != nil && array(document["outputs"]) != nil {
		out, err := normalizeVerifiers(document, name, source)
		return out, "prime-verifiers-generate-outputs", err
	}
	return nil, "", errors.New("unsupported trace: expected canonical NDJSON, Inspect AI EvalLog JSON, or Verifiers GenerateOutputs JSON")
}

func ParseCanonical(canonical []byte, name, format string, sourceSize int) (Collection, error) {
	if sourceSize < 0 || sourceSize > MaxRecommendedBytes {
		return Collection{}, fmt.Errorf("trace is %d bytes; browser maximum is %d bytes", sourceSize, MaxRecommendedBytes)
	}
	if len(canonical) > MaxRecommendedBytes {
		return Collection{}, fmt.Errorf("canonical trace is %d bytes; browser maximum is %d bytes", len(canonical), MaxRecommendedBytes)
	}
	runs := map[string]*model.Run{}
	cases := map[string]*model.Case{}
	groups := map[string]*model.Group{}
	trajectories := map[string]*model.Trajectory{}
	events := map[string][]model.Event{}
	signals := map[string][]model.Signal{}
	artifacts := map[string][]model.Artifact{}
	trajectoryOrder := make([]string, 0)
	err := model.Decode(bytes.NewReader(canonical), func(record *model.Record) error {
		switch value := record.Value.(type) {
		case *model.Run:
			runs[value.ID] = value
		case *model.Case:
			cases[value.ID] = value
		case *model.Group:
			groups[value.ID] = value
		case *model.Trajectory:
			if _, exists := trajectories[value.ID]; !exists {
				trajectoryOrder = append(trajectoryOrder, value.ID)
			}
			trajectories[value.ID] = value
		case *model.Event:
			events[value.TrajectoryID] = append(events[value.TrajectoryID], *value)
		case *model.Signal:
			signals[value.TrajectoryID] = append(signals[value.TrajectoryID], *value)
		case *model.Artifact:
			artifacts[value.TrajectoryID] = append(artifacts[value.TrajectoryID], *value)
		}
		return nil
	})
	if err != nil {
		return Collection{}, err
	}
	sum := sha256.Sum256(canonical)
	source := Source{ID: "browser-" + hex.EncodeToString(sum[:8]), Name: filepath.Base(name), Format: format, Size: sourceSize, IndexState: "complete"}
	rows := make([]BrowseRow, 0, len(trajectories))
	data := make(map[string]TrajectoryData, len(trajectories))
	for _, id := range trajectoryOrder {
		trajectory := *trajectories[id]
		group := groups[trajectory.GroupID]
		var currentCase *model.Case
		var run *model.Run
		if group != nil {
			currentCase = cases[group.CaseID]
		}
		if currentCase != nil {
			run = runs[currentCase.RunID]
		}
		metrics := trajectoryMetrics(trajectory, events[id], signals[id])
		rows = append(rows, BrowseRow{SourceID: source.ID, SourceName: source.Name, RunName: nameOfRun(run), CaseName: nameOfCase(currentCase), GroupName: nameOfGroup(group), Trajectory: trajectory, Metrics: metrics})
		page := map[string]any{"count": len(events[id]), "total": len(events[id]), "limit": len(events[id]), "has_more": false}
		data[id] = TrajectoryData{Trajectory: trajectory, Events: nonnil(events[id]), Signals: nonnil(signals[id]), Artifacts: nonnil(artifacts[id]), Run: run, Case: currentCase, Group: group, Source: source, Page: page}
	}
	browse := map[string]any{"sources": []Source{source}, "trajectories": rows, "count": len(rows)}
	return Collection{Source: source, Browse: browse, Trajectories: data}, nil
}

func Analysis(collection Collection, trajectoryID string) (map[string]any, error) {
	item, ok := collection.Trajectories[trajectoryID]
	if !ok {
		return nil, fmt.Errorf("unknown trajectory %q", trajectoryID)
	}
	output, err := (analyzers.LoopRetry{}).Analyze(context.Background(), analyzers.Input{APIVersion: analyzers.APIVersion, Operation: analyzers.OperationAnalyze, TrajectoryID: trajectoryID, Events: item.Events, Signals: item.Signals})
	if err != nil {
		return nil, err
	}
	return map[string]any{"analysis": output, "cached": false, "analyzed_at": "in-browser"}, nil
}

func Compare(collection Collection, leftID, rightID string) (map[string]any, error) {
	left, lok := collection.Trajectories[leftID]
	right, rok := collection.Trajectories[rightID]
	if !lok || !rok {
		return nil, errors.New("comparison trajectory not found")
	}
	result, complexity, err := alignment.AlignBounded(left.Events, right.Events, alignment.MaxComparisonWork, alignment.MaxComparisonWorkspace)
	if err != nil {
		if errors.Is(err, alignment.ErrTooLarge) {
			return nil, &CodedError{Code: "comparison_too_large", Message: fmt.Sprintf("comparison divergent middle %dx%d requires %d alignment cells and %d workspace bytes; maximums are %d and %d", complexity.MiddleLeft, complexity.MiddleRight, complexity.WorkCells, complexity.WorkspaceBytes, alignment.MaxComparisonWork, alignment.MaxComparisonWorkspace)}
		}
		return nil, err
	}
	return map[string]any{
		"source":      collection.Source,
		"left":        comparisonSide(left),
		"right":       comparisonSide(right),
		"alignment":   result,
		"differences": comparisonDifferences(left, right),
	}, nil
}

func comparisonSide(side TrajectoryData) map[string]any {
	context := map[string]any{
		"run":        map[string]any{"value": side.Run},
		"case":       map[string]any{"value": side.Case},
		"group":      map[string]any{"value": side.Group},
		"trajectory": map[string]any{"value": side.Trajectory},
	}
	return map[string]any{
		"context": context, "run": side.Run, "case": side.Case, "group": side.Group,
		"trajectory": side.Trajectory, "events": side.Events, "event_provenance": []any{},
		"signals": side.Signals, "artifacts": side.Artifacts,
	}
}

func comparisonDifferences(left, right TrajectoryData) map[string]any {
	leftReward, leftRewardOK := signalValue(left.Signals, "reward")
	rightReward, rightRewardOK := signalValue(right.Signals, "reward")
	leftSuccess, leftSuccessOK := boolSignalValue(left.Signals, "pass", "success")
	rightSuccess, rightSuccessOK := boolSignalValue(right.Signals, "pass", "success")
	leftTokens, leftTokensOK := integerSignalValue(left.Signals, "token_count", "total_tokens", "tokens")
	rightTokens, rightTokensOK := integerSignalValue(right.Signals, "token_count", "total_tokens", "tokens")
	leftContext, leftCompactions := contextEventCounts(left.Events)
	rightContext, rightCompactions := contextEventCounts(right.Events)
	leftVerifiers, rightVerifiers := verifierResults(left.Events), verifierResults(right.Events)
	return map[string]any{
		"event_count":         countDifference(len(left.Events), len(right.Events)),
		"status":              valueDifference(left.Trajectory.Status, right.Trajectory.Status),
		"termination":         valueDifference(left.Trajectory.Termination, right.Trajectory.Termination),
		"reward":              optionalValueDifference(leftReward, leftRewardOK, rightReward, rightRewardOK),
		"success":             optionalValueDifference(leftSuccess, leftSuccessOK, rightSuccess, rightSuccessOK),
		"token_count":         optionalIntegerDifference(leftTokens, leftTokensOK, rightTokens, rightTokensOK),
		"context_event_count": countDifference(leftContext, rightContext),
		"compaction_count":    countDifference(leftCompactions, rightCompactions),
		"verifier_results":    map[string]any{"left": leftVerifiers, "right": rightVerifiers, "changed": !reflect.DeepEqual(verifierComparable(leftVerifiers), verifierComparable(rightVerifiers))},
	}
}

func trajectoryMetrics(t model.Trajectory, events []model.Event, signals []model.Signal) map[string]any {
	metrics := map[string]any{"trajectory": t, "event_count": len(events), "error_count": 0, "status": t.Status, "termination": t.Termination}
	for _, event := range events {
		if event.Kind == "error" {
			metrics["error_count"] = metrics["error_count"].(int) + 1
		}
	}
	for _, signal := range signals {
		if signal.Name == "reward" {
			metrics["reward"] = signal.Value
		}
		if signal.Name == "pass" || signal.Name == "success" {
			metrics[signal.Name] = signal.Value
		}
	}
	return metrics
}

func countDifference(left, right int) map[string]any {
	return map[string]any{"left": left, "right": right, "delta": right - left}
}
func valueDifference(left, right any) map[string]any {
	return map[string]any{"left": left, "right": right, "changed": !reflect.DeepEqual(left, right)}
}
func signalValue(signals []model.Signal, names ...string) (any, bool) {
	for _, name := range names {
		for _, signal := range signals {
			if strings.EqualFold(strings.TrimSpace(signal.Name), name) {
				return signal.Value, true
			}
		}
	}
	return nil, false
}
func boolSignalValue(signals []model.Signal, names ...string) (bool, bool) {
	value, ok := signalValue(signals, names...)
	result, valid := value.(bool)
	return result, ok && valid
}
func integerSignalValue(signals []model.Signal, names ...string) (int64, bool) {
	value, ok := signalValue(signals, names...)
	if !ok {
		return 0, false
	}
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(number.String(), 10, 64)
		return parsed, err == nil && parsed >= 0
	case float64:
		return int64(number), number >= 0 && number == math.Trunc(number) && number <= math.MaxInt64
	case int64:
		return number, number >= 0
	}
	return 0, false
}
func optionalValueDifference(left any, leftOK bool, right any, rightOK bool) map[string]any {
	result := map[string]any{"changed": leftOK != rightOK || (leftOK && !reflect.DeepEqual(left, right))}
	if leftOK {
		result["left"] = left
	}
	if rightOK {
		result["right"] = right
	}
	return result
}
func optionalIntegerDifference(left int64, leftOK bool, right int64, rightOK bool) map[string]any {
	result := map[string]any{"changed": leftOK != rightOK || (leftOK && left != right)}
	if leftOK {
		result["left"] = left
	}
	if rightOK {
		result["right"] = right
	}
	if leftOK && rightOK {
		result["delta"] = right - left
	}
	return result
}
func contextEventCounts(events []model.Event) (contextEvents, compactions int) {
	for _, event := range events {
		if event.Context != nil {
			contextEvents++
			if event.Context.Operation == "compaction" {
				compactions++
			}
			continue
		}
		if strings.HasPrefix(event.AlignmentKey, "context:") {
			contextEvents++
		}
		if event.AlignmentKey == "context:compaction" {
			compactions++
		}
	}
	return contextEvents, compactions
}
func verifierResults(events []model.Event) []map[string]any {
	results := make([]map[string]any, 0)
	for _, event := range events {
		if event.Kind != "grader" {
			continue
		}
		result := map[string]any{"event_id": event.ID, "sequence": event.Sequence}
		if event.AlignmentKey != "" {
			result["alignment_key"] = event.AlignmentKey
		}
		if event.Output != nil {
			result["output"] = event.Output
		}
		results = append(results, result)
	}
	return results
}
func verifierComparable(results []map[string]any) []map[string]any {
	comparable := make([]map[string]any, len(results))
	for index, result := range results {
		comparable[index] = map[string]any{"alignment_key": result["alignment_key"], "output": result["output"]}
	}
	return comparable
}
func reward(signals []model.Signal) any {
	for _, signal := range signals {
		if signal.Name == "reward" {
			return signal.Value
		}
	}
	return nil
}
func nameOfRun(v *model.Run) string {
	if v == nil {
		return ""
	}
	return v.Name
}
func nameOfCase(v *model.Case) string {
	if v == nil {
		return ""
	}
	return v.Name
}
func nameOfGroup(v *model.Group) string {
	if v == nil {
		return ""
	}
	return v.Name
}
func nonnil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
func firstLine(data []byte) []byte {
	if index := bytes.IndexByte(data, '\n'); index >= 0 {
		return data[:index]
	}
	return data
}
func object(value any) map[string]any { result, _ := value.(map[string]any); return result }
func array(value any) []any           { result, _ := value.([]any); return result }
func text(value any) string {
	if value == nil {
		return ""
	}
	if result, ok := value.(string); ok {
		return result
	}
	data, _ := json.Marshal(value)
	return string(data)
}
func number(value any) (float64, bool) {
	result, ok := value.(float64)
	return result, ok && !math.IsNaN(result) && !math.IsInf(result, 0)
}

func stableID(prefix string, parts ...any) string {
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	return prefix + "-" + hex.EncodeToString(sum[:12])
}

func appendRecord(buffer *bytes.Buffer, record any) {
	data, _ := json.Marshal(record)
	buffer.Write(data)
	buffer.WriteByte('\n')
}

func normalizeInspect(document map[string]any, name string) ([]byte, error) {
	eval := object(document["eval"])
	samples := array(document["samples"])
	runID := stableID("inspect-run", eval["task_id"], eval["created"])
	var out bytes.Buffer
	records := int64(0)
	emit := func(record any) { appendRecord(&out, record); records++ }
	emit(model.Run{RecordType: model.RecordRun, ID: runID, Name: text(eval["task"]), Metadata: model.Metadata{"adapter": "inspect-ai", "model": eval["model"]}})
	groups := map[string]string{}
	for sampleIndex, rawSample := range samples {
		sample := object(rawSample)
		if sample == nil {
			return nil, fmt.Errorf("sample %d must be an object", sampleIndex)
		}
		key := text(sample["id"])
		if key == "" {
			return nil, fmt.Errorf("sample %d requires id", sampleIndex)
		}
		groupID := groups[key]
		if groupID == "" {
			caseID := stableID("inspect-case", runID, sample["id"])
			groupID = stableID("inspect-group", runID, sample["id"])
			groups[key] = groupID
			emit(model.Case{RecordType: model.RecordCase, ID: caseID, RunID: runID, Name: key, Input: sample["input"], Metadata: model.Metadata{"target": sample["target"]}})
			emit(model.Group{RecordType: model.RecordGroup, ID: groupID, CaseID: caseID, Name: "Inspect sample " + key})
		}
		trajectoryID := stableID("inspect-trajectory", runID, sample["uuid"], sampleIndex)
		status := "completed"
		if sample["error"] != nil {
			status = "failed"
		}
		termination := ""
		if output := object(sample["output"]); output != nil {
			termination = text(output["stop_reason"])
			if termination == "" {
				termination = text(output["finish_reason"])
			}
		}
		emit(model.Trajectory{RecordType: model.RecordTrajectory, ID: trajectoryID, GroupID: groupID, Status: status, Termination: termination, Metadata: model.Metadata{"sample_id": sample["id"], "epoch": sample["epoch"]}})
		parent := ""
		scoreEvents := map[string]string{}
		for sequence, rawEvent := range array(sample["events"]) {
			event := object(rawEvent)
			if event == nil {
				return nil, fmt.Errorf("sample event %d must be an object", sequence)
			}
			eventType := text(event["event"])
			eventID := stableID("inspect-event", trajectoryID, event["uuid"], sequence)
			canonical := model.Event{RecordType: model.RecordEvent, ID: eventID, TrajectoryID: trajectoryID, Sequence: int64(sequence), Kind: "log", ParentID: parent, Timestamp: text(event["timestamp"]), Source: &model.SourceLocation{Path: name}, Metadata: model.Metadata{"source_format": "inspect-ai-eval-log-json-v2", "inspect_event": eventType}}
			raw, _ := json.Marshal(event)
			canonical.Raw = raw
			switch eventType {
			case "model":
				canonical.Kind = "generation"
				canonical.Input = event["input"]
				canonical.Output = event["output"]
			case "tool":
				canonical.Kind = "tool"
				canonical.AlignmentKey = "tool:" + text(event["function"])
				canonical.Input = map[string]any{"id": event["id"], "name": event["function"], "arguments": event["arguments"]}
				canonical.Output = map[string]any{"result": event["result"], "error": event["error"], "truncated": event["truncated"]}
			case "compaction":
				canonical.Kind = "state"
				operation := "compaction"
				if text(event["type"]) == "trim" {
					operation = "truncation"
				}
				canonical.AlignmentKey = "context:" + operation
				canonical.Data = event
				canonical.Context = &model.Context{Operation: operation, Provenance: "source_native"}
				if value, ok := number(event["tokens_after"]); ok {
					n := int64(value)
					canonical.Context.InputTokens = &n
				}
				if value, ok := number(event["tokens_before"]); ok {
					n := int64(value)
					canonical.Context.InputTokensBefore = &n
				}
			case "score":
				canonical.Kind = "grader"
				canonical.AlignmentKey = "grader:" + text(event["scorer"])
				canonical.Input = map[string]any{"target": event["target"]}
				canonical.Output = event["score"]
				scoreEvents[text(event["scorer"])] = eventID
			case "error", "sample_limit":
				canonical.Kind = "error"
				canonical.Data = event
			case "message":
				canonical.Kind = "message"
				canonical.Data = event
			case "state", "store":
				canonical.Kind = "state"
				canonical.Data = event
			default:
				canonical.Data = event
			}
			emit(canonical)
			parent = eventID
		}
		scoreNames := make([]string, 0, len(object(sample["scores"])))
		for scorer := range object(sample["scores"]) {
			scoreNames = append(scoreNames, scorer)
		}
		sort.Strings(scoreNames)
		for _, scorer := range scoreNames {
			rawScore := object(sample["scores"])[scorer]
			value := rawScore
			if score := object(rawScore); score != nil {
				value = score["value"]
			}
			switch value.(type) {
			case bool, string, float64:
				emit(model.Signal{RecordType: model.RecordSignal, ID: stableID("inspect-signal", trajectoryID, scorer), TrajectoryID: trajectoryID, EventID: scoreEvents[scorer], Name: scorer, Value: value})
			}
		}
	}
	appendRecord(&out, model.Complete{RecordType: model.RecordComplete, Records: records, Warnings: 0})
	return out.Bytes(), nil
}

func normalizeVerifiers(document map[string]any, name string, raw []byte) ([]byte, error) {
	metadata := object(document["metadata"])
	outputs := array(document["outputs"])
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:8])
	runID := "run-" + digest
	var out bytes.Buffer
	records := int64(0)
	emit := func(record any) { appendRecord(&out, record); records++ }
	emit(model.Run{RecordType: model.RecordRun, ID: runID, Name: text(metadata["env_id"]), StartedAt: text(metadata["date"]), Metadata: model.Metadata{"adapter": "verifiers-generate", "model": metadata["model"], "rollouts_per_example": metadata["rollouts_per_example"]}})
	seen := map[string]bool{}
	for outputIndex, rawOutput := range outputs {
		output := object(rawOutput)
		if output == nil {
			return nil, fmt.Errorf("output %d must be an object", outputIndex)
		}
		caseKey := text(output["example_id"])
		if caseKey == "" {
			caseKey = fmt.Sprint(outputIndex)
		}
		caseID := "case-" + digest + "-" + caseKey
		groupID := "group-" + digest + "-" + caseKey
		if !seen[caseKey] {
			seen[caseKey] = true
			emit(model.Case{RecordType: model.RecordCase, ID: caseID, RunID: runID, Name: text(output["task"]), Input: output["prompt"], Metadata: model.Metadata{"answer": output["answer"], "info": output["info"]}})
			emit(model.Group{RecordType: model.RecordGroup, ID: groupID, CaseID: caseID})
		}
		trajectoryID := text(output["trajectory_id"])
		if trajectoryID == "" {
			trajectoryID = fmt.Sprintf("trajectory-%s-%d", digest, outputIndex)
		}
		status := "incomplete"
		if completed, _ := output["is_completed"].(bool); completed {
			status = "completed"
		}
		emit(model.Trajectory{RecordType: model.RecordTrajectory, ID: trajectoryID, GroupID: groupID, Status: status, Termination: text(output["stop_condition"]), Metadata: model.Metadata{"is_truncated": output["is_truncated"], "timing": output["timing"]}})
		parent := ""
		sequence := int64(0)
		for stepIndex, rawStep := range array(output["trajectory"]) {
			step := object(rawStep)
			eventID := fmt.Sprintf("event-%s-%d-%d", digest, outputIndex, stepIndex)
			promptTokens := maskedCount(object(step["tokens"])["prompt_mask"])
			event := model.Event{RecordType: model.RecordEvent, ID: eventID, TrajectoryID: trajectoryID, Sequence: sequence, Kind: "generation", ParentID: parent, AlignmentKey: fmt.Sprintf("generation:%d", stepIndex), Input: step["prompt"], Output: step["completion"], Data: map[string]any{"tokens": step["tokens"], "reward": step["reward"], "advantage": step["advantage"], "extras": step["extras"]}, Source: &model.SourceLocation{Path: name}, Metadata: model.Metadata{"context_provenance": "adapter_derived_from_prompt_mask"}}
			rawRecord, _ := json.Marshal(step)
			event.Raw = rawRecord
			if promptTokens >= 0 {
				n := int64(promptTokens)
				event.Context = &model.Context{InputTokens: &n, Provenance: "adapter_derived", Derivation: "count of non-zero entries in TrajectoryStep.tokens.prompt_mask"}
			}
			emit(event)
			parent = eventID
			sequence++
		}
		rewardEventID := fmt.Sprintf("reward-%s-%d", digest, outputIndex)
		emit(model.Event{RecordType: model.RecordEvent, ID: rewardEventID, TrajectoryID: trajectoryID, Sequence: sequence, Kind: "reward", ParentID: parent, AlignmentKey: "reward:final", Data: map[string]any{"reward": output["reward"], "metrics": output["metrics"]}, Source: &model.SourceLocation{Path: name}})
		if value, ok := number(output["reward"]); ok {
			emit(model.Signal{RecordType: model.RecordSignal, ID: fmt.Sprintf("signal-reward-%s-%d", digest, outputIndex), TrajectoryID: trajectoryID, EventID: rewardEventID, Name: "reward", Value: value})
		}
		keys := make([]string, 0)
		for key := range object(output["metrics"]) {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := object(output["metrics"])[key]
			switch value.(type) {
			case bool, string, float64:
				emit(model.Signal{RecordType: model.RecordSignal, ID: stableID("signal", digest, outputIndex, key), TrajectoryID: trajectoryID, EventID: rewardEventID, Name: key, Value: value})
			}
		}
	}
	appendRecord(&out, model.Complete{RecordType: model.RecordComplete, Records: records, Warnings: 0})
	return out.Bytes(), nil
}

func maskedCount(value any) int {
	items := array(value)
	if items == nil {
		return -1
	}
	count := 0
	for _, item := range items {
		n, ok := number(item)
		if !ok {
			return -1
		}
		if n != 0 {
			count++
		}
	}
	return count
}

// DecodeAdapterResult validates canonical NDJSON returned by an uploaded module.
func DecodeAdapterResult(data []byte, sourceName string, sourceSize int) ([]byte, error) {
	collection, err := DecodeAdapterCollection(data, sourceName, sourceSize)
	if err != nil {
		return nil, err
	}
	return json.Marshal(collection)
}

// DecodeAdapterCollection validates canonical adapter output without an
// intermediate JSON round trip.
func DecodeAdapterCollection(data []byte, sourceName string, sourceSize int) (Collection, error) {
	return ParseCanonical(data, sourceName, "browser-wasm-adapter", sourceSize)
}

// DecodeCollection accepts a collection previously returned by Parse.
func DecodeCollection(data []byte) (Collection, error) {
	var result Collection
	err := json.Unmarshal(data, &result)
	return result, err
}
