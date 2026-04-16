package http

import "errors"

// recommended_config is a batch-only public objective name. applyBatchOptimizationGuards rewrites it to
// RecommendedConfigEngineObjective before the engine request is sent (mirrors simulation-core: if params.Batch != nil && objName == "recommended_config" { objName = "p95_latency_ms" } before improvement.NewObjectiveFunction).
// Legacy hill-climb wiring keeps a normal metric name; batch scoring and batch_* fields remain authoritative.
const (
	ObjectiveRecommendedConfig       = "recommended_config"
	RecommendedConfigEngineObjective = "p95_latency_ms"
)

// allowedOptimizationObjectives is the set of valid values for optimization.objective in non-alias modes.
var allowedOptimizationObjectives = map[string]bool{
	"p95_latency_ms":     true,
	"p99_latency_ms":     true,
	"mean_latency_ms":    true,
	"throughput_rps":     true,
	"error_rate":         true,
	"cost":               true,
	"cpu_utilization":    true,
	"memory_utilization": true,
}

const errInvalidObjective = "invalid optimization.objective: must be one of p95_latency_ms, p99_latency_ms, mean_latency_ms, throughput_rps, error_rate, cost, cpu_utilization, memory_utilization (or recommended_config with optimization.batch)"

var errRecommendedConfigRequiresBatch = errors.New(`optimization.objective "recommended_config" is only valid when optimization.batch is set (batch mode)`)

func validateOptimizationObjective(objective string, batchSet bool) error {
	if objective == "" {
		return nil
	}
	if objective == ObjectiveRecommendedConfig {
		if !batchSet {
			return errRecommendedConfigRequiresBatch
		}
		return nil
	}
	if !allowedOptimizationObjectives[objective] {
		return errors.New(errInvalidObjective)
	}
	return nil
}
