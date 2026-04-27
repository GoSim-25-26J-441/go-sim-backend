package amg_apd_scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	parsed, err := ParseScenarioDocYAML([]byte(yamlStr))
	if err != nil {
		t.Fatalf("ParseScenarioDocYAML: %v", err)
	}
	if err := ValidateScenarioDraft(parsed); err != nil {
		t.Fatalf("ValidateScenarioDraft: %v", err)
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
	sc, _, err := GenerateFromAMGAPDYAML(amg)
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
	sc, _, err := GenerateFromAMGAPDYAML(amg)
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

// Real AMG/APD diagrams use services[].name, not id.
func TestGenerateFromAMGAPD_NameOnlyServices_RealShape(t *testing.T) {
	amg := []byte(`services:
  - name: web-ui
    type: service
  - name: customer-bff
    type: service
dependencies:
  - from: web-ui
    to: customer-bff
    kind: rest
    sync: true
configs:
  slo:
    target_rps: 700
`)
	yamlStr, err := GenerateScenarioYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseScenarioDocYAML([]byte(yamlStr))
	if err != nil {
		t.Fatalf("ParseScenarioDocYAML: %v", err)
	}
	if err := ValidateScenarioDraft(parsed); err != nil {
		t.Fatalf("ValidateScenarioDraft: %v", err)
	}
	if strings.Contains(yamlStr, "<nil>") {
		t.Fatalf("generated YAML must not contain literal <nil> service id")
	}
	if !strings.Contains(yamlStr, "web-ui") || !strings.Contains(yamlStr, "customer-bff") {
		t.Fatalf("expected web-ui and customer-bff in output, got:\n%s", yamlStr)
	}
	sc, _, err := GenerateFromAMGAPDYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Workload) != 1 || sc.Workload[0].Arrival.RateRPS != 700 {
		t.Fatalf("workload rate: want 700, got %+v", sc.Workload)
	}
	foundDown := false
	for _, s := range sc.Services {
		for _, ep := range s.Endpoints {
			for _, d := range ep.Downstream {
				if strings.HasPrefix(d.To, "customer-bff:") && strings.Contains(d.To, "/") {
					foundDown = true
				}
			}
		}
	}
	if !foundDown {
		t.Fatalf("expected downstream to customer-bff:/read (or similar), got %#v", sc.Services)
	}
}

func TestGenerateFromAMGAPD_MissingServiceIDAndName_Error(t *testing.T) {
	amg := []byte(`services:
  - type: service
dependencies: []
`)
	_, err := GenerateScenarioYAML(amg)
	if err == nil {
		t.Fatal("expected error for service missing id and name")
	}
	if strings.Contains(err.Error(), "<nil>") {
		t.Fatalf("error must not mention <nil> as id: %v", err)
	}
	_, _, err2 := GenerateFromAMGAPDYAML(amg)
	if err2 == nil {
		t.Fatal("expected error from GenerateFromAMGAPDYAML")
	}
}

func TestGenerateFromAMGAPD_DuplicateServiceName_Error(t *testing.T) {
	amg := []byte(`services:
  - name: dup-svc
    type: service
  - name: dup-svc
    type: service
dependencies: []
`)
	_, _, err := GenerateFromAMGAPDYAML(amg)
	if err == nil {
		t.Fatal("expected duplicate service error")
	}
	if !strings.Contains(err.Error(), "duplicate AMG/APD service name/id") {
		t.Fatalf("unexpected error: %v", err)
	}
}
