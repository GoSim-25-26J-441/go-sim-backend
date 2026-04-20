package amg_apd_scenario

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// MetadataSchemaVersion returns metadata.schema_version after unmarshaling (string or numeric-safe).
func MetadataSchemaVersion(doc *ScenarioDoc) string {
	if doc == nil || doc.Metadata == nil {
		return ""
	}
	switch v := doc.Metadata["schema_version"].(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// ScenarioDoc is a backend-local DTO for scenario-v2 YAML aligned with simulation-core field names.
// Only fields the AMG/APD generator emits are defined; simulation-core remains the authority for validation.
type ScenarioDoc struct {
	Metadata map[string]any `yaml:"metadata,omitempty"`
	Hosts    []HostDoc      `yaml:"hosts"`
	Services []ServiceDoc   `yaml:"services"`
	Workload []WorkloadDoc  `yaml:"workload"`
}

// HostDoc describes a placement host.
type HostDoc struct {
	ID       string  `yaml:"id"`
	Cores    int     `yaml:"cores"`
	MemoryGB float64 `yaml:"memory_gb"`
}

// ServiceDoc describes a simulated service (v2-oriented shape).
type ServiceDoc struct {
	ID        string         `yaml:"id"`
	Kind      string         `yaml:"kind,omitempty"`
	Role      string         `yaml:"role,omitempty"`
	Replicas  int            `yaml:"replicas"`
	Model     string         `yaml:"model"`
	CPUCores  float64        `yaml:"cpu_cores"`
	MemoryMB  float64        `yaml:"memory_mb"`
	Scaling   *ScalingDoc    `yaml:"scaling,omitempty"`
	Behavior  *BehaviorDoc   `yaml:"behavior,omitempty"`
	Routing   *RoutingDoc    `yaml:"routing,omitempty"`
	Endpoints []EndpointDoc  `yaml:"endpoints"`
	Placement map[string]any `yaml:"placement,omitempty"`
}

// ScalingDoc mirrors horizontal / vertical scaling flags.
type ScalingDoc struct {
	Horizontal     bool `yaml:"horizontal"`
	VerticalCPU    bool `yaml:"vertical_cpu"`
	VerticalMemory bool `yaml:"vertical_memory"`
}

// RoutingDoc is used for api_gateway-style services.
type RoutingDoc struct {
	Strategy string `yaml:"strategy"`
}

// BehaviorDoc holds queue or topic behavior when applicable.
type BehaviorDoc struct {
	Queue *QueueBehaviorDoc `yaml:"queue,omitempty"`
	Topic *TopicBehaviorDoc `yaml:"topic,omitempty"`
}

// QueueBehaviorDoc models queue service behavior.
type QueueBehaviorDoc struct {
	Capacity            int        `yaml:"capacity"`
	ConsumerConcurrency int        `yaml:"consumer_concurrency"`
	ConsumerTarget      string     `yaml:"consumer_target"`
	DeliveryLatencyMs   LatencyDoc `yaml:"delivery_latency_ms"`
	AckTimeoutMs        int        `yaml:"ack_timeout_ms"`
	MaxRedeliveries     int        `yaml:"max_redeliveries"`
	DropPolicy          string     `yaml:"drop_policy"`
}

// TopicBehaviorDoc models topic service behavior.
type TopicBehaviorDoc struct {
	Partitions        int                  `yaml:"partitions"`
	Capacity          int                  `yaml:"capacity"`
	DeliveryLatencyMs LatencyDoc           `yaml:"delivery_latency_ms"`
	Subscribers       []TopicSubscriberDoc `yaml:"subscribers"`
}

// TopicSubscriberDoc is a topic subscriber entry.
type TopicSubscriberDoc struct {
	Name            string `yaml:"name"`
	ConsumerTarget  string `yaml:"consumer_target"`
	ConsumerGroup   string `yaml:"consumer_group"`
	DropPolicy      string `yaml:"drop_policy"`
	MaxRedeliveries int    `yaml:"max_redeliveries"`
}

// EndpointDoc is a single HTTP-style endpoint on a service.
type EndpointDoc struct {
	Path            string          `yaml:"path"`
	MeanCPUMs       float64         `yaml:"mean_cpu_ms"`
	CPUSigmaMs      float64         `yaml:"cpu_sigma_ms"`
	DefaultMemoryMB float64         `yaml:"default_memory_mb"`
	NetLatencyMs    LatencyDoc      `yaml:"net_latency_ms"`
	Downstream      []DownstreamDoc `yaml:"downstream,omitempty"`
}

// DownstreamDoc is a downstream hop from an endpoint.
type DownstreamDoc struct {
	To            string     `yaml:"to"`
	Mode          string     `yaml:"mode"`
	Kind          string     `yaml:"kind"`
	Probability   float64    `yaml:"probability"`
	CallCountMean float64    `yaml:"call_count_mean"`
	CallLatencyMs LatencyDoc `yaml:"call_latency_ms"`
}

// LatencyDoc is a simple latency distribution.
type LatencyDoc struct {
	Mean  float64 `yaml:"mean"`
	Sigma float64 `yaml:"sigma"`
}

// WorkloadDoc is an external workload pattern.
type WorkloadDoc struct {
	From    string     `yaml:"from"`
	To      string     `yaml:"to"`
	Arrival ArrivalDoc `yaml:"arrival"`
}

// ArrivalDoc describes arrival process (Poisson rate).
type ArrivalDoc struct {
	Type    string  `yaml:"type"`
	RateRPS float64 `yaml:"rate_rps"`
}

// ParseScenarioDocYAML unmarshals scenario YAML into ScenarioDoc for read-only metrics/spec extraction.
// It is not authoritative validation; callers should still use simulation-core HTTP validate for runs.
func ParseScenarioDocYAML(b []byte) (*ScenarioDoc, error) {
	var d ScenarioDoc
	if err := yaml.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse scenario YAML: %w", err)
	}
	return &d, nil
}

// ValidateScenarioDraft performs non-authoritative structural checks on a generated draft.
func ValidateScenarioDraft(doc *ScenarioDoc) error {
	if doc == nil {
		return fmt.Errorf("scenario is nil")
	}
	if len(doc.Services) == 0 {
		return fmt.Errorf("scenario has no services")
	}
	if len(doc.Workload) == 0 {
		return fmt.Errorf("scenario has no workload")
	}
	seen := make(map[string]struct{})
	for _, s := range doc.Services {
		id := strings.TrimSpace(s.ID)
		if id == "" || strings.Contains(id, "<nil>") {
			return fmt.Errorf("service has empty or invalid id")
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("duplicate service id %q", id)
		}
		seen[id] = struct{}{}
	}
	endpointIndex := map[string]map[string]struct{}{}
	for _, s := range doc.Services {
		paths := make(map[string]struct{})
		for _, ep := range s.Endpoints {
			p := strings.TrimSpace(ep.Path)
			if p != "" {
				paths[p] = struct{}{}
			}
		}
		endpointIndex[s.ID] = paths
	}
	for _, w := range doc.Workload {
		if w.To == "" || !strings.Contains(w.To, ":") {
			return fmt.Errorf("workload has invalid to %q", w.To)
		}
		parts := strings.SplitN(w.To, ":", 2)
		svc, path := parts[0], parts[1]
		paths, ok := endpointIndex[svc]
		if !ok {
			return fmt.Errorf("workload references unknown service %q", svc)
		}
		if _, ok := paths[path]; !ok {
			return fmt.Errorf("workload references missing endpoint %s:%s", svc, path)
		}
	}
	for _, s := range doc.Services {
		for _, ep := range s.Endpoints {
			for _, d := range ep.Downstream {
				if d.To == "" || !strings.Contains(d.To, ":") {
					return fmt.Errorf("downstream has invalid to %q", d.To)
				}
				parts := strings.SplitN(d.To, ":", 2)
				tgtSvc, tgtPath := parts[0], parts[1]
				paths, ok := endpointIndex[tgtSvc]
				if !ok {
					return fmt.Errorf("downstream references unknown service %q", tgtSvc)
				}
				if _, ok := paths[tgtPath]; !ok {
					return fmt.Errorf("downstream references missing endpoint %s:%s", tgtSvc, tgtPath)
				}
			}
		}
	}
	return nil
}
