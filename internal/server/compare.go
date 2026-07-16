package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/unlatch-ai/rolloutviz/internal/alignment"
	rolloutindex "github.com/unlatch-ai/rolloutviz/internal/index"
	"github.com/unlatch-ai/rolloutviz/internal/model"
)

const (
	MaxComparisonEvents             = 20_000
	MaxComparisonAlignmentWork      = 25_000_000
	MaxComparisonAlignmentWorkspace = 64 << 20
	MaxComparisonRawBytes           = 64 << 20
)

type comparisonSide struct {
	Context         rolloutindex.TrajectoryContext `json:"context"`
	Run             *model.Run                     `json:"run"`
	Case            *model.Case                    `json:"case"`
	Group           *model.Group                   `json:"group"`
	Trajectory      *model.Trajectory              `json:"trajectory"`
	Events          []*model.Event                 `json:"events"`
	EventProvenance []indexedProvenance            `json:"event_provenance"`
	Signals         []*model.Signal                `json:"signals"`
	Artifacts       []*model.Artifact              `json:"artifacts"`
}

type valueDifference struct {
	Left    any  `json:"left,omitempty"`
	Right   any  `json:"right,omitempty"`
	Changed bool `json:"changed"`
}

type countDifference struct {
	Left  int `json:"left"`
	Right int `json:"right"`
	Delta int `json:"delta"`
}

type comparisonDifferences struct {
	EventCount  countDifference `json:"event_count"`
	Status      valueDifference `json:"status"`
	Termination valueDifference `json:"termination"`
	Reward      valueDifference `json:"reward"`
}

func (api *indexedAPI) compare(response http.ResponseWriter, request *http.Request) {
	values := request.URL.Query()
	if err := validateComparisonQuery(values); err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	sourceID, _ := requiredSingle(values, "trajectory")
	leftID, _ := requiredSingle(values, "left")
	rightID, _ := requiredSingle(values, "right")
	if leftID == rightID {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", errors.New("left and right must identify different trajectories"))
		return
	}

	source, err := api.reader.Source(request.Context(), sourceID)
	if err != nil {
		api.writeReadError(response, "source_not_found", err)
		return
	}
	left, err := api.comparisonSide(request.Context(), sourceID, leftID)
	if err != nil {
		api.writeComparisonError(response, "left", err)
		return
	}
	right, err := api.comparisonSide(request.Context(), sourceID, rightID)
	if err != nil {
		api.writeComparisonError(response, "right", err)
		return
	}
	leftEvents := eventValues(left.Events)
	rightEvents := eventValues(right.Events)
	result, complexity, err := alignment.AlignBounded(leftEvents, rightEvents, MaxComparisonAlignmentWork, MaxComparisonAlignmentWorkspace)
	if err != nil {
		if errors.Is(err, alignment.ErrTooLarge) {
			writeJSONError(response, http.StatusRequestEntityTooLarge, "comparison_too_large", fmt.Errorf(
				"comparison divergent middle %dx%d requires %d alignment cells and %d workspace bytes; maximums are %d and %d",
				complexity.MiddleLeft, complexity.MiddleRight, complexity.WorkCells, complexity.WorkspaceBytes, MaxComparisonAlignmentWork, MaxComparisonAlignmentWorkspace,
			))
			return
		}
		writeJSONError(response, http.StatusInternalServerError, "alignment_failed", err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"source":      source,
		"left":        left,
		"right":       right,
		"alignment":   result,
		"differences": compareDifferences(left, right),
	})
}

func validateComparisonQuery(values url.Values) error {
	if err := validateQuery(values, map[string]bool{"trajectory": true, "left": true, "right": true}); err != nil {
		return err
	}
	for _, name := range []string{"trajectory", "left", "right"} {
		value, err := requiredSingle(values, name)
		if err != nil {
			return err
		}
		if len(value) > 256 {
			return fmt.Errorf("%s must be at most 256 characters", name)
		}
	}
	return nil
}

func (api *indexedAPI) comparisonSide(ctx context.Context, sourceID, trajectoryID string) (comparisonSide, error) {
	trajectoryContext, err := api.reader.TrajectoryContext(ctx, sourceID, trajectoryID)
	if err != nil {
		return comparisonSide{}, err
	}
	events, provenance, rawBytes, err := api.allComparisonEvents(ctx, sourceID, trajectoryID)
	if err != nil {
		return comparisonSide{}, err
	}
	signals, err := api.reader.SignalsPage(ctx, sourceID, trajectoryID, 0, MaxCompleteChildRecords)
	if err != nil {
		return comparisonSide{}, err
	}
	if signals.Total > int64(len(signals.Items)) {
		return comparisonSide{}, fmt.Errorf("%w: trajectory %q has %d signals; maximum is %d", errComparisonTooLarge, trajectoryID, signals.Total, MaxCompleteChildRecords)
	}
	artifacts, err := api.reader.ArtifactsPage(ctx, sourceID, trajectoryID, 0, MaxCompleteChildRecords)
	if err != nil {
		return comparisonSide{}, err
	}
	if artifacts.Total > int64(len(artifacts.Items)) {
		return comparisonSide{}, fmt.Errorf("%w: trajectory %q has %d artifacts; maximum is %d", errComparisonTooLarge, trajectoryID, artifacts.Total, MaxCompleteChildRecords)
	}
	rawBytes += signals.RawBytes + artifacts.RawBytes
	if rawBytes > MaxComparisonRawBytes {
		return comparisonSide{}, fmt.Errorf("%w: trajectory %q comparison input is %d raw bytes; maximum is %d", errComparisonTooLarge, trajectoryID, rawBytes, MaxComparisonRawBytes)
	}
	return comparisonSide{
		Context: trajectoryContext, Run: trajectoryContext.Run.Value, Case: trajectoryContext.Case.Value,
		Group: trajectoryContext.Group.Value, Trajectory: trajectoryContext.Trajectory.Value,
		Events: events, EventProvenance: provenance,
		Signals: canonicalSignals(signals.Items), Artifacts: canonicalArtifacts(artifacts.Items),
	}, nil
}

