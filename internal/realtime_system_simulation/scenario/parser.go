package scenario

import (
	"math"

	"gopkg.in/yaml.v3"
)

// Host represents a host entry in scenario YAML.
type Host struct {
	ID       string  `yaml:"id"`
	Cores    int     `yaml:"cores"`
	MemoryGB float64 `yaml:"memory_gb"`
}

// Service represents a service entry in scenario YAML.
type Service struct {
	ID       string  `yaml:"id"`
	Replicas int     `yaml:"replicas"`
	CPUCores float64 `yaml:"cpu_cores"`
	MemoryMB float64 `yaml:"memory_mb"`
}

// WorkloadArrival represents the arrival section of a workload entry (e.g. rate_rps for Poisson).
type WorkloadArrival struct {
	Type    string  `yaml:"type"`
	RateRPS float64 `yaml:"rate_rps"`
}

// WorkloadEntry represents one entry in the workload array (from, to, arrival).
type WorkloadEntry struct {
	From    string         `yaml:"from"`
	To      string         `yaml:"to"`
	Arrival WorkloadArrival `yaml:"arrival"`
}

// Scenario is the canonical parsed shape of scenario YAML (hosts, services, workload, optional nodes).
type Scenario struct {
	Nodes    int            `yaml:"nodes"`
	Hosts    []Host         `yaml:"hosts"`
	Services []Service      `yaml:"services"`
	Workload []WorkloadEntry `yaml:"workload"`
}

// ParseScenarioYAML unmarshals scenario YAML into Scenario. Returns an empty Scenario on parse error.
func ParseScenarioYAML(yamlContent []byte) (Scenario, error) {
	var s Scenario
	if err := yaml.Unmarshal(yamlContent, &s); err != nil {
		return Scenario{}, err
	}
	return s, nil
}

// VCPU returns total vCPU: sum of host cores if hosts present; else sum of replicas * cpu_cores per service.
func (s *Scenario) VCPU() float64 {
	var vcpu float64
	if len(s.Hosts) > 0 {
		for _, h := range s.Hosts {
			vcpu += float64(h.Cores)
		}
	} else {
		for _, svc := range s.Services {
			vcpu += float64(svc.Replicas) * svc.CPUCores
		}
	}
	return vcpu
}

// MemoryGB returns total memory in GB: when hosts have memory_gb set use their sum; otherwise from
// services (replicas * memory_mb / 1024) or fallback len(hosts)*16.
func (s *Scenario) MemoryGB() float64 {
	var memoryGB float64
	if len(s.Hosts) > 0 {
		for _, h := range s.Hosts {
			if h.MemoryGB > 0 {
				memoryGB += h.MemoryGB
			}
		}
		if memoryGB == 0 {
			if len(s.Services) > 0 {
				for _, svc := range s.Services {
					memoryGB += float64(svc.Replicas) * svc.MemoryMB / 1024
				}
			} else {
				memoryGB = float64(len(s.Hosts)) * 16
			}
		}
	} else {
		for _, svc := range s.Services {
			memoryGB += float64(svc.Replicas) * svc.MemoryMB / 1024
		}
	}
	return memoryGB
}

// NodeCount returns the number of nodes: Nodes if set > 0, else len(Hosts).
func (s *Scenario) NodeCount() int {
	if s.Nodes > 0 {
		return s.Nodes
	}
	return len(s.Hosts)
}

// RateRPS returns the first workload entry's arrival.rate_rps when present and > 0; otherwise 0.
// This is the intended request rate (e.g. Poisson 10 rps), not achieved throughput.
func (s *Scenario) RateRPS() float64 {
	for _, w := range s.Workload {
		if w.Arrival.RateRPS > 0 {
			return w.Arrival.RateRPS
		}
	}
	return 0
}

// ToHostsServices returns hosts and services as slice-of-maps for use in handlers (e.g. convert to gin.H).
// Host memory_gb uses the YAML value when set; otherwise defaults to 16.
func (s *Scenario) ToHostsServices() (hosts []map[string]interface{}, services []map[string]interface{}) {
	for _, h := range s.Hosts {
		memGB := 16.0
		if h.MemoryGB > 0 {
			memGB = h.MemoryGB
		}
		hosts = append(hosts, map[string]interface{}{
			"host_id":   h.ID,
			"cpu_cores": h.Cores,
			"memory_gb": memGB,
		})
	}
	for _, svc := range s.Services {
		services = append(services, map[string]interface{}{
			"service_id":  svc.ID,
			"replicas":    svc.Replicas,
			"cpu_cores":   svc.CPUCores,
			"memory_mb":   svc.MemoryMB,
		})
	}
	return hosts, services
}

// ToUtilisationPercent normalises a utilisation value to 0-100 scale. If the value is in (0, 1] it is
// treated as a ratio and multiplied by 100; otherwise it is assumed already a percentage. Result is
// clamped to [0, 100].
func ToUtilisationPercent(value float64) float64 {
	if value > 0 && value <= 1 {
		value = value * 100
	}
	return math.Max(0, math.Min(100, value))
}
