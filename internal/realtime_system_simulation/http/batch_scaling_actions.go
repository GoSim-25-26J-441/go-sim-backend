package http

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/scenario"
)

// Batch scaling action enum values and short debug names must stay aligned with
// simulation/v1 BatchScalingAction in simulation-core (values 1–12 = core service + host actions).
// Queue/topic concurrency actions 13–16 are omitted from the BFF default effective set until the
// contract explicitly enables them in this backend.
const (
	batchScalingActionHostMin = int32(7)
	batchScalingActionHostMax = int32(12)
)

// coreBatchScalingDebugNames indexes simulation-core enum value -> stable short debugging label (1-based index 0..11).
var coreBatchScalingDebugNames = []string{
	"SERVICE_REPLICA_SCALE_UP",
	"SERVICE_REPLICA_SCALE_DOWN",
	"SERVICE_CPU_INCREASE",
	"SERVICE_CPU_DECREASE",
	"SERVICE_MEMORY_INCREASE",
	"SERVICE_MEMORY_DECREASE",
	"HOST_SCALE_OUT",
	"HOST_SCALE_IN",
	"HOST_CPU_INCREASE",
	"HOST_CPU_DECREASE",
	"HOST_MEMORY_INCREASE",
	"HOST_MEMORY_DECREASE",
}

// legacyProtoStyleStrings maps historical JSON enum strings seen in clients to numeric codes.
// Extend when simulation-core adds aliases; keep in sync with proto JSON names where applicable.
var legacyProtoStyleStrings = map[string]int32{
	"BATCH_SCALING_ACTION_SCALE_REPLICAS": 1,
	"BATCH_SCALING_ACTION_SCALE_HOSTS":    7,
}

// NormalizedAllowedActionsDebug is stored under metadata.batch.normalized_allowed_actions for debugging / export.
type NormalizedAllowedActionsDebug struct {
	Numeric          []int32  `json:"numeric"`
	Names            []string `json:"names"`
	OmittedInRequest bool     `json:"omitted_in_request"`
}

// defaultEffectiveCoreBatchScalingNumerics returns the effective default action set (1–12) when the client
// omits optimization.batch.allowed_actions — used only for debug metadata, not serialized to the engine payload.
func defaultEffectiveCoreBatchScalingNumerics() []int32 {
	out := make([]int32, len(coreBatchScalingDebugNames))
	for i := range coreBatchScalingDebugNames {
		out[i] = int32(i + 1)
	}
	return out
}

func shortNameForBatchScalingNumeric(n int32) string {
	if n >= 1 && int(n) <= len(coreBatchScalingDebugNames) {
		return coreBatchScalingDebugNames[n-1]
	}
	return fmt.Sprintf("ACTION_%d", n)
}

func parseAllowedActionsToNumerics(batch map[string]interface{}) ([]int32, error) {
	v, ok := batch["allowed_actions"]
	if !ok || v == nil {
		return nil, nil
	}
	var raw []interface{}
	switch x := v.(type) {
	case []interface{}:
		raw = x
	default:
		return nil, fmt.Errorf("allowed_actions must be an array")
	}
	out := make([]int32, 0, len(raw))
	for _, el := range raw {
		n, ok := parseSingleAllowedActionElement(el)
		if !ok {
			return nil, fmt.Errorf("allowed_actions: unsupported element type %T", el)
		}
		out = append(out, n)
	}
	return out, nil
}

func parseSingleAllowedActionElement(el interface{}) (int32, bool) {
	switch x := el.(type) {
	case float64:
		return int32(x), true
	case int:
		return int32(x), true
	case int32:
		return x, true
	case int64:
		return int32(x), true
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 32); err == nil {
			return int32(n), true
		}
		if n, ok := legacyProtoStyleStrings[s]; ok {
			return n, true
		}
		// Try uppercase normalization
		if n, ok := legacyProtoStyleStrings[strings.ToUpper(s)]; ok {
			return n, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func numericsToDebugNames(nums []int32) []string {
	names := make([]string, len(nums))
	for i, n := range nums {
		names[i] = shortNameForBatchScalingNumeric(n)
	}
	return names
}

// buildNormalizedAllowedActionsDebug builds the compact debug payload for metadata.
// When the client omitted allowed_actions, Numeric/Names reflect the effective core default (1–12) with OmittedInRequest=true.
// When explicit, Numeric/Names reflect the parsed request (normalized to numeric + short names).
func buildNormalizedAllowedActionsDebug(batch map[string]interface{}, allowedActionsOmitted bool) (*NormalizedAllowedActionsDebug, error) {
	if allowedActionsOmitted {
		nums := defaultEffectiveCoreBatchScalingNumerics()
		return &NormalizedAllowedActionsDebug{
			Numeric:          nums,
			Names:            numericsToDebugNames(nums),
			OmittedInRequest: true,
		}, nil
	}
	nums, err := parseAllowedActionsToNumerics(batch)
	if err != nil {
		return nil, err
	}
	return &NormalizedAllowedActionsDebug{
		Numeric:          nums,
		Names:            numericsToDebugNames(nums),
		OmittedInRequest: false,
	}, nil
}

func allowedActionsSliceContainsHostBand(nums []int32) bool {
	for _, n := range nums {
		if n >= batchScalingActionHostMin && n <= batchScalingActionHostMax {
			return true
		}
	}
	return false
}

func initialHostCountFromScenarioYAML(scenarioYAML string) (int, error) {
	s, err := scenario.ParseScenarioYAML([]byte(scenarioYAML))
	if err != nil {
		return 0, err
	}
	n := s.NodeCount()
	if n <= 0 {
		return 0, fmt.Errorf("no hosts")
	}
	return n, nil
}

// hostScalingConfigured reports fleet intent that typically requires host scaling actions (7–12).
func hostScalingConfigured(batch map[string]interface{}, initialHosts int) bool {
	maxH, okMax := jsonNumberToPositiveFloat(batch["max_hosts"])
	minH, okMin := jsonNumberToPositiveFloat(batch["min_hosts"])
	if okMax && maxH > float64(initialHosts) {
		return true
	}
	if okMin && minH < float64(initialHosts) {
		return true
	}
	return false
}

func batchHostScalingWithoutHostActionsWarning(
	batch map[string]interface{},
	scenarioYAML string,
	allowedActionsOmitted bool,
	explicitNumerics []int32,
) string {
	if allowedActionsOmitted {
		return ""
	}
	initial, err := initialHostCountFromScenarioYAML(scenarioYAML)
	if err != nil || initial <= 0 {
		return ""
	}
	if !hostScalingConfigured(batch, initial) {
		return ""
	}
	if allowedActionsSliceContainsHostBand(explicitNumerics) {
		return ""
	}
	return "batch optimization: host scaling configured but host actions are not enabled"
}