var errComparisonTooLarge = errors.New("comparison exceeds event limit")

func (api *indexedAPI) allComparisonEvents(ctx context.Context, sourceID, trajectoryID string) ([]*model.Event, []indexedProvenance, int64, error) {
	result := make([]*model.Event, 0)
	provenance := make([]indexedProvenance, 0)
	var rawBytes int64
	var after *int64
	for {
		page, err := api.reader.Events(ctx, rolloutindex.EventQuery{
			SourceID: sourceID, TrajectoryID: trajectoryID, AfterSequence: after, Limit: MaxIndexedPageLimit,
		})
		if err != nil {
			return nil, nil, 0, err
		}
		if page.Total > MaxComparisonEvents || len(result)+len(page.Events) > MaxComparisonEvents {
			return nil, nil, 0, fmt.Errorf("%w: trajectory %q has %d events; maximum is %d", errComparisonTooLarge, trajectoryID, page.Total, MaxComparisonEvents)
		}
		rawBytes += page.RawBytes
		if rawBytes > MaxComparisonRawBytes {
			return nil, nil, 0, fmt.Errorf("%w: trajectory %q events are %d raw bytes; maximum is %d", errComparisonTooLarge, trajectoryID, rawBytes, MaxComparisonRawBytes)
		}
		result = append(result, canonicalEvents(page.Events)...)
		provenance = append(provenance, eventProvenance(page.Events)...)
		if page.NextSequence == nil {
			break
		}
		if len(page.Events) == 0 || (after != nil && *page.NextSequence <= *after) {
			return nil, nil, 0, errors.New("index returned a non-advancing event page")
		}
		next := *page.NextSequence
		after = &next
	}
	return result, provenance, rawBytes, nil
}

func (api *indexedAPI) writeComparisonError(response http.ResponseWriter, side string, err error) {
	if errors.Is(err, errComparisonTooLarge) {
		writeJSONError(response, http.StatusRequestEntityTooLarge, "comparison_too_large", err)
		return
	}
	if errors.Is(err, rolloutindex.ErrNotFound) {
		writeJSONError(response, http.StatusNotFound, side+"_trajectory_not_found", err)
		return
	}
	writeJSONError(response, http.StatusInternalServerError, "index_query_failed", err)
}

func eventValues(events []*model.Event) []model.Event {
	result := make([]model.Event, 0, len(events))
	for _, event := range events {
		if event != nil {
			result = append(result, *event)
		}
	}
	return result
}

func compareDifferences(left, right comparisonSide) comparisonDifferences {
	leftStatus, leftTermination := "", ""
	rightStatus, rightTermination := "", ""
	if left.Trajectory != nil {
		leftStatus, leftTermination = left.Trajectory.Status, left.Trajectory.Termination
	}
	if right.Trajectory != nil {
		rightStatus, rightTermination = right.Trajectory.Status, right.Trajectory.Termination
	}
	leftReward, leftRewardOK := rewardValue(left.Signals)
	rightReward, rightRewardOK := rewardValue(right.Signals)
	return comparisonDifferences{
		EventCount:  countDifference{Left: len(left.Events), Right: len(right.Events), Delta: len(right.Events) - len(left.Events)},
		Status:      valueDifference{Left: leftStatus, Right: rightStatus, Changed: leftStatus != rightStatus},
		Termination: valueDifference{Left: leftTermination, Right: rightTermination, Changed: leftTermination != rightTermination},
		Reward:      valueDifference{Left: optionalValue(leftReward, leftRewardOK), Right: optionalValue(rightReward, rightRewardOK), Changed: !valuesEqual(leftReward, leftRewardOK, rightReward, rightRewardOK)},
	}
}

func rewardValue(signals []*model.Signal) (any, bool) {
	for _, signal := range signals {
		if signal != nil && strings.EqualFold(strings.TrimSpace(signal.Name), "reward") {
			return signal.Value, true
		}
	}
	return nil, false
}

func optionalValue(value any, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

func valuesEqual(left any, leftOK bool, right any, rightOK bool) bool {
	if leftOK != rightOK {
		return false
	}
	if !leftOK {
		return true
	}
	return reflect.DeepEqual(left, right)
}
