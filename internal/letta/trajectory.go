// Package letta maps Letta's public trajectory v1 format to RLViz canonical
// records. The input is the normalized record array, not a native harness log.
package letta

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

const Format = "letta-trajectory-v1-json"

var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})$`)

type metaRecord struct {
	Role      string `json:"role"`
	Source    string `json:"source"`
	CWD       string `json:"cwd,omitempty"`
	GitBranch string `json:"git_branch,omitempty"`
	Model     string `json:"model,omitempty"`
}

type toolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

type record struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content,omitempty"`
	Timestamp  string     `json:"timestamp,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Probe recognizes a trajectory v1 array from its leading meta record without
// reading the remaining session.
func Probe(reader io.Reader) (bool, string, error) {
	decoder := json.NewDecoder(reader)
	start, err := decoder.Token()
	if err != nil {
		return false, "", err
	}
	if start != json.Delim('[') || !decoder.More() {
		return false, "", nil
	}
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return false, "", err
	}
	var meta metaRecord
	if err := strictDecode(raw, &meta); err != nil {
		return false, "", nil
	}
	if meta.Role != "meta" || strings.TrimSpace(meta.Source) == "" {
		return false, "", nil
	}
	return true, meta.Source, nil
}

// NormalizeBytes validates and normalizes one complete trajectory v1 array.
func NormalizeBytes(source []byte, sourceName string) ([]byte, error) {
	return Normalize(bytes.NewReader(source), sourceName)
}

// Normalize streams a trajectory v1 record array into canonical NDJSON.
func Normalize(reader io.Reader, sourceName string) ([]byte, error) {
	decoder := json.NewDecoder(reader)
	start, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode trajectory v1 JSON: %w", err)
	}
	if start != json.Delim('[') {
		return nil, errors.New("unsupported trajectory document: expected a trajectory v1 record array")
	}
	if !decoder.More() {
		return nil, errors.New("trajectory v1 requires a leading meta record")
	}

	var metaRaw json.RawMessage
	if err := decoder.Decode(&metaRaw); err != nil {
		return nil, fmt.Errorf("decode trajectory v1 meta record: %w", err)
	}
	var metaHeader struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(metaRaw, &metaHeader); err != nil || metaHeader.Role != "meta" {
		return nil, errors.New("trajectory v1 requires a leading meta record")
	}
	var meta metaRecord
	if err := strictDecode(metaRaw, &meta); err != nil {
		return nil, fmt.Errorf("invalid trajectory v1 meta record: %w", err)
	}
	if meta.Role != "meta" || strings.TrimSpace(meta.Source) == "" {
		return nil, errors.New("trajectory v1 first record requires role meta and a nonempty source")
	}

	key := sourceName + "\x00" + meta.Source + "\x00" + meta.CWD + "\x00" + meta.GitBranch + "\x00" + meta.Model
	runID := stableID("letta-run", key)
	caseID := stableID("letta-case", runID)
	groupID := stableID("letta-group", caseID)
	trajectoryID := stableID("letta-trajectory", groupID)

	var out bytes.Buffer
	count := int64(0)
	emit := func(value any) error {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return marshalErr
		}
		out.Write(encoded)
		out.WriteByte('\n')
		count++
		return nil
	}
	metadata := model.Metadata{"source_format": "trajectory-v1", "harness": meta.Source}
	if meta.CWD != "" {
		metadata["cwd"] = meta.CWD
	}
	if meta.GitBranch != "" {
		metadata["git_branch"] = meta.GitBranch
	}
	if meta.Model != "" {
		metadata["model"] = meta.Model
	}
	if err := emit(model.Run{RecordType: model.RecordRun, ID: runID, Name: meta.Source, Metadata: metadata}); err != nil {
		return nil, err
	}
	if err := emit(model.Case{RecordType: model.RecordCase, ID: caseID, RunID: runID, Name: sourceName}); err != nil {
		return nil, err
	}
	if err := emit(model.Group{RecordType: model.RecordGroup, ID: groupID, CaseID: caseID, Name: "Trajectory session"}); err != nil {
		return nil, err
	}
	if err := emit(model.Trajectory{RecordType: model.RecordTrajectory, ID: trajectoryID, GroupID: groupID, Metadata: metadata}); err != nil {
		return nil, err
	}

	sequence := int64(0)
	recordIndex := 1
	type pendingCall struct{ eventID, name string }
	pending := map[string][]pendingCall{}
	for decoder.More() {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode trajectory v1 record %d: %w", recordIndex, err)
		}
		var header struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			return nil, fmt.Errorf("decode trajectory v1 record %d: %w", recordIndex, err)
		}
		var item record
		if err := strictDecode(raw, &item); err != nil {
			return nil, fmt.Errorf("invalid trajectory v1 record %d: %w", recordIndex, err)
		}
		if err := validateRequiredFields(raw, item, recordIndex); err != nil {
			return nil, err
		}
		if err := validateRecord(item, recordIndex); err != nil {
			return nil, err
		}
		source := &model.SourceLocation{Path: sourceName}
		switch header.Role {
		case "user":
			eventID := stableID("letta-event", trajectoryID, recordIndex, "user")
			if err := emit(model.Event{RecordType: model.RecordEvent, ID: eventID, TrajectoryID: trajectoryID, Sequence: sequence, Kind: "message", Timestamp: item.Timestamp, AlignmentKey: "message:user", Input: map[string]any{"role": "user", "content": deref(item.Content)}, Source: source, Raw: raw, Metadata: model.Metadata{"trajectory_role": "user", "title": "User message"}}); err != nil {
				return nil, err
			}
			sequence++
		case "reasoning":
			eventID := stableID("letta-event", trajectoryID, recordIndex, "reasoning")
			if err := emit(model.Event{RecordType: model.RecordEvent, ID: eventID, TrajectoryID: trajectoryID, Sequence: sequence, Kind: "generation", Timestamp: item.Timestamp, AlignmentKey: "generation:reasoning", Output: map[string]any{"role": "assistant", "content": deref(item.Content)}, Data: map[string]any{"trajectory_role": "reasoning"}, Source: source, Raw: raw, Metadata: model.Metadata{"trajectory_role": "reasoning", "title": "Reasoning"}}); err != nil {
				return nil, err
			}
			sequence++
		case "assistant":
			if len(item.ToolCalls) == 0 {
				eventID := stableID("letta-event", trajectoryID, recordIndex, "assistant")
				if err := emit(model.Event{RecordType: model.RecordEvent, ID: eventID, TrajectoryID: trajectoryID, Sequence: sequence, Kind: "generation", Timestamp: item.Timestamp, AlignmentKey: "message:assistant", Output: map[string]any{"role": "assistant", "content": deref(item.Content)}, Source: source, Raw: raw, Metadata: model.Metadata{"trajectory_role": "assistant", "title": "Assistant message"}}); err != nil {
					return nil, err
				}
				sequence++
				break
			}
			for callIndex, call := range item.ToolCalls {
				eventID := stableID("letta-event", trajectoryID, recordIndex, "tool", callIndex, call.ID)
				arguments := any(call.Args)
				var parsed any
				if json.Unmarshal([]byte(call.Args), &parsed) == nil {
					arguments = parsed
				}
				event := model.Event{RecordType: model.RecordEvent, ID: eventID, TrajectoryID: trajectoryID, Sequence: sequence, Kind: "tool", Timestamp: item.Timestamp, AlignmentKey: "tool:" + call.Name, Input: map[string]any{"id": call.ID, "name": call.Name, "arguments": arguments}, Source: source, Raw: raw, Metadata: model.Metadata{"trajectory_role": "assistant", "tool_call_id": call.ID, "title": call.Name}}
				if err := emit(event); err != nil {
					return nil, err
				}
				pending[call.ID] = append(pending[call.ID], pendingCall{eventID: eventID, name: call.Name})
				sequence++
			}
		case "tool":
			parentID, toolName := "", "tool"
			if calls := pending[item.ToolCallID]; len(calls) > 0 {
				parentID, toolName = calls[0].eventID, calls[0].name
				pending[item.ToolCallID] = calls[1:]
			}
			eventID := stableID("letta-event", trajectoryID, recordIndex, "tool-result", item.ToolCallID)
			if err := emit(model.Event{RecordType: model.RecordEvent, ID: eventID, TrajectoryID: trajectoryID, Sequence: sequence, Kind: "observation", Timestamp: item.Timestamp, ParentID: parentID, AlignmentKey: "observation:" + toolName, Output: deref(item.Content), Data: map[string]any{"tool_call_id": item.ToolCallID}, Source: source, Raw: raw, Metadata: model.Metadata{"trajectory_role": "tool", "tool_call_id": item.ToolCallID, "tool_name": toolName, "title": toolName + " result"}}); err != nil {
				return nil, err
			}
			sequence++
		default:
			return nil, fmt.Errorf("trajectory v1 record %d has unsupported role %q", recordIndex, header.Role)
		}
		recordIndex++
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim(']') {
		return nil, errors.New("trajectory v1 array is not complete")
	}
	if decoder.More() {
		return nil, errors.New("trajectory v1 contains data after the record array")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("trajectory v1 contains data after the record array")
	}
	if sequence == 0 {
		return nil, errors.New("trajectory v1 requires at least one conversational record")
	}
	if err := emit(model.Complete{RecordType: model.RecordComplete, Records: count, Warnings: 0}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func validateRecord(item record, index int) error {
	if item.Role == "meta" {
		return fmt.Errorf("trajectory v1 record %d has meta outside the leading position", index)
	}
	if item.Role != "user" && item.Role != "reasoning" && item.Role != "assistant" && item.Role != "tool" {
		return fmt.Errorf("trajectory v1 record %d has unsupported role %q", index, item.Role)
	}
	if !timestampPattern.MatchString(item.Timestamp) {
		return fmt.Errorf("trajectory v1 record %d has invalid timestamp %q", index, item.Timestamp)
	}
	switch item.Role {
	case "user", "reasoning":
		if item.Content == nil || len(item.ToolCalls) > 0 || item.ToolCallID != "" {
			return fmt.Errorf("trajectory v1 record %d has invalid %s fields", index, item.Role)
		}
	case "assistant":
		if item.ToolCallID != "" {
			return fmt.Errorf("trajectory v1 record %d assistant message cannot contain tool_call_id", index)
		}
		if len(item.ToolCalls) > 0 {
			if item.Content != nil {
				return fmt.Errorf("trajectory v1 record %d with tool_calls requires null content", index)
			}
			for callIndex, call := range item.ToolCalls {
				if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
					return fmt.Errorf("trajectory v1 record %d tool call %d requires id and name", index, callIndex)
				}
			}
		} else if item.Content == nil || *item.Content == "" {
			return fmt.Errorf("trajectory v1 record %d assistant message requires nonempty content", index)
		}
	case "tool":
		if strings.TrimSpace(item.ToolCallID) == "" || item.Content == nil || len(item.ToolCalls) > 0 {
			return fmt.Errorf("trajectory v1 record %d has invalid tool result fields", index)
		}
	}
	return nil
}

func validateRequiredFields(raw []byte, item record, index int) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("decode trajectory v1 record %d: %w", index, err)
	}
	required := []string{"role", "timestamp"}
	allowed := map[string]bool{"role": true, "timestamp": true}
	switch item.Role {
	case "user", "reasoning", "assistant":
		required = append(required, "content")
		allowed["content"] = true
		if item.Role == "assistant" {
			allowed["tool_calls"] = true
		}
	case "tool":
		required = append(required, "tool_call_id", "content")
		allowed["tool_call_id"] = true
		allowed["content"] = true
	}
	for _, field := range required {
		if _, ok := fields[field]; !ok {
			return fmt.Errorf("trajectory v1 record %d requires %s", index, field)
		}
	}
	for field := range fields {
		if !allowed[field] {
			return fmt.Errorf("trajectory v1 record %d role %s cannot contain %s", index, item.Role, field)
		}
	}
	if item.Role == "assistant" {
		_, hasToolCalls := fields["tool_calls"]
		if hasToolCalls && len(item.ToolCalls) == 0 {
			return fmt.Errorf("trajectory v1 record %d tool_calls must not be empty", index)
		}
		if !hasToolCalls {
			return nil
		}
		var calls []map[string]json.RawMessage
		if err := json.Unmarshal(fields["tool_calls"], &calls); err != nil {
			return fmt.Errorf("trajectory v1 record %d has invalid tool_calls", index)
		}
		for callIndex, call := range calls {
			for _, field := range []string{"id", "name", "args"} {
				if _, ok := call[field]; !ok {
					return fmt.Errorf("trajectory v1 record %d tool call %d requires %s", index, callIndex, field)
				}
			}
		}
	}
	return nil
}

func strictDecode(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stableID(parts ...any) string {
	hash := sha256.New()
	for _, part := range parts {
		fmt.Fprint(hash, part, "\x00")
	}
	return parts[0].(string) + "-" + hex.EncodeToString(hash.Sum(nil)[:12])
}
