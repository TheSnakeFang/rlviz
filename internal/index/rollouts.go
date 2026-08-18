package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// The query derives only facts with explicit canonical provenance. Generic
// metadata dimensions use a small documented key allowlist; tool outcome is
// intentionally absent until the canonical model has call/result span status.
const rolloutQueryCTE = `WITH
event_summary AS (
  SELECT source_id,trajectory_id,COUNT(*) event_count,
    SUM(CASE WHEN kind='error' THEN 1 ELSE 0 END) error_count,
    SUM(CASE WHEN kind='tool' THEN 1 ELSE 0 END) tool_call_count,
    MIN(sequence) first_sequence,MAX(sequence) last_sequence
  FROM events GROUP BY source_id,trajectory_id
),
signal_ranked AS (
  SELECT source_id,trajectory_id,lower(name) metric,raw,
    ROW_NUMBER() OVER (PARTITION BY source_id,trajectory_id,lower(name) ORDER BY line DESC,id DESC) rank
  FROM signals
  WHERE lower(name) IN ('reward','pass','success','token_count','total_tokens','tokens','cost_usd','total_cost_usd',
    'latency_ms','duration_ms','latency_seconds','duration_seconds','checkpoint','model','model_family','environment_version','env_version')
),
signal_summary AS (
  SELECT source_id,trajectory_id,
    MAX(CASE WHEN metric='reward' AND rank=1 AND json_type(raw,'$.value') IN ('integer','real') THEN CAST(json_extract(raw,'$.value') AS REAL) END) reward,
    COALESCE(
      MAX(CASE WHEN metric='pass' AND rank=1 AND json_type(raw,'$.value') IN ('true','false') THEN json_extract(raw,'$.value') END),
      MAX(CASE WHEN metric='success' AND rank=1 AND json_type(raw,'$.value') IN ('true','false') THEN json_extract(raw,'$.value') END)
    ) pass,
    COALESCE(
      MAX(CASE WHEN metric='token_count' AND rank=1 AND json_type(raw,'$.value')='integer' AND CAST(json_extract(raw,'$.value') AS INTEGER)>=0 THEN CAST(json_extract(raw,'$.value') AS INTEGER) END),
      MAX(CASE WHEN metric='total_tokens' AND rank=1 AND json_type(raw,'$.value')='integer' AND CAST(json_extract(raw,'$.value') AS INTEGER)>=0 THEN CAST(json_extract(raw,'$.value') AS INTEGER) END),
      MAX(CASE WHEN metric='tokens' AND rank=1 AND json_type(raw,'$.value')='integer' AND CAST(json_extract(raw,'$.value') AS INTEGER)>=0 THEN CAST(json_extract(raw,'$.value') AS INTEGER) END)
    ) token_count,
    COALESCE(
      MAX(CASE WHEN metric='cost_usd' AND rank=1 AND json_type(raw,'$.value') IN ('integer','real') AND CAST(json_extract(raw,'$.value') AS REAL)>=0 THEN CAST(json_extract(raw,'$.value') AS REAL) END),
      MAX(CASE WHEN metric='total_cost_usd' AND rank=1 AND json_type(raw,'$.value') IN ('integer','real') AND CAST(json_extract(raw,'$.value') AS REAL)>=0 THEN CAST(json_extract(raw,'$.value') AS REAL) END)
    ) cost_usd,
    COALESCE(
      MAX(CASE WHEN metric='latency_ms' AND rank=1 AND json_type(raw,'$.value') IN ('integer','real') AND CAST(json_extract(raw,'$.value') AS REAL)>=0 THEN CAST(json_extract(raw,'$.value') AS REAL) END),
      MAX(CASE WHEN metric='duration_ms' AND rank=1 AND json_type(raw,'$.value') IN ('integer','real') AND CAST(json_extract(raw,'$.value') AS REAL)>=0 THEN CAST(json_extract(raw,'$.value') AS REAL) END),
      MAX(CASE WHEN metric='latency_seconds' AND rank=1 AND json_type(raw,'$.value') IN ('integer','real') AND CAST(json_extract(raw,'$.value') AS REAL)>=0 THEN CAST(json_extract(raw,'$.value') AS REAL)*1000 END),
      MAX(CASE WHEN metric='duration_seconds' AND rank=1 AND json_type(raw,'$.value') IN ('integer','real') AND CAST(json_extract(raw,'$.value') AS REAL)>=0 THEN CAST(json_extract(raw,'$.value') AS REAL)*1000 END)
    ) latency_ms,
    MAX(CASE WHEN metric='checkpoint' AND rank=1 AND json_type(raw,'$.value')='text' THEN json_extract(raw,'$.value') END) checkpoint,
    COALESCE(
      MAX(CASE WHEN metric='model' AND rank=1 AND json_type(raw,'$.value')='text' THEN json_extract(raw,'$.value') END),
      MAX(CASE WHEN metric='model_family' AND rank=1 AND json_type(raw,'$.value')='text' THEN json_extract(raw,'$.value') END)
    ) model,
    COALESCE(
      MAX(CASE WHEN metric='environment_version' AND rank=1 AND json_type(raw,'$.value')='text' THEN json_extract(raw,'$.value') END),
      MAX(CASE WHEN metric='env_version' AND rank=1 AND json_type(raw,'$.value')='text' THEN json_extract(raw,'$.value') END)
    ) environment_version
  FROM signal_ranked GROUP BY source_id,trajectory_id
),
signal_counts AS (SELECT source_id,trajectory_id,COUNT(*) signal_count FROM signals GROUP BY source_id,trajectory_id),
artifact_counts AS (SELECT source_id,trajectory_id,COUNT(*) artifact_count FROM artifacts GROUP BY source_id,trajectory_id),
candidates AS (
  SELECT src.id source_id,src.path source_path,src.indexed_at_ns,
    r.id run_id,r.name run_name,c.id case_id,c.name case_name,g.id group_id,g.name group_name,
    COALESCE(CASE WHEN json_type(t.raw,'$.metadata.checkpoint')='text' THEN json_extract(t.raw,'$.metadata.checkpoint') END,
      CASE WHEN json_type(r.raw,'$.metadata.checkpoint')='text' THEN json_extract(r.raw,'$.metadata.checkpoint') END,ss.checkpoint,'') checkpoint,
    COALESCE(CASE WHEN json_type(t.raw,'$.metadata.model')='text' THEN json_extract(t.raw,'$.metadata.model') END,
      CASE WHEN json_type(r.raw,'$.metadata.model')='text' THEN json_extract(r.raw,'$.metadata.model') END,ss.model,'') model,
    COALESCE(CASE WHEN json_type(t.raw,'$.metadata.environment_version')='text' THEN json_extract(t.raw,'$.metadata.environment_version') END,
      CASE WHEN json_type(t.raw,'$.metadata.env_version')='text' THEN json_extract(t.raw,'$.metadata.env_version') END,
      CASE WHEN json_type(r.raw,'$.metadata.environment_version')='text' THEN json_extract(r.raw,'$.metadata.environment_version') END,
      CASE WHEN json_type(r.raw,'$.metadata.env_version')='text' THEN json_extract(r.raw,'$.metadata.env_version') END,ss.environment_version,'') environment_version,
    t.id trajectory_id,t.raw trajectory_raw,t.line trajectory_line,t.byte_offset trajectory_byte_offset,t.byte_length trajectory_byte_length,
    t.status,t.termination,COALESCE(es.event_count,0) event_count,COALESCE(es.error_count,0) error_count,
    COALESCE(es.tool_call_count,0) tool_call_count,es.first_sequence,es.last_sequence,
    COALESCE(sc.signal_count,0) signal_count,COALESCE(ac.artifact_count,0) artifact_count,
    ss.reward,ss.pass,ss.token_count,ss.cost_usd,ss.latency_ms
  FROM trajectories t
  JOIN sources src ON src.id=t.source_id
  JOIN groups g ON g.source_id=t.source_id AND g.id=t.group_id
  JOIN cases c ON c.source_id=g.source_id AND c.id=g.case_id
  JOIN runs r ON r.source_id=c.source_id AND r.id=c.run_id
  LEFT JOIN event_summary es ON es.source_id=t.source_id AND es.trajectory_id=t.id
  LEFT JOIN signal_summary ss ON ss.source_id=t.source_id AND ss.trajectory_id=t.id
  LEFT JOIN signal_counts sc ON sc.source_id=t.source_id AND sc.trajectory_id=t.id
  LEFT JOIN artifact_counts ac ON ac.source_id=t.source_id AND ac.trajectory_id=t.id
)
`

