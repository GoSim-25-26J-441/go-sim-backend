package http

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/scenario"
)

// Simulation-core batch scaling action enum names (JSON / proto).
const (
	BatchScalingActionScaleReplicas = "BATCH_SCALING_ACTION_SCALE_REPLICAS"
	BatchScalingActionScaleHosts    = "BATCH_SCALING_ACTION_SCALE_HOSTS"
)

// populateBatchFleetDefaults fills missing optimization.batch fleet fields from scenario_yaml.
// Caller must run normalizeBatchJSONForEngine first. Preserves explicit client values; returns an error
// when required fields cannot be inferred safely.
func populateBatchFleetDefaults(batch map[string]interface{}, scenarioYAML string) error {
	if batch == nil {
		return nil
	}
	s, err := scenario.ParseScenarioYAML([]byte(scenarioYAML))
	if err != nil {
		return fmt.Errorf("batch optimization: parse scenario_yaml: %w", err)
	}
	initialHosts := s.NodeCount()
	if initialHosts <= 0 {
		return fmt.Errorf("batch optimization: scenario defines no hosts or nodes; cannot set optimization.batch.min_hosts/max_hosts")
	}

	if _, ok := batch["max_hosts"]; !ok {
		batch["max_hosts"] = float64(initialHosts)
	}
	if _, ok := batch["min_hosts"]; !ok {
		batch["min_hosts"] = float64(1)
	}

	if err := validateBatchHostsRange(batch); err != nil {
		return err
	}

	minCores, minMem, inferErr := minHostResourcesFromScenario(s)
	if inferErr != nil {
		if _, has := batch["min_host_cpu_cores"]; !has {
			return fmt.Errorf("batch optimization: %w; set optimization.batch.min_host_cpu_cores explicitly", inferErr)
		}
		if _, has := batch["min_host_memory_gb"]; !has {
			return fmt.Errorf("batch optimization: %w; set optimization.batch.min_host_memory_gb explicitly", inferErr)
		}
	} else {
		if _, ok := batch["min_host_cpu_cores"]; !ok {
			batch["min_host_cpu_cores"] = minCores
		}
		if _, ok := batch["min_host_memory_gb"]; !ok {
			batch["min_host_memory_gb"] = minMem
		}
	}

	if err := validateBatchHostResourceFloors(batch); err != nil {
		return err
	}

	if batchAllowedActionsUnsetOrEmpty(batch) {
		batch["allowed_actions"] = []interface{}{
			BatchScalingActionScaleReplicas,
			BatchScalingActionScaleHosts,
		}
	}
	return nil
}

func minHostResourcesFromScenario(s scenario.Scenario) (minCores float64, minMemGB float64, err error) {
	if len(s.Hosts) == 0 {
		return 0, 0, fmt.Errorf("cannot infer per-host cpu/memory without a hosts array")
	}
	minCoresF := math.MaxFloat64
	minMemF := math.MaxFloat64
	for _, h := range s.Hosts {
		if h.Cores <= 0 {
			return 0, 0, fmt.Errorf("host %q has invalid cores for inference", h.ID)
		}
		mem := h.MemoryGB
		if mem <= 0 {
			mem = 16 // match scenario.ToHostsServices default when memory_gb omitted
		}
		minCoresF = math.Min(minCoresF, float64(h.Cores))
		minMemF = math.Min(minMemF, mem)
	}
	if minCoresF <= 0 || minCoresF == math.MaxFloat64 {
		return 0, 0, fmt.Errorf("cannot infer min_host_cpu_cores from scenario hosts")
	}
	if minMemF <= 0 || minMemF == math.MaxFloat64 {
		return 0, 0, fmt.Errorf("cannot infer min_host_memory_gb from scenario hosts")
	}
	return minCoresF, minMemF, nil
}

func batchAllowedActionsUnsetOrEmpty(batch map[string]interface{}) bool {
	v, ok := batch["allowed_actions"]
	if !ok || v == nil {
		return true
	}
	switch x := v.(type) {
	case []interface{}:
		return len(x) == 0
	default:
		return false
	}
}

func validateBatchHostsRange(batch map[string]interface{}) error {
	minH, minOK := jsonNumberToPositiveFloat(batch["min_hosts"])
	maxH, maxOK := jsonNumberToPositiveFloat(batch["max_hosts"])
	if minOK && maxOK && minH > maxH {
		return fmt.Errorf("batch optimization: min_hosts (%g) must be <= max_hosts (%g)", minH, maxH)
	}
	return nil
}

func validateBatchHostResourceFloors(batch map[string]interface{}) error {
	if c, ok := jsonNumberToPositiveFloat(batch["min_host_cpu_cores"]); !ok || c <= 0 {
		return fmt.Errorf("batch optimization: min_host_cpu_cores must be a positive number")
	}
	if m, ok := jsonNumberToPositiveFloat(batch["min_host_memory_gb"]); !ok || m <= 0 {
		return fmt.Errorf("batch optimization: min_host_memory_gb must be a positive number")
	}
	return nil
}

func jsonNumberToPositiveFloat(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
