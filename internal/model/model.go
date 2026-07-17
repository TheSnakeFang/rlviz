// Package model defines RLViz's versioned canonical rollout records.
package model

import "encoding/json"

const APIVersion = "rlviz.dev/v1alpha1"

type RecordType string

const (
	RecordRun        RecordType = "run"
	RecordCase       RecordType = "case"
	RecordGroup      RecordType = "group"
	RecordTrajectory RecordType = "trajectory"
	RecordEvent      RecordType = "event"
	RecordSignal     RecordType = "signal"
	RecordArtifact   RecordType = "artifact"
	RecordComplete   RecordType = "complete"
)

type Metadata map[string]any

type Run struct {
	RecordType RecordType `json:"record_type"`
	ID         string     `json:"id"`
	Name       string     `json:"name,omitempty"`
	StartedAt  string     `json:"started_at,omitempty"`
	Metadata   Metadata   `json:"metadata,omitempty"`
}

type Case struct {
	RecordType RecordType `json:"record_type"`
	ID         string     `json:"id"`
	RunID      string     `json:"run_id"`
	Name       string     `json:"name,omitempty"`
	Input      any        `json:"input,omitempty"`
	Metadata   Metadata   `json:"metadata,omitempty"`
}

type Group struct {
	RecordType RecordType `json:"record_type"`
	ID         string     `json:"id"`
	CaseID     string     `json:"case_id"`
	Name       string     `json:"name,omitempty"`
	Metadata   Metadata   `json:"metadata,omitempty"`
}

type Trajectory struct {
	RecordType  RecordType `json:"record_type"`
	ID          string     `json:"id"`
	GroupID     string     `json:"group_id"`
	ParentID    string     `json:"parent_id,omitempty"`
	BranchID    string     `json:"branch_id,omitempty"`
	Status      string     `json:"status,omitempty"`
	Termination string     `json:"termination,omitempty"`
	Metadata    Metadata   `json:"metadata,omitempty"`
}

type SourceLocation struct {
	Path       string `json:"path"`
	Line       *int64 `json:"line,omitempty"`
	ByteOffset *int64 `json:"byte_offset,omitempty"`
	ByteLength *int64 `json:"byte_length,omitempty"`
}

type Event struct {
	RecordType   RecordType      `json:"record_type"`
	ID           string          `json:"id"`
	TrajectoryID string          `json:"trajectory_id"`
	Sequence     int64           `json:"sequence"`
	Kind         string          `json:"kind"`
	Timestamp    string          `json:"timestamp,omitempty"`
	ParentID     string          `json:"parent_id,omitempty"`
	BranchID     string          `json:"branch_id,omitempty"`
	AlignmentKey string          `json:"alignment_key,omitempty"`
	StateHash    string          `json:"state_hash,omitempty"`
	Input        any             `json:"input,omitempty"`
	Output       any             `json:"output,omitempty"`
	Data         any             `json:"data,omitempty"`
	Source       *SourceLocation `json:"source,omitempty"`
	Raw          json.RawMessage `json:"raw,omitempty"`
	Metadata     Metadata        `json:"metadata,omitempty"`
}

type Signal struct {
	RecordType   RecordType `json:"record_type"`
	ID           string     `json:"id"`
	TrajectoryID string     `json:"trajectory_id"`
	EventID      string     `json:"event_id,omitempty"`
	Name         string     `json:"name"`
	Value        any        `json:"value"`
	Unit         string     `json:"unit,omitempty"`
	Metadata     Metadata   `json:"metadata,omitempty"`
}

type Artifact struct {
	RecordType   RecordType `json:"record_type"`
	ID           string     `json:"id"`
	TrajectoryID string     `json:"trajectory_id"`
	EventID      string     `json:"event_id,omitempty"`
	Name         string     `json:"name,omitempty"`
	MediaType    string     `json:"media_type"`
	Path         string     `json:"path,omitempty"`
	Text         string     `json:"text,omitempty"`
	JSON         any        `json:"json,omitempty"`
	SHA256       string     `json:"sha256,omitempty"`
	Metadata     Metadata   `json:"metadata,omitempty"`
}

type Complete struct {
	RecordType RecordType `json:"record_type"`
	Records    int64      `json:"records"`
	Warnings   int64      `json:"warnings"`
}

// Record contains one decoded NDJSON record and its original bytes.
type Record struct {
	Type       RecordType
	Value      any
	Raw        json.RawMessage
	Line       int64
	ByteOffset int64
	ByteLength int64
}
