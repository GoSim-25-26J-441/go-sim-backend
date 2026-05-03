package http

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Batch evaluation guardrails (BFF). Simulation-core may apply its own limits; this prevents unbounded batch cost
// when clients omit max_evaluations. Exhaustion text in batch summaries, host memory admission as non-error,
// cost_weights defaults, and early-stop are implemented in simulation-core, not this repo.
const (
	DefaultBatchMaxEvaluations int32 = 64
	// MaxBatchMaxEvaluationsCap is the hard ceiling applied to optimization.max_evaluations when batch is set.
	MaxBatchMaxEvaluationsCap int32 = 256
	// maxInteractiveEvaluationDurationMs triggers a non-blocking create warning only (value is not clamped here).
	maxInteractiveEvaluationDurationMs int64 = 120_000
)

// batchMetadataNormalizedAllowedActionsKey is nested under run metadata as metadata.batch.normalized_allowed_actions (replay/export debugging).
const batchMetadataNormalizedAllowedActionsKey = "normalized_allowed_actions"

// applyBatchOptimizationGuards adjusts optimization JSON before forwarding to simulation-core when optimization.batch
// is a non-null object. Unknown top-level keys are preserved via map round-trip.
// scenarioYAML is used to fill missing fleet bounds in batch when safe; empty yaml returns a validation error for incomplete batch.
// When allowed_actions is omitted, it is not synthesized for the engine payload; batchMeta carries normalized_allowed_actions for metadata only.
func applyBatchOptimizationGuards(raw json.RawMessage, scenarioYAML string) (json.RawMessage, []string, map[string]interface{}, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw, nil, nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, nil, fmt.Errorf("optimization: %w", err)
	}
	batchMap, err := coerceBatchToMapInPlace(m)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("optimization: %w", err)
	}
	if batchMap == nil {
		return raw, nil, nil, nil
	}
	normalizeBatchJSONForEngine(batchMap)
	if arr, ok := batchMap["allowed_actions"].([]interface{}); ok && len(arr) == 0 {
		delete(batchMap, "allowed_actions")
	}

	if err = populateBatchFleetDefaults(batchMap, scenarioYAML); err != nil {
		return nil, nil, nil, err
	}

	allowedActionsOmitted := batchAllowedActionsAbsentOrEmpty(batchMap)
	var explicitNumerics []int32
	if !allowedActionsOmitted {
		explicitNumerics, err = parseAllowedActionsToNumerics(batchMap)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("optimization: %w", err)
		}
	}

	var warnings []string
	if w := batchHostScalingWithoutHostActionsWarning(batchMap, scenarioYAML, allowedActionsOmitted, explicitNumerics); w != "" {
		warnings = append(warnings, w)
	}

	meKey := "max_evaluations"
	cur, present := jsonNumberToInt64(m[meKey])
	if !present || cur <= 0 {
		m[meKey] = float64(DefaultBatchMaxEvaluations)
		warnings = append(warnings, fmt.Sprintf(
			"batch optimization: max_evaluations was unset or non-positive; applied default %d (server cap %d)",
			DefaultBatchMaxEvaluations, MaxBatchMaxEvaluationsCap,
		))
		cur = int64(DefaultBatchMaxEvaluations)
	}
	if cur > int64(MaxBatchMaxEvaluationsCap) {
		before := cur
		m[meKey] = float64(MaxBatchMaxEvaluationsCap)
		warnings = append(warnings, fmt.Sprintf(
			"batch optimization: max_evaluations capped from %d to %d",
			before, MaxBatchMaxEvaluationsCap,
		))
	}

	if dur, ok := jsonNumberToInt64(m["evaluation_duration_ms"]); ok && dur > maxInteractiveEvaluationDurationMs {
		warnings = append(warnings, fmt.Sprintf(
			"batch optimization: evaluation_duration_ms=%d may be slow for interactive use (threshold %d ms)",
			dur, maxInteractiveEvaluationDurationMs,
		))
	}

	if o, ok := m["objective"].(string); ok && o == ObjectiveRecommendedConfig {
		m["objective"] = RecommendedConfigEngineObjective
	}

	debug, err := buildNormalizedAllowedActionsDebug(batchMap, allowedActionsOmitted)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("optimization: %w", err)
	}
	batchMeta := map[string]interface{}{
		batchMetadataNormalizedAllowedActionsKey: debug,
	}

	out, err := json.Marshal(m)
	if err != nil {
		return nil, nil, nil, err
	}
	return json.RawMessage(out), warnings, batchMeta, nil
}

func batchAllowedActionsAbsentOrEmpty(batch map[string]interface{}) bool {
	v, ok := batch["allowed_actions"]
	if !ok || v == nil {
		return true
	}
	arr, ok := v.([]interface{})
	if !ok {
		return false
	}
	return len(arr) == 0
}

// coerceBatchToMapInPlace ensures m["batch"] is a map[string]interface{}.
// Clients sometimes send batch as a JSON string; without this, guard/normalization is skipped and the engine sees bad JSON.
func coerceBatchToMapInPlace(m map[string]interface{}) (map[string]interface{}, error) {
	v, ok := m["batch"]
	if !ok || v == nil {
		return nil, nil
	}
	switch x := v.(type) {
	case map[string]interface{}:
		return x, nil
	case string:
		var inner map[string]interface{}
		if err := json.Unmarshal([]byte(x), &inner); err != nil {
			return nil, fmt.Errorf("batch must be a JSON object or a string containing a JSON object: %w", err)
		}
		m["batch"] = inner
		return inner, nil
	default:
		return nil, fmt.Errorf("batch must be a JSON object (got %T)", x)
	}
}

// normalizeBatchJSONForEngine fixes shapes that break simulation-core JSON decode into generated protos.
// Proto repeated enums must be JSON arrays; a single string must become ["VALUE"].
func normalizeBatchJSONForEngine(batch map[string]interface{}) {
	// JSONPB / some clients use camelCase
	if _, has := batch["allowed_actions"]; !has {
		if v, ok := batch["allowedActions"]; ok {
			batch["allowed_actions"] = v
			delete(batch, "allowedActions")
		}
	}

	v, ok := batch["allowed_actions"]
	if !ok || v == nil {
		return
	}
	switch x := v.(type) {
	case string:
		batch["allowed_actions"] = []interface{}{x}
	case float64:
		batch["allowed_actions"] = []interface{}{x}
	case json.Number:
		batch["allowed_actions"] = []interface{}{x}
	case int:
		batch["allowed_actions"] = []interface{}{x}
	case int32:
		batch["allowed_actions"] = []interface{}{x}
	case int64:
		batch["allowed_actions"] = []interface{}{x}
	case []interface{}:
		// Already a JSON array; elements are typically strings (enum names).
	default:
		// leave as-is
	}
}

func jsonNumberToInt64(v interface{}) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case json.Number:
		i, err := x.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}
