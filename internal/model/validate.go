package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

var eventKinds = map[string]struct{}{
	"message": {}, "generation": {}, "tool": {}, "environment_action": {},
	"observation": {}, "state": {}, "reward": {}, "grader": {},
	"artifact": {}, "error": {}, "log": {},
}

var contextOperations = map[string]struct{}{
	"compaction": {}, "truncation": {}, "injection": {}, "restore": {},
}

type Validator struct {
	ids          map[string]RecordType
	runs         map[string]struct{}
	cases        map[string]string
	groups       map[string]string
	trajectories map[string]string
	events       map[string]string
	lastSequence map[string]int64
	records      int64
	complete     bool
}

func NewValidator() *Validator {
	return &Validator{
		ids: make(map[string]RecordType), runs: make(map[string]struct{}),
		cases: make(map[string]string), groups: make(map[string]string),
		trajectories: make(map[string]string), events: make(map[string]string),
		lastSequence: make(map[string]int64),
	}
}

func (v *Validator) Add(record *Record) error {
	if v.complete {
		return errors.New("complete must be the final record")
	}
	if record == nil || record.Value == nil {
		return errors.New("record is nil")
	}
	if record.Type != RecordComplete {
		v.records++
	}

	switch value := record.Value.(type) {
	case *Run:
		if err := v.addID(value.ID, RecordRun); err != nil {
			return err
		}
		if value.RecordType != RecordRun {
			return errors.New("record_type must be run")
		}
		v.runs[value.ID] = struct{}{}
	case *Case:
		if err := v.addID(value.ID, RecordCase); err != nil {
			return err
		}
		if value.RecordType != RecordCase {
			return errors.New("record_type must be case")
		}
		if !v.hasRun(value.RunID) {
			return validationField("run_id", fmt.Errorf("case %q references unknown or later run %q", value.ID, value.RunID))
		}
		v.cases[value.ID] = value.RunID
	case *Group:
		if err := v.addID(value.ID, RecordGroup); err != nil {
			return err
		}
		if value.RecordType != RecordGroup {
			return errors.New("record_type must be group")
		}
		if _, ok := v.cases[value.CaseID]; !ok {
			return validationField("case_id", fmt.Errorf("group %q references unknown or later case %q", value.ID, value.CaseID))
		}
		v.groups[value.ID] = value.CaseID
	case *Trajectory:
		if err := v.addID(value.ID, RecordTrajectory); err != nil {
			return err
		}
		if value.RecordType != RecordTrajectory {
			return errors.New("record_type must be trajectory")
		}
		if _, ok := v.groups[value.GroupID]; !ok {
			return validationField("group_id", fmt.Errorf("trajectory %q references unknown or later group %q", value.ID, value.GroupID))
		}
		if value.ParentID != "" {
			parentGroup, ok := v.trajectories[value.ParentID]
			if !ok {
				return validationField("parent_id", fmt.Errorf("trajectory %q references unknown or later parent %q", value.ID, value.ParentID))
			}
			if parentGroup != value.GroupID {
				return fmt.Errorf("trajectory %q parent %q belongs to another group", value.ID, value.ParentID)
			}
		}
		v.trajectories[value.ID] = value.GroupID
	case *Event:
		if err := v.addID(value.ID, RecordEvent); err != nil {
			return err
		}
		if value.RecordType != RecordEvent {
			return errors.New("record_type must be event")
		}
		if _, ok := v.trajectories[value.TrajectoryID]; !ok {
			return validationField("trajectory_id", fmt.Errorf("event %q references unknown or later trajectory %q", value.ID, value.TrajectoryID))
		}
		if value.Sequence < 0 {
			return validationField("sequence", fmt.Errorf("event %q sequence must be non-negative", value.ID))
		}
		if previous, ok := v.lastSequence[value.TrajectoryID]; ok && value.Sequence <= previous {
			return validationField("sequence", fmt.Errorf("event %q sequence %d is not greater than prior sequence %d", value.ID, value.Sequence, previous))
		}
		if _, ok := eventKinds[value.Kind]; !ok {
			return validationField("kind", fmt.Errorf("event %q has unsupported kind %q", value.ID, value.Kind))
		}
		if value.ParentID != "" {
			parentTrajectory, ok := v.events[value.ParentID]
			if !ok {
				return validationField("parent_id", fmt.Errorf("event %q references unknown or later parent %q", value.ID, value.ParentID))
			}
			if parentTrajectory != value.TrajectoryID {
				return fmt.Errorf("event %q parent %q belongs to another trajectory", value.ID, value.ParentID)
			}
		}
		if value.Source != nil {
			if value.Source.Path == "" {
				return validationField("source.path", fmt.Errorf("event %q source.path is required", value.ID))
			}
			if negative(value.Source.Line) {
				return validationField("source.line", fmt.Errorf("event %q source line must be non-negative", value.ID))
			}
			if negative(value.Source.ByteOffset) {
				return validationField("source.byte_offset", fmt.Errorf("event %q source byte offset must be non-negative", value.ID))
			}
			if negative(value.Source.ByteLength) {
				return validationField("source.byte_length", fmt.Errorf("event %q source byte length must be non-negative", value.ID))
			}
		}
		if value.Context != nil {
			if err := v.validateContext(value); err != nil {
				return err
			}
		}
		v.events[value.ID] = value.TrajectoryID
		v.lastSequence[value.TrajectoryID] = value.Sequence
	case *Signal:
		if err := v.addID(value.ID, RecordSignal); err != nil {
			return err
		}
		if value.RecordType != RecordSignal {
			return errors.New("record_type must be signal")
		}
		if _, ok := v.trajectories[value.TrajectoryID]; !ok {
			return validationField("trajectory_id", fmt.Errorf("signal %q references unknown or later trajectory %q", value.ID, value.TrajectoryID))
		}
		if value.Name == "" {
			return validationField("name", fmt.Errorf("signal %q name is required", value.ID))
		}
		if value.EventID != "" {
			t, ok := v.events[value.EventID]
			if !ok {
				return validationField("event_id", fmt.Errorf("signal %q references unknown or later event %q", value.ID, value.EventID))
			}
			if t != value.TrajectoryID {
				return fmt.Errorf("signal %q event belongs to another trajectory", value.ID)
			}
		}
		if err := validateSignalValue(value.Value); err != nil {
			return validationField("value", fmt.Errorf("signal %q value: %w", value.ID, err))
		}
	case *Artifact:
		if err := v.addID(value.ID, RecordArtifact); err != nil {
			return err
		}
		if value.RecordType != RecordArtifact {
			return errors.New("record_type must be artifact")
		}
		if _, ok := v.trajectories[value.TrajectoryID]; !ok {
			return validationField("trajectory_id", fmt.Errorf("artifact %q references unknown or later trajectory %q", value.ID, value.TrajectoryID))
		}
		if value.MediaType == "" {
			return validationField("media_type", fmt.Errorf("artifact %q media_type is required", value.ID))
		}
		if value.EventID != "" {
			t, ok := v.events[value.EventID]
			if !ok {
				return validationField("event_id", fmt.Errorf("artifact %q references unknown or later event %q", value.ID, value.EventID))
			}
			if t != value.TrajectoryID {
				return fmt.Errorf("artifact %q event belongs to another trajectory", value.ID)
			}
		}
		locations := 0
		if value.Path != "" {
			locations++
		}
		if value.Text != "" {
			locations++
		}
		if value.JSON != nil {
			locations++
		}
		if locations != 1 {
			return fmt.Errorf("artifact %q must provide exactly one of path, text, or json", value.ID)
		}
		if value.SHA256 != "" && (len(value.SHA256) != 64 || strings.Trim(value.SHA256, "0123456789abcdef") != "") {
			return validationField("sha256", fmt.Errorf("artifact %q sha256 must be 64 lowercase hex characters", value.ID))
		}
	case *Complete:
		if value.RecordType != RecordComplete {
			return errors.New("record_type must be complete")
		}
		if value.Records < 0 {
			return validationField("records", errors.New("complete records must be non-negative"))
		}
		if value.Warnings < 0 {
			return validationField("warnings", errors.New("complete warnings must be non-negative"))
		}
		if value.Records != v.records {
			return validationField("records", fmt.Errorf("complete reports %d records, decoded %d", value.Records, v.records))
		}
		v.complete = true
	default:
		return fmt.Errorf("unsupported decoded value %T", record.Value)
	}
	return nil
}

