package scenario

import (
	"testing"
)

func TestParseScenarioYAML_Valid(t *testing.T) {
	t.Run("hosts only", func(t *testing.T) {
		yaml := []byte(`
hosts:
  - id: host-1
    cores: 4
  - id: host-2
    cores: 4
`)
		s, err := ParseScenarioYAML(yaml)
		if err != nil {
			t.Fatalf("ParseScenarioYAML: %v", err)
		}
		if got := s.VCPU(); got != 8 {
			t.Errorf("VCPU() = %v, want 8", got)
		}
		if got := s.MemoryGB(); got != 32 {
			t.Errorf("MemoryGB() = %v, want 32 (fallback 2*16)", got)
		}
		if got := s.NodeCount(); got != 2 {
			t.Errorf("NodeCount() = %v, want 2", got)
		}
	})

	t.Run("services only", func(t *testing.T) {
		yaml := []byte(`
services:
  - id: svc-1
    replicas: 2
    cpu_cores: 2
    memory_mb: 1024
  - id: svc-2
    replicas: 1
    cpu_cores: 4
    memory_mb: 2048
`)
		s, err := ParseScenarioYAML(yaml)
		if err != nil {
			t.Fatalf("ParseScenarioYAML: %v", err)
		}
		// 2*2 + 1*4 = 8 vCPU
		if got := s.VCPU(); got != 8 {
			t.Errorf("VCPU() = %v, want 8", got)
		}
		// 2*1024/1024 + 1*2048/1024 = 2 + 2 = 4 GB
		if got := s.MemoryGB(); got != 4 {
			t.Errorf("MemoryGB() = %v, want 4", got)
		}
		if got := s.NodeCount(); got != 0 {
			t.Errorf("NodeCount() = %v, want 0", got)
		}
	})

	t.Run("hosts and services", func(t *testing.T) {
		yaml := []byte(`
hosts:
  - id: h1
    cores: 4
  - id: h2
    cores: 4
services:
  - id: api
    replicas: 2
    cpu_cores: 1
    memory_mb: 512
`)
		s, err := ParseScenarioYAML(yaml)
		if err != nil {
			t.Fatalf("ParseScenarioYAML: %v", err)
		}
		if got := s.VCPU(); got != 8 {
			t.Errorf("VCPU() = %v, want 8", got)
		}
		// from services: 2*512/1024 = 1 GB
		if got := s.MemoryGB(); got != 1 {
			t.Errorf("MemoryGB() = %v, want 1", got)
		}
		if got := s.NodeCount(); got != 2 {
			t.Errorf("NodeCount() = %v, want 2", got)
		}
	})

	t.Run("top-level nodes", func(t *testing.T) {
		yaml := []byte(`
nodes: 5
hosts:
  - id: h1
    cores: 2
`)
		s, err := ParseScenarioYAML(yaml)
		if err != nil {
			t.Fatalf("ParseScenarioYAML: %v", err)
		}
		if got := s.NodeCount(); got != 5 {
			t.Errorf("NodeCount() = %v, want 5 (from nodes)", got)
		}
	})

	t.Run("hosts with memory_gb", func(t *testing.T) {
		yaml := []byte(`
hosts:
  - id: host-1
    cores: 4
    memory_gb: 16
`)
		s, err := ParseScenarioYAML(yaml)
		if err != nil {
			t.Fatalf("ParseScenarioYAML: %v", err)
		}
		if got := s.MemoryGB(); got != 16 {
			t.Errorf("MemoryGB() = %v, want 16 (from host memory_gb)", got)
		}
		hosts, _ := s.ToHostsServices()
		if len(hosts) != 1 {
			t.Fatalf("len(hosts) = %v", len(hosts))
		}
		if hosts[0]["memory_gb"] != 16.0 {
			t.Errorf("hosts[0].memory_gb = %v, want 16", hosts[0]["memory_gb"])
		}
	})

	t.Run("workload rate_rps", func(t *testing.T) {
		yaml := []byte(`
hosts:
  - id: h1
    cores: 4
workload:
  - from: client
    to: svc1:/test
    arrival:
      type: poisson
      rate_rps: 10
`)
		s, err := ParseScenarioYAML(yaml)
		if err != nil {
			t.Fatalf("ParseScenarioYAML: %v", err)
		}
		if got := s.RateRPS(); got != 10 {
			t.Errorf("RateRPS() = %v, want 10", got)
		}
	})
}

func TestParseScenarioYAML_EdgeCases(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		yaml := []byte(``)
		s, err := ParseScenarioYAML(yaml)
		if err != nil {
			t.Fatalf("ParseScenarioYAML: %v", err)
		}
		if got := s.VCPU(); got != 0 {
			t.Errorf("VCPU() = %v, want 0", got)
		}
		if got := s.MemoryGB(); got != 0 {
			t.Errorf("MemoryGB() = %v, want 0", got)
		}
		if got := s.NodeCount(); got != 0 {
			t.Errorf("NodeCount() = %v, want 0", got)
		}
	})

	t.Run("invalid YAML", func(t *testing.T) {
		yaml := []byte(`hosts: [ invalid`)
		_, err := ParseScenarioYAML(yaml)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})
}

func TestToHostsServices(t *testing.T) {
	yaml := []byte(`
hosts:
  - id: host-1
    cores: 4
services:
  - id: api
    replicas: 2
    cpu_cores: 1
    memory_mb: 512
`)
	s, err := ParseScenarioYAML(yaml)
	if err != nil {
		t.Fatalf("ParseScenarioYAML: %v", err)
	}
	hosts, services := s.ToHostsServices()
	if len(hosts) != 1 {
		t.Errorf("len(hosts) = %v, want 1", len(hosts))
	}
	if len(services) != 1 {
		t.Errorf("len(services) = %v, want 1", len(services))
	}
	if hosts[0]["host_id"] != "host-1" || hosts[0]["cpu_cores"] != 4 {
		t.Errorf("hosts[0] = %v", hosts[0])
	}
	// memory_gb is float64 when set from default (16.0)
	if mg, ok := hosts[0]["memory_gb"].(float64); !ok || mg != 16 {
		t.Errorf("hosts[0].memory_gb = %v (type %T), want 16", hosts[0]["memory_gb"], hosts[0]["memory_gb"])
	}
	if services[0]["service_id"] != "api" || services[0]["replicas"] != 2 {
		t.Errorf("services[0] = %v", services[0])
	}
}

func TestToUtilisationPercent(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  float64
	}{
		{"ratio 0.72 -> 72", 0.72, 72},
		{"already 72 -> 72", 72, 72},
		{"zero", 0, 0},
		{"one -> 100", 1, 100},
		{"ratio 0.61 -> 61", 0.61, 61},
		{"negative clamped to 0", -10, 0},
		{"over 100 clamped", 150, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToUtilisationPercent(tt.value)
			if got != tt.want {
				t.Errorf("ToUtilisationPercent(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