func (i *Index) QueryRollouts(ctx context.Context, query RolloutQuery) (RolloutQueryPage, error) {
	query.Limit = boundedQueryLimit(query.Limit)
	if query.Offset < 0 {
		query.Offset = 0
	}
	tx, err := i.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return RolloutQueryPage{}, fmt.Errorf("begin rollout query snapshot: %w", err)
	}
	defer tx.Rollback()
	where, args := rolloutWhere(query)
	aggregates, err := rolloutAggregates(ctx, tx, where, args)
	if err != nil {
		return RolloutQueryPage{}, err
	}
	page := RolloutQueryPage{Total: aggregates.Count, Offset: query.Offset, Limit: query.Limit, Aggregates: aggregates}
	if query.Offset >= page.Total {
		if err := tx.Commit(); err != nil {
			return page, fmt.Errorf("commit rollout query snapshot: %w", err)
		}
		return page, nil
	}
	order := rolloutOrder(query.Sort, query.Descending)
	selectedSQL := rolloutQueryCTE + `, selected AS (
  SELECT candidates.*,ROW_NUMBER() OVER (ORDER BY ` + order + `) page_order
  FROM candidates WHERE ` + strings.Join(where, " AND ") + `
  ORDER BY ` + order + ` LIMIT ? OFFSET ?
)
SELECT selected.source_id,selected.source_path,selected.run_id,selected.run_name,selected.case_id,selected.case_name,
  selected.group_id,selected.group_name,selected.checkpoint,selected.model,selected.environment_version,selected.trajectory_id,
  selected.trajectory_raw,selected.trajectory_line,selected.trajectory_byte_offset,selected.trajectory_byte_length,
  selected.event_count,selected.error_count,selected.tool_call_count,selected.first_sequence,selected.last_sequence,
  selected.signal_count,selected.artifact_count,selected.reward,selected.pass,selected.token_count,selected.cost_usd,selected.latency_ms,
  s.name,s.raw,s.line,selected.page_order
FROM selected LEFT JOIN signals s ON s.source_id=selected.source_id AND s.trajectory_id=selected.trajectory_id
ORDER BY selected.page_order,s.line,s.id`
	pageArgs := append(append([]any(nil), args...), query.Limit, query.Offset)
	rows, err := tx.QueryContext(ctx, selectedSQL, pageArgs...)
	if err != nil {
		return page, fmt.Errorf("query rollouts: %w", err)
	}
	defer rows.Close()
	byKey := make(map[string]int)
	var readBytes, signalRows int64
	for rows.Next() {
		var item RolloutSummary
		var trajectoryID string
		var trajectoryRaw, signalRaw []byte
		var first, last sql.NullInt64
		var reward, cost, latency sql.NullFloat64
		var pass, tokens sql.NullInt64
		var signalName sql.NullString
		var signalLine, pageOrder sql.NullInt64
		if err := rows.Scan(&item.SourceID, &item.SourceName, &item.RunID, &item.RunName, &item.CaseID, &item.CaseName,
			&item.GroupID, &item.GroupName, &item.Checkpoint, &item.Model, &item.EnvironmentVersion,
			&trajectoryID,
			&trajectoryRaw, &item.Summary.Trajectory.Line, &item.Summary.Trajectory.ByteOffset, &item.Summary.Trajectory.ByteLength,
			&item.Summary.EventCount, &item.Summary.ErrorCount, &item.Summary.ToolCallCount, &first, &last,
			&item.Summary.SignalCount, &item.Summary.ArtifactCount, &reward, &pass, &tokens, &cost, &latency,
			&signalName, &signalRaw, &signalLine, &pageOrder); err != nil {
			return page, err
		}
		readBytes += int64(len(trajectoryRaw) + len(signalRaw))
		if readBytes > MaxGroupSummaryRawBytes {
			return page, fmt.Errorf("%w: rollout query exceeded maximum %d raw bytes", ErrResultTooLarge, MaxGroupSummaryRawBytes)
		}
		key := item.SourceID + "\x00" + trajectoryID
		index, exists := byKey[key]
		if !exists {
			item.SourceName = filepath.Base(item.SourceName)
			if err := decodeRaw(trajectoryRaw, &item.Summary.Trajectory.Value, &item.Summary.Trajectory.Raw); err != nil {
				return page, err
			}
			item.Summary.RunName, item.Summary.CaseName, item.Summary.GroupName = item.RunName, item.CaseName, item.GroupName
			item.Summary.Status, item.Summary.Termination = item.Summary.Trajectory.Value.Status, item.Summary.Trajectory.Value.Termination
			item.Summary.Signals, item.Summary.signalUnits = make(map[string]json.RawMessage), make(map[string]string)
			if first.Valid {
				value := first.Int64
				item.Summary.FirstSequence = &value
			}
			if last.Valid {
				value := last.Int64
				item.Summary.LastSequence = &value
			}
			if reward.Valid {
				value := reward.Float64
				item.Summary.Reward = &value
			}
			if pass.Valid {
				value := pass.Int64 != 0
				item.Summary.Success = &value
			}
			if tokens.Valid {
				value := tokens.Int64
				item.Summary.TokenCount = &value
			}
			if cost.Valid {
				value := cost.Float64
				item.Summary.CostUSD = &value
			}
			if latency.Valid {
				value := latency.Float64
				item.Summary.LatencyMS = &value
			}
			page.Items = append(page.Items, item)
			index = len(page.Items) - 1
			byKey[key] = index
		}
		if signalName.Valid {
			signalRows++
			if signalRows > MaxGroupSummarySignals {
				return page, fmt.Errorf("%w: rollout query exceeded maximum %d signals", ErrResultTooLarge, MaxGroupSummarySignals)
			}
			value, unit, valueErr := signalValue(signalRaw)
			if valueErr != nil {
				return page, fmt.Errorf("decode signal %q: %w", signalName.String, valueErr)
			}
			name := canonicalMetricName(signalName.String)
			page.Items[index].Summary.Signals[name] = value
			page.Items[index].Summary.signalUnits[name] = strings.ToLower(unit)
		}
	}
	rowErr := rows.Err()
	closeErr := rows.Close()
	if rowErr != nil {
		return page, rowErr
	}
	if closeErr != nil {
		return page, closeErr
	}
	if err := tx.Commit(); err != nil {
		return page, fmt.Errorf("commit rollout query snapshot: %w", err)
	}
	return page, nil
}

type rolloutQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func rolloutAggregates(ctx context.Context, querier rolloutQuerier, where []string, args []any) (RolloutQueryAggregates, error) {
	query := rolloutQueryCTE + `SELECT COUNT(*),
  COALESCE(SUM(CASE WHEN pass=1 THEN 1 ELSE 0 END),0),
  COALESCE(SUM(CASE WHEN pass=0 THEN 1 ELSE 0 END),0),
  COALESCE(SUM(CASE WHEN pass IS NULL THEN 1 ELSE 0 END),0),
  MIN(reward),MAX(reward),AVG(reward),MIN(token_count),MAX(token_count),
  MIN(cost_usd),MAX(cost_usd),AVG(cost_usd),SUM(cost_usd),MIN(tool_call_count),MAX(tool_call_count)
FROM candidates WHERE ` + strings.Join(where, " AND ")
	var result RolloutQueryAggregates
	var rewardMin, rewardMax, rewardMean, costMin, costMax, costMean, totalCost sql.NullFloat64
	var tokenMin, tokenMax, toolMin, toolMax sql.NullInt64
	if err := querier.QueryRowContext(ctx, query, args...).Scan(&result.Count, &result.Success, &result.Failure, &result.Unknown,
		&rewardMin, &rewardMax, &rewardMean, &tokenMin, &tokenMax,
		&costMin, &costMax, &costMean, &totalCost, &toolMin, &toolMax); err != nil {
		return result, fmt.Errorf("aggregate rollouts: %w", err)
	}
	if rewardMin.Valid {
		result.Reward = &NumericRange{Min: rewardMin.Float64, Max: rewardMax.Float64, Mean: rewardMean.Float64}
	}
	if tokenMin.Valid {
		result.TokenCount = &IntegerRange{Min: tokenMin.Int64, Max: tokenMax.Int64}
	}
	if costMin.Valid {
		result.CostUSD = &NumericRange{Min: costMin.Float64, Max: costMax.Float64, Mean: costMean.Float64}
	}
	if totalCost.Valid {
		value := totalCost.Float64
		result.TotalCostUSD = &value
	}
	if toolMin.Valid {
		result.ToolCallCount = &IntegerRange{Min: toolMin.Int64, Max: toolMax.Int64}
	}
	return result, nil
}