func (v *Validator) Finish() error {
	if !v.complete {
		return errors.New("stream ended without a complete record")
	}
	return nil
}

func (v *Validator) addID(id string, kind RecordType) error {
	if id == "" {
		return validationField("id", fmt.Errorf("%s id is required", kind))
	}
	if previous, ok := v.ids[id]; ok {
		return validationField("id", fmt.Errorf("duplicate id %q (already used by %s)", id, previous))
	}
	v.ids[id] = kind
	return nil
}

func validationField(field string, err error) error {
	return &FieldValidationError{Field: field, Err: err}
}

func (v *Validator) validateContext(event *Event) error {
	observation := event.Context
	if observation.Provenance != "source_native" && observation.Provenance != "adapter_derived" {
		return validationField("context.provenance", errors.New("context provenance must be source_native or adapter_derived"))
	}
	if observation.Provenance == "adapter_derived" && strings.TrimSpace(observation.Derivation) == "" {
		return validationField("context.derivation", errors.New("adapter-derived context requires derivation"))
	}
	if observation.Operation != "" {
		if _, ok := contextOperations[observation.Operation]; !ok {
			return validationField("context.operation", fmt.Errorf("unsupported context operation %q", observation.Operation))
		}
	} else if observation.InputTokensBefore != nil {
		return validationField("context.input_tokens_before", errors.New("context input_tokens_before requires an operation"))
	}
	if negative(observation.InputTokens) {
		return validationField("context.input_tokens", errors.New("context input_tokens must be non-negative"))
	}
	if negative(observation.InputTokensBefore) {
		return validationField("context.input_tokens_before", errors.New("context input_tokens_before must be non-negative"))
	}
	if observation.Capacity != nil && *observation.Capacity <= 0 {
		return validationField("context.capacity", errors.New("context capacity must be positive"))
	}
	if observation.Operation == "" && observation.InputTokens == nil && observation.Capacity == nil && len(observation.RetainedEventIDs) == 0 && len(observation.DroppedEventIDs) == 0 && len(observation.SummarizedEventIDs) == 0 && strings.TrimSpace(observation.Summary) == "" {
		return validationField("context", errors.New("context must contain an observation or lifecycle fact"))
	}
	seen := make(map[string]string)
	referenceGroups := []struct {
		field string
		ids   []string
	}{
		{field: "context.retained_event_ids", ids: observation.RetainedEventIDs},
		{field: "context.dropped_event_ids", ids: observation.DroppedEventIDs},
		{field: "context.summarized_event_ids", ids: observation.SummarizedEventIDs},
	}
	for _, group := range referenceGroups {
		for _, id := range group.ids {
			if strings.TrimSpace(id) == "" {
				return validationField(group.field, errors.New("context event references must be non-empty"))
			}
			if previous, ok := seen[id]; ok {
				return validationField(group.field, fmt.Errorf("context event %q is already listed in %s", id, previous))
			}
			trajectoryID, ok := v.events[id]
			if !ok {
				return validationField(group.field, fmt.Errorf("context references unknown or later event %q", id))
			}
			if trajectoryID != event.TrajectoryID {
				return validationField(group.field, fmt.Errorf("context event %q belongs to another trajectory", id))
			}
			seen[id] = group.field
		}
	}
	return nil
}

func (v *Validator) hasRun(id string) bool { _, ok := v.runs[id]; return ok }

func negative(value *int64) bool { return value != nil && *value < 0 }

func validateSignalValue(value any) error {
	switch v := value.(type) {
	case string, bool, json.Number:
		return nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("must be finite")
		}
		return nil
	case nil:
		return errors.New("is required")
	default:
		return fmt.Errorf("must be a number, string, or boolean, got %T", value)
	}
}
