package amg_apd_scenario

import "fmt"

// GenerationOptions configures AMG/APD scenario generation beyond diagram structure.
type GenerationOptions struct {
	Hosts []HostDoc
}

func defaultScenarioHosts() []HostDoc {
	return []HostDoc{
		{ID: "host-1", Cores: 8, MemoryGB: 32},
		{ID: "host-2", Cores: 8, MemoryGB: 32},
		{ID: "host-3", Cores: 8, MemoryGB: 32},
	}
}

func effectiveHosts(opts GenerationOptions) []HostDoc {
	if len(opts.Hosts) > 0 {
		out := make([]HostDoc, len(opts.Hosts))
		copy(out, opts.Hosts)
		return out
	}
	return defaultScenarioHosts()
}

// HostDocsFromCounts builds deterministic host-1..host-N entries for placement metadata.
func HostDocsFromCounts(nodes, cores int, memoryGB float64) []HostDoc {
	if nodes <= 0 || cores <= 0 || memoryGB <= 0 {
		return nil
	}
	hosts := make([]HostDoc, 0, nodes)
	for i := 1; i <= nodes; i++ {
		hosts = append(hosts, HostDoc{
			ID:       fmt.Sprintf("host-%d", i),
			Cores:    cores,
			MemoryGB: memoryGB,
		})
	}
	return hosts
}
