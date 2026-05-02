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
	yaml := minimalValidCoreScenarioYAML("svc-batch-guard-default")
	out, warns, err := applyBatchOptimizationGuards(raw, yaml)
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "default 64")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, float64(64), m["max_evaluations"])
	assert.Equal(t, float64(42), m["future_field"])
	batch := m["batch"].(map[string]interface{})
	assert.Equal(t, float64(1), batch["min_hosts"])
	assert.Equal(t, float64(1), batch["max_hosts"])
	assert.Equal(t, float64(4), batch["min_host_cpu_cores"])
	assert.Equal(t, float64(16), batch["min_host_memory_gb"])
	act := batch["allowed_actions"].([]interface{})
	require.Len(t, act, 2)
	assert.Equal(t, BatchScalingActionScaleReplicas, act[0])
	assert.Equal(t, BatchScalingActionScaleHosts, act[1])
}

func TestApplyBatchOptimizationGuards_CapMaxEvaluations(t *testing.T) {
	raw := json.RawMessage(`{"batch":{},"max_evaluations":999}`)
	yaml := minimalValidCoreScenarioYAML("svc-batch-guard-cap")
	out, warns, err := applyBatchOptimizationGuards(raw, yaml)
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "capped from 999 to 256")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, float64(256), m["max_evaluations"])
}

func TestApplyBatchOptimizationGuards_NoBatchUnchanged(t *testing.T) {
	raw := json.RawMessage(`{"online":false,"max_evaluations":0}`)
	out, warns, err := applyBatchOptimizationGuards(raw, "")
	require.NoError(t, err)
	assert.Empty(t, warns)
	assert.JSONEq(t, string(raw), string(out))
}

func TestApplyBatchOptimizationGuards_AllowedActionsStringBecomesArray(t *testing.T) {
	raw := json.RawMessage(`{"batch":{"allowed_actions":"BATCH_SCALING_ACTION_SCALE_REPLICAS"},"max_evaluations":3}`)
	yaml := minimalValidCoreScenarioYAML("svc-batch-guard-actions-str")
	out, _, err := applyBatchOptimizationGuards(raw, yaml)
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
	yaml := minimalValidCoreScenarioYAML("svc-batch-guard-actions-json-str")
	out, _, err := applyBatchOptimizationGuards(raw, yaml)
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
	yaml := minimalValidCoreScenarioYAML("svc-batch-guard-camel")
	out, _, err := applyBatchOptimizationGuards(raw, yaml)
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
	yaml := minimalValidCoreScenarioYAML("svc-batch-guard-rc")
	out, _, err := applyBatchOptimizationGuards(raw, yaml)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, RecommendedConfigEngineObjective, m["objective"])
}

func TestApplyBatchOptimizationGuards_EvaluationDurationWarning(t *testing.T) {
	raw := json.RawMessage(`{"batch":{},"max_evaluations":10,"evaluation_duration_ms":200000}`)
	yaml := minimalValidCoreScenarioYAML("svc-batch-guard-evaldur")
	out, warns, err := applyBatchOptimizationGuards(raw, yaml)
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

func TestApplyBatchOptimizationGuards_EmptyScenarioYAML(t *testing.T) {
	raw := json.RawMessage(`{"batch":{}}`)
	_, _, err := applyBatchOptimizationGuards(raw, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hosts")
}

func TestApplyBatchOptimizationGuards_NodesOnlyRequiresExplicitHostResources(t *testing.T) {
	raw := json.RawMessage(`{"batch":{},"max_evaluations":10}`)
	yaml := `nodes: 2
hosts: []
services:
  - id: svc
    replicas: 1
    model: cpu
    endpoints:
      - path: /read
        mean_cpu_ms: 1
        cpu_sigma_ms: 0
        net_latency_ms: {mean: 1, sigma: 0.1}
        downstream: []
workload:
  - from: client
    to: svc:/read
    arrival:
      type: poisson
      rate_rps: 10
`
	_, _, err := applyBatchOptimizationGuards(raw, yaml)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min_host_cpu_cores")
}

func TestApplyBatchOptimizationGuards_NodesOnlyWithExplicitResourcesOK(t *testing.T) {
	raw := json.RawMessage(`{"batch":{"min_host_cpu_cores":2,"min_host_memory_gb":8},"max_evaluations":10}`)
	yaml := `nodes: 2
hosts: []
services:
  - id: svc
    replicas: 1
    model: cpu
    endpoints:
      - path: /read
        mean_cpu_ms: 1
        cpu_sigma_ms: 0
        net_latency_ms: {mean: 1, sigma: 0.1}
        downstream: []
workload:
  - from: client
    to: svc:/read
    arrival:
      type: poisson
      rate_rps: 10
`
	out, _, err := applyBatchOptimizationGuards(raw, yaml)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	batch := m["batch"].(map[string]interface{})
	assert.Equal(t, float64(2), batch["max_hosts"])
	assert.Equal(t, float64(2), batch["min_host_cpu_cores"])
	assert.Equal(t, float64(8), batch["min_host_memory_gb"])
}

func TestApplyBatchOptimizationGuards_HeterogeneousHostsUsesMinimums(t *testing.T) {
	raw := json.RawMessage(`{"batch":{},"max_evaluations":10}`)
	yaml := `hosts:
  - id: big
    cores: 8
    memory_gb: 32
  - id: small
    cores: 4
    memory_gb: 16
services:
  - id: svc
    replicas: 1
    model: cpu
    endpoints:
      - path: /read
        mean_cpu_ms: 1
        cpu_sigma_ms: 0
        net_latency_ms: {mean: 1, sigma: 0.1}
        downstream: []
workload:
  - from: client
    to: svc:/read
    arrival:
      type: poisson
      rate_rps: 10
`
	out, _, err := applyBatchOptimizationGuards(raw, yaml)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &m))
	batch := m["batch"].(map[string]interface{})
	assert.Equal(t, float64(4), batch["min_host_cpu_cores"])
	assert.Equal(t, float64(16), batch["min_host_memory_gb"])
	assert.Equal(t, float64(2), batch["max_hosts"])
}
