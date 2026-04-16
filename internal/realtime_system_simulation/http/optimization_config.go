package http

import (
	"bytes"
	"encoding/json"
	"strings"
)

// OptimizationConfig mirrors simulation-core OptimizationConfig (JSON / proto).
// Keep in sync with proto/simulation/v1/simulation.proto; regenerate clients after proto changes.
// Batch is raw JSON so new batch fields pass through without backend churn.
type OptimizationConfig struct {
	// Objective: standard metrics, or recommended_config only with optimization.batch (rewritten to p95_latency_ms for the engine).
	Objective            string  `json:"objective,omitempty"`
	MaxIterations        int32   `json:"max_iterations,omitempty"`
	MaxEvaluations       int32   `json:"max_evaluations,omitempty"`
	StepSize             float64 `json:"step_size,omitempty"`
	EvaluationDurationMs int64   `json:"evaluation_duration_ms,omitempty"`
	Online               bool    `json:"online,omitempty"`
	TargetP95LatencyMs   float64 `json:"target_p95_latency_ms,omitempty"`
	ControlIntervalMs    int64   `json:"control_interval_ms,omitempty"`
	MinHosts             int32   `json:"min_hosts,omitempty"`
	MaxHosts             int32   `json:"max_hosts,omitempty"`
	// Scale-down gating (Phase 1): 0–1, 0 = off
	ScaleDownCPUUtilMax float64 `json:"scale_down_cpu_util_max,omitempty"`
	ScaleDownMemUtilMax float64 `json:"scale_down_mem_util_max,omitempty"`
	// Primary target (Phase 2)
	OptimizationTargetPrimary string  `json:"optimization_target_primary,omitempty"`
	TargetUtilHigh            float64 `json:"target_util_high,omitempty"`
	TargetUtilLow             float64 `json:"target_util_low,omitempty"`
	// Host scale-in (Phase 3): 0–1, 0 = host scale-in disabled
	ScaleDownHostCPUUtilMax float64 `json:"scale_down_host_cpu_util_max,omitempty"`
	// Online controller & wall-clock limits
	MaxControllerSteps   int32   `json:"max_controller_steps,omitempty"`
	MaxOnlineDurationMs  int64   `json:"max_online_duration_ms,omitempty"`
	AllowUnboundedOnline *bool   `json:"allow_unbounded_online,omitempty"`
	MaxNoopIntervals     int32   `json:"max_noop_intervals,omitempty"`
	LeaseTTLMs           int64   `json:"lease_ttl_ms,omitempty"`
	ScaleDownCooldownMs  int64   `json:"scale_down_cooldown_ms,omitempty"`
	HostDrainTimeoutMs   int64   `json:"host_drain_timeout_ms,omitempty"`
	MemoryHeadroomMB     float64 `json:"memory_headroom_mb,omitempty"`
	// When non-empty and online is false, simulation-core runs batch (beam) optimization instead of hill-climb.
	Batch json.RawMessage `json:"batch,omitempty"`
}

// UnmarshalOptimizationConfig parses optimization JSON for validation. Returns false if optJSON is empty or whitespace.
func UnmarshalOptimizationConfig(optJSON json.RawMessage) (OptimizationConfig, bool, error) {
	if len(bytes.TrimSpace(optJSON)) == 0 {
		return OptimizationConfig{}, false, nil
	}
	if strings.TrimSpace(string(optJSON)) == "null" {
		return OptimizationConfig{}, false, nil
	}
	var opt OptimizationConfig
	if err := json.Unmarshal(optJSON, &opt); err != nil {
		return OptimizationConfig{}, true, err
	}
	return opt, true, nil
}
