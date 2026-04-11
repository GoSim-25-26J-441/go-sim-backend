package http

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyBatchOptimizationGuards_DefaultMaxEvaluations(t *testing.T) {
	raw := json.RawMessage(`{"online":false,"batch":{},"future_field":42}`)
	out, warns, err := applyBatchOptimizationGuards(raw)
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "default 64")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, float64(64), m["max_evaluations"])
	assert.Equal(t, float64(42), m["future_field"])
}

func TestApplyBatchOptimizationGuards_CapMaxEvaluations(t *testing.T) {
	raw := json.RawMessage(`{"batch":{},"max_evaluations":999}`)
	out, warns, err := applyBatchOptimizationGuards(raw)
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "capped from 999 to 256")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, float64(256), m["max_evaluations"])
}

func TestApplyBatchOptimizationGuards_NoBatchUnchanged(t *testing.T) {
	raw := json.RawMessage(`{"online":false,"max_evaluations":0}`)
	out, warns, err := applyBatchOptimizationGuards(raw)
	require.NoError(t, err)
	assert.Empty(t, warns)
	assert.JSONEq(t, string(raw), string(out))
}

func TestApplyBatchOptimizationGuards_AllowedActionsStringBecomesArray(t *testing.T) {
	raw := json.RawMessage(`{"batch":{"allowed_actions":"BATCH_SCALING_ACTION_SCALE_REPLICAS"},"max_evaluations":3}`)
	out, _, err := applyBatchOptimizationGuards(raw)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	batch := m["batch"].(map[string]interface{})
	act := batch["allowed_actions"].([]interface{})
	require.Len(t, act, 1)
	assert.Equal(t, "BATCH_SCALING_ACTION_SCALE_REPLICAS", act[0])
}

func TestApplyBatchOptimizationGuards_BatchAsJSONString_NormalizesAllowedActions(t *testing.T) {
	// batch is sometimes double-encoded as a string; coercion must run normalization inside it.
	raw := json.RawMessage(`{"batch":"{\"allowed_actions\":\"BATCH_SCALING_ACTION_SCALE_REPLICAS\"}","max_evaluations":2}`)
	out, _, err := applyBatchOptimizationGuards(raw)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	batch := m["batch"].(map[string]interface{})
	act := batch["allowed_actions"].([]interface{})
	require.Len(t, act, 1)
	assert.Equal(t, "BATCH_SCALING_ACTION_SCALE_REPLICAS", act[0])
}

func TestApplyBatchOptimizationGuards_AllowedActionsCamelCase(t *testing.T) {
	raw := json.RawMessage(`{"batch":{"allowedActions":"BATCH_SCALING_ACTION_SCALE_REPLICAS"},"max_evaluations":1}`)
	out, _, err := applyBatchOptimizationGuards(raw)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	batch := m["batch"].(map[string]interface{})
	_, hasCamel := batch["allowedActions"]
	assert.False(t, hasCamel)
	act := batch["allowed_actions"].([]interface{})
	require.Len(t, act, 1)
	assert.Equal(t, "BATCH_SCALING_ACTION_SCALE_REPLICAS", act[0])
}

func TestApplyBatchOptimizationGuards_RecommendedConfigObjectiveRewritten(t *testing.T) {
	raw := json.RawMessage(`{"batch":{},"objective":"recommended_config","max_evaluations":5}`)
	out, _, err := applyBatchOptimizationGuards(raw)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, RecommendedConfigEngineObjective, m["objective"])
}

func TestApplyBatchOptimizationGuards_EvaluationDurationWarning(t *testing.T) {
	raw := json.RawMessage(`{"batch":{},"max_evaluations":10,"evaluation_duration_ms":200000}`)
	out, warns, err := applyBatchOptimizationGuards(raw)
	require.NoError(t, err)
	var found bool
	for _, w := range warns {
		if strings.Contains(w, "evaluation_duration_ms") && strings.Contains(w, "interactive") {
			found = true
			break
		}
	}
	assert.True(t, found, "warnings=%v", warns)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, float64(200000), m["evaluation_duration_ms"])
}
