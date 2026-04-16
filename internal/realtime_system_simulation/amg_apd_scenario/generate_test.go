package amg_apd_scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	simconfig "github.com/GoSim-25-26J-441/simulation-core/pkg/config"
)

func TestGenerateFromAMGAPD_Sample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "minimal_amg_apd.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	yamlStr, err := GenerateScenarioYAML(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := simconfig.ParseScenarioYAML([]byte(yamlStr)); err != nil {
		t.Fatalf("parser validation: %v", err)
	}
	if !strings.Contains(yamlStr, "/create") || !strings.Contains(yamlStr, "/read") {
		t.Fatalf("expected CRUD paths in generated YAML")
	}
	if !strings.Contains(yamlStr, "orders:/read") && !strings.Contains(yamlStr, "orders:") {
		t.Fatalf("expected downstream service:endpoint form")
	}
}

func TestGenerateFromAMGAPD_TargetRPSFromSLO(t *testing.T) {
	amg := []byte(`services:
  - id: a
    type: api_gateway
dependencies: []
configs:
  slo:
    target_rps: 99
`)
	sc, err := GenerateFromAMGAPDYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Workload) != 1 || sc.Workload[0].Arrival.RateRPS != 99 {
		t.Fatalf("workload rate: got %+v", sc.Workload)
	}
}

func TestGenerateFromAMGAPD_DownstreamTargetFormat(t *testing.T) {
	amg := []byte(`services:
  - id: gw
    type: api_gateway
  - id: svc1
    type: service
dependencies:
  - from: gw
    to: svc1
    sync: true
    kind: rest
configs:
  slo:
    target_rps: 10
`)
	sc, err := GenerateFromAMGAPDYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range sc.Services {
		for _, ep := range s.Endpoints {
			for _, d := range ep.Downstream {
				if strings.HasPrefix(d.To, "svc1:") && strings.Contains(d.To, "/") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected downstream to use service:/endpoint, got %#v", sc.Services)
	}
}
