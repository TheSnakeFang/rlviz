package index

import (
	"encoding/json"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/analyzers"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

type AnalysisResult struct {
	Output     analyzers.Output `json:"analysis"`
	Cached     bool             `json:"cached"`
	AnalyzedAt time.Time        `json:"analyzed_at"`
}

type Source struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	Adapter     string    `json:"adapter,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
}

type SourceInfo struct {
	Source
	IndexedAt   time.Time       `json:"indexed_at"`
	Records     int64           `json:"records"`
	Warnings    int64           `json:"warnings"`
	CompleteRaw json.RawMessage `json:"complete_raw"`
	IndexState  IndexState      `json:"index_state"`
	IndexError  string          `json:"index_error,omitempty"`
}

type IndexState string

const (
	Indexing        IndexState = "indexing"
	IndexComplete   IndexState = "complete"
	IndexRefreshing IndexState = "refreshing"
	IndexFailed     IndexState = "failed"
)

type CacheState string

const (
	CacheMissing CacheState = "missing"
	CacheFresh   CacheState = "fresh"
	CacheStale   CacheState = "stale"
)

type SourceStatus struct {
	State  CacheState  `json:"state"`
	Cached *SourceInfo `json:"cached,omitempty"`
}

type IndexedRecord[T any] struct {
	Value      T               `json:"value"`
	Raw        json.RawMessage `json:"raw"`
	Line       int64           `json:"line"`
	ByteOffset int64           `json:"byte_offset"`
	ByteLength int64           `json:"byte_length"`
}

type TrajectoryContext struct {
	Run        IndexedRecord[*model.Run]        `json:"run"`
	Case       IndexedRecord[*model.Case]       `json:"case"`
	Group      IndexedRecord[*model.Group]      `json:"group"`
	Trajectory IndexedRecord[*model.Trajectory] `json:"trajectory"`
}

type EventQuery struct {
	SourceID      string
	TrajectoryID  string
	AfterSequence *int64
	Limit         int
	Kinds         []string
	Query         string
	ContextOnly   *bool
}

type EventPage struct {
	Events       []IndexedRecord[*model.Event] `json:"events"`
	NextSequence *int64                        `json:"next_sequence,omitempty"`
	Total        int64                         `json:"total"`
	RawBytes     int64                         `json:"-"`
}

// RecordPage is a bounded window over an indexed child collection. Total is
// computed independently so callers can reject truncation when completeness is
// required without first materializing every row.
type RecordPage[T any] struct {
	Items    []IndexedRecord[T]
	Total    int64
	RawBytes int64
	Offset   int64
}

type SummaryPage struct {
	Items    []TrajectorySummary
	Total    int64
	RawBytes int64
}

type TrajectorySummary struct {
	Trajectory IndexedRecord[*model.Trajectory] `json:"trajectory"`
	RunName    string                           `json:"-"`
	CaseName   string                           `json:"-"`
	GroupName  string                           `json:"-"`
	// Signals preserves each canonical signal value as its original JSON. If a
	// trajectory repeats a name, the later canonical record wins.
	Signals       map[string]json.RawMessage `json:"signals,omitempty"`
	Reward        *float64                   `json:"reward,omitempty"`
	Success       *bool                      `json:"success,omitempty"`
	TokenCount    *int64                     `json:"token_count,omitempty"`
	CostUSD       *float64                   `json:"cost_usd,omitempty"`
	LatencyMS     *float64                   `json:"latency_ms,omitempty"`
	Status        string                     `json:"status,omitempty"`
	Termination   string                     `json:"termination,omitempty"`
	EventCount    int64                      `json:"event_count"`
	ErrorCount    int64                      `json:"error_count"`
	ToolCallCount int64                      `json:"tool_call_count"`
	FirstSequence *int64                     `json:"first_sequence,omitempty"`
	LastSequence  *int64                     `json:"last_sequence,omitempty"`
	SignalCount   int64                      `json:"signal_count"`
	ArtifactCount int64                      `json:"artifact_count"`
	signalUnits   map[string]string
}

// RolloutQuery is the bounded, server-side collection query contract. Every
// filter is exact except Query, which is a case-insensitive substring search
// over source-backed identifiers, names, status, and termination.
type RolloutQuery struct {
	Offset             int64
	Limit              int
	Query              string
	SourceID           string
	Run                string
	Case               string
	Group              string
	Checkpoint         string
	Model              string
	EnvironmentVersion string
	Status             string
	Termination        string
	Tool               string
	Pass               *bool
	RewardMin          *float64
	RewardMax          *float64
	TokensMin          *int64
	TokensMax          *int64
	CostMin            *float64
	CostMax            *float64
	Sort               string
	Descending         bool
}

type RolloutSummary struct {
	SourceID           string            `json:"source_id"`
	SourceName         string            `json:"source_name"`
	RunID              string            `json:"run_id"`
	RunName            string            `json:"run_name,omitempty"`
	CaseID             string            `json:"case_id"`
	CaseName           string            `json:"case_name,omitempty"`
	GroupID            string            `json:"group_id"`
	GroupName          string            `json:"group_name,omitempty"`
	Checkpoint         string            `json:"checkpoint,omitempty"`
	Model              string            `json:"model,omitempty"`
	EnvironmentVersion string            `json:"environment_version,omitempty"`
	Summary            TrajectorySummary `json:"summary"`
}

type RolloutQueryAggregates struct {
	Count         int64         `json:"count"`
	Success       int64         `json:"success"`
	Failure       int64         `json:"failure"`
	Unknown       int64         `json:"unknown"`
	Reward        *NumericRange `json:"reward,omitempty"`
	TokenCount    *IntegerRange `json:"token_count,omitempty"`
	CostUSD       *NumericRange `json:"cost_usd,omitempty"`
	TotalCostUSD  *float64      `json:"total_cost_usd,omitempty"`
	ToolCallCount *IntegerRange `json:"tool_call_count,omitempty"`
}

type RolloutQueryPage struct {
	Items      []RolloutSummary       `json:"items"`
	Total      int64                  `json:"total"`
	Offset     int64                  `json:"offset"`
	Limit      int                    `json:"limit"`
	Aggregates RolloutQueryAggregates `json:"aggregates"`
}

type NumericRange struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Mean float64 `json:"mean"`
}

type IntegerRange struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

type GroupAggregates struct {
	Count         int           `json:"count"`
	Success       int           `json:"success"`
	Failure       int           `json:"failure"`
	Unknown       int           `json:"unknown"`
	Reward        *NumericRange `json:"reward,omitempty"`
	EventCount    *IntegerRange `json:"event_count,omitempty"`
	TokenCount    *IntegerRange `json:"token_count,omitempty"`
	CostUSD       *NumericRange `json:"cost_usd,omitempty"`
	TotalCostUSD  *float64      `json:"total_cost_usd,omitempty"`
	LatencyMS     *NumericRange `json:"latency_ms,omitempty"`
	ErrorCount    *IntegerRange `json:"error_count,omitempty"`
	ToolCallCount *IntegerRange `json:"tool_call_count,omitempty"`
}

type RawRecord struct {
	Ordinal    int64            `json:"ordinal"`
	Type       model.RecordType `json:"record_type"`
	ID         string           `json:"id,omitempty"`
	Raw        json.RawMessage  `json:"raw"`
	ByteOffset int64            `json:"byte_offset"`
	ByteLength int64            `json:"byte_length"`
}
