package index

import (
	"bytes"
	"encoding/json"
	"strconv"
)

func signalValue(raw []byte) (json.RawMessage, string, error) {
	var envelope struct {
		Value json.RawMessage `json:"value"`
		Unit  string          `json:"unit"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, "", err
	}
	return append(json.RawMessage(nil), envelope.Value...), envelope.Unit, nil
}

// Normalized metric precedence is deliberately narrow and exact:
// reward; pass then success; token_count then total_tokens then tokens;
// error_count (falling back to the count of events whose kind is "error");
// latency_ms then duration_ms, followed by *_seconds, then bare latency and
// duration only when their unit is exactly ms/millisecond(s) or s/second(s).
// Values of an incompatible type are ignored rather than guessed or coerced.
func normalizeSummary(summary *TrajectorySummary) {
	summary.Reward = firstFloat(summary.Signals, "reward")
	summary.Success = firstBool(summary.Signals, "pass", "success")
	summary.TokenCount = firstInt(summary.Signals, "token_count", "total_tokens", "tokens")
	if errorCount := firstInt(summary.Signals, "error_count"); errorCount != nil {
		summary.ErrorCount = *errorCount
	}
	if value := firstNonnegativeFloat(summary.Signals, "latency_ms", "duration_ms"); value != nil {
		summary.LatencyMS = value
	} else if value := firstNonnegativeFloat(summary.Signals, "latency_seconds", "duration_seconds"); value != nil {
		milliseconds := *value * 1000
		summary.LatencyMS = &milliseconds
	} else {
		summary.LatencyMS = firstDurationMS(summary.Signals, summary.signalUnits, "latency", "duration")
	}
}

func firstDurationMS(signals map[string]json.RawMessage, units map[string]string, names ...string) *float64 {
	for _, name := range names {
		value := firstNonnegativeFloat(signals, name)
		if value == nil {
			continue
		}
		switch units[name] {
		case "ms", "millisecond", "milliseconds":
			return value
		case "s", "second", "seconds":
			milliseconds := *value * 1000
			return &milliseconds
		}
	}
	return nil
}

func firstNonnegativeFloat(signals map[string]json.RawMessage, names ...string) *float64 {
	for _, name := range names {
		value := firstFloat(signals, name)
		if value != nil && *value >= 0 {
			return value
		}
	}
	return nil
}

func AggregateGroup(summaries []TrajectorySummary) GroupAggregates {
	aggregates := GroupAggregates{Count: len(summaries)}
	var rewards, latencies []float64
	var events, errors, tokens []int64
	for _, summary := range summaries {
		switch {
		case summary.Success == nil:
			aggregates.Unknown++
		case *summary.Success:
			aggregates.Success++
		default:
			aggregates.Failure++
		}
		events = append(events, summary.EventCount)
		errors = append(errors, summary.ErrorCount)
		if summary.Reward != nil {
			rewards = append(rewards, *summary.Reward)
		}
		if summary.TokenCount != nil {
			tokens = append(tokens, *summary.TokenCount)
		}
		if summary.LatencyMS != nil {
			latencies = append(latencies, *summary.LatencyMS)
		}
	}
	aggregates.Reward = numericRange(rewards)
	aggregates.EventCount = integerRange(events)
	aggregates.ErrorCount = integerRange(errors)
	aggregates.TokenCount = integerRange(tokens)
	aggregates.LatencyMS = numericRange(latencies)
	return aggregates
}

func numericRange(values []float64) *NumericRange {
	if len(values) == 0 {
		return nil
	}
	result := &NumericRange{Min: values[0], Max: values[0]}
	for _, value := range values {
		if value < result.Min {
			result.Min = value
		}
		if value > result.Max {
			result.Max = value
		}
		result.Mean += value
	}
	result.Mean /= float64(len(values))
	return result
}

func integerRange(values []int64) *IntegerRange {
	if len(values) == 0 {
		return nil
	}
	result := &IntegerRange{Min: values[0], Max: values[0]}
	for _, value := range values[1:] {
		if value < result.Min {
			result.Min = value
		}
		if value > result.Max {
			result.Max = value
		}
	}
	return result
}

func firstFloat(signals map[string]json.RawMessage, names ...string) *float64 {
	for _, name := range names {
		if raw, ok := signals[name]; ok {
			var number json.Number
			if err := json.Unmarshal(raw, &number); err == nil {
				if value, err := number.Float64(); err == nil {
					return &value
				}
			}
		}
	}
	return nil
}

func firstInt(signals map[string]json.RawMessage, names ...string) *int64 {
	for _, name := range names {
		if raw, ok := signals[name]; ok {
			value, err := strconv.ParseInt(string(raw), 10, 64)
			if err == nil && value >= 0 {
				return &value
			}
		}
	}
	return nil
}

func firstBool(signals map[string]json.RawMessage, names ...string) *bool {
	for _, name := range names {
		if raw, ok := signals[name]; ok {
			var value bool
			if err := json.Unmarshal(raw, &value); err == nil {
				return &value
			}
			switch string(raw) {
			case "0":
				value := false
				return &value
			case "1":
				value := true
				return &value
			}
		}
	}
	return nil
}
