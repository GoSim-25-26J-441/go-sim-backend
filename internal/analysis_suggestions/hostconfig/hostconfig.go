// Package hostconfig parses validated scenario host sizing from analysis-suggestions stored requests.
package hostconfig

import (
	"encoding/json"
)

// ScenarioHostConfig is normalized VM/host sizing derived from stored design + simulation inputs.
type ScenarioHostConfig struct {
	Nodes    int     `json:"nodes"`
	Cores    int     `json:"cores"`
	MemoryGB float64 `json:"memory_gb"`
}

type storedEnvelope struct {
	Design struct {
		PreferredVCPU     int     `json:"preferred_vcpu"`
		PreferredMemoryGB float64 `json:"preferred_memory_gb"`
	} `json:"design"`
	Simulation struct {
		Nodes int `json:"nodes"`
	} `json:"simulation"`
}

// ParseScenarioHostConfig unmarshals request JSON and validates required fields.
// Returns ok=false when nodes/cores/memory are missing or non-positive.
func ParseScenarioHostConfig(requestJSON []byte) (ScenarioHostConfig, bool) {
	var env storedEnvelope
	if err := json.Unmarshal(requestJSON, &env); err != nil {
		return ScenarioHostConfig{}, false
	}
	nodes := env.Simulation.Nodes
	cores := env.Design.PreferredVCPU
	mem := env.Design.PreferredMemoryGB
	if nodes <= 0 || cores <= 0 || mem <= 0 {
		return ScenarioHostConfig{}, false
	}
	return ScenarioHostConfig{
		Nodes:    nodes,
		Cores:    cores,
		MemoryGB: mem,
	}, true
}

type canonicalHostConfig struct {
	Nodes    int     `json:"nodes"`
	Cores    int     `json:"cores"`
	MemoryGB float64 `json:"memory_gb"`
}

// CanonicalJSON returns deterministic JSON for hashing valid configs.
func CanonicalJSON(cfg ScenarioHostConfig) string {
	b, err := json.Marshal(canonicalHostConfig{
		Nodes:    cfg.Nodes,
		Cores:    cfg.Cores,
		MemoryGB: cfg.MemoryGB,
	})
	if err != nil {
		return ""
	}
	return string(b)
}