func rolloutWhere(query RolloutQuery) ([]string, []any) {
	where := []string{"1=1"}
	args := make([]any, 0)
	addExact := func(column, value string) {
		if value != "" {
			where = append(where, column+"=?")
			args = append(args, value)
		}
	}
	addNamed := func(idColumn, nameColumn, value string) {
		if value != "" {
			where = append(where, "("+idColumn+"=? OR "+nameColumn+"=?)")
			args = append(args, value, value)
		}
	}
	addExact("source_id", query.SourceID)
	addNamed("run_id", "run_name", query.Run)
	addNamed("case_id", "case_name", query.Case)
	addNamed("group_id", "group_name", query.Group)
	addExact("checkpoint", query.Checkpoint)
	addExact("model", query.Model)
	addExact("environment_version", query.EnvironmentVersion)
	addExact("status", query.Status)
	addExact("termination", query.Termination)
	if query.Pass != nil {
		where = append(where, "pass=?")
		args = append(args, *query.Pass)
	}
	addFloatRange := func(column string, min, max *float64) {
		if min != nil {
			where = append(where, column+">=?")
			args = append(args, *min)
		}
		if max != nil {
			where = append(where, column+"<=?")
			args = append(args, *max)
		}
	}
	addFloatRange("reward", query.RewardMin, query.RewardMax)
	addFloatRange("cost_usd", query.CostMin, query.CostMax)
	if query.TokensMin != nil {
		where = append(where, "token_count>=?")
		args = append(args, *query.TokensMin)
	}
	if query.TokensMax != nil {
		where = append(where, "token_count<=?")
		args = append(args, *query.TokensMax)
	}
	if query.Tool != "" {
		where = append(where, `EXISTS (SELECT 1 FROM events tool_event WHERE tool_event.source_id=candidates.source_id
      AND tool_event.trajectory_id=candidates.trajectory_id AND tool_event.kind='tool'
      AND json_extract(tool_event.raw,'$.input.name')=? COLLATE NOCASE)`)
		args = append(args, query.Tool)
	}
	if query.Query != "" {
		pattern := "%" + escapeLike(query.Query) + "%"
		where = append(where, `(source_path LIKE ? ESCAPE '\' COLLATE NOCASE OR run_id LIKE ? ESCAPE '\' COLLATE NOCASE
      OR run_name LIKE ? ESCAPE '\' COLLATE NOCASE OR case_id LIKE ? ESCAPE '\' COLLATE NOCASE OR case_name LIKE ? ESCAPE '\' COLLATE NOCASE
      OR group_id LIKE ? ESCAPE '\' COLLATE NOCASE OR group_name LIKE ? ESCAPE '\' COLLATE NOCASE OR trajectory_id LIKE ? ESCAPE '\' COLLATE NOCASE
      OR status LIKE ? ESCAPE '\' COLLATE NOCASE OR termination LIKE ? ESCAPE '\' COLLATE NOCASE
      OR checkpoint LIKE ? ESCAPE '\' COLLATE NOCASE OR model LIKE ? ESCAPE '\' COLLATE NOCASE OR environment_version LIKE ? ESCAPE '\' COLLATE NOCASE)`)
		for range 13 {
			args = append(args, pattern)
		}
	}
	return where, args
}

func rolloutOrder(sortName string, descending bool) string {
	column := map[string]string{
		"reward": "reward", "tokens": "token_count", "cost": "cost_usd", "tools": "tool_call_count",
		"run": "run_name", "case": "case_name", "checkpoint": "checkpoint", "model": "model",
	}[sortName]
	if column == "" {
		column = "indexed_at_ns"
		descending = true
	}
	direction := "ASC"
	if descending {
		direction = "DESC"
	}
	unknown := column + " IS NULL"
	if sortName == "run" || sortName == "case" || sortName == "checkpoint" || sortName == "model" {
		unknown = column + "=''"
	}
	return unknown + " ASC," + column + " " + direction + ",source_id ASC,trajectory_line ASC,trajectory_id ASC"
}
