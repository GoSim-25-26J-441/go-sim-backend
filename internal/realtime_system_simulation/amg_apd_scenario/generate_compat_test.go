package amg_apd_scenario

import (
	"strings"
	"testing"
)

// TestGenerateScenario_V2FieldsParseAndValidate asserts scenario-v2-shaped output parses locally and
// downstream calls carry Mode/Kind/Probability where the generator sets them.
func TestGenerateScenario_V2FieldsParseAndValidate(t *testing.T) {
	amg := []byte(`services:
  - name: gw
    type: api_gateway
  - name: app
    type: service
  - name: store
    type: database
dependencies:
  - from: gw
    to: app
    kind: rest
    sync: true
  - from: app
    to: store
    kind: grpc
    sync: false
configs:
  slo:
    target_rps: 50
`)
	yamlStr, err := GenerateScenarioYAML(amg)
	if err != nil {
		t.Fatalf("GenerateScenarioYAML: %v", err)
	}
	parsed, err := ParseScenarioDocYAML([]byte(yamlStr))
	if err != nil {
		t.Fatalf("ParseScenarioDocYAML: %v", err)
	}

	if MetadataSchemaVersion(parsed) != "0.2.0" {
		t.Fatalf("metadata.schema_version: want 0.2.0, got %#v", parsed.Metadata)
	}

	if len(parsed.Hosts) == 0 {
		t.Fatal("expected hosts")
	}
	if len(parsed.Services) == 0 || len(parsed.Workload) == 0 {
		t.Fatalf("expected services and workload, got services=%d workload=%d", len(parsed.Services), len(parsed.Workload))
	}

	for _, needle := range []string{"hosts:", "services:", "workload:", "endpoints:", "downstream:"} {
		if !strings.Contains(yamlStr, needle) {
			t.Fatalf("generated YAML missing %q", needle)
		}
	}

	var gw *ServiceDoc
	for i := range parsed.Services {
		if parsed.Services[i].ID == "gw" {
			gw = &parsed.Services[i]
			break
		}
	}
	if gw == nil || gw.Kind != "api_gateway" || gw.Role != "ingress" {
		t.Fatalf("gw service: want api_gateway/ingress, got %#v", gw)
	}

	var sawAsyncDB bool
	var sawSyncRest bool
	for _, s := range parsed.Services {
		for _, ep := range s.Endpoints {
			for _, d := range ep.Downstream {
				if d.Probability != 1 || d.CallCountMean != 1 {
					t.Fatalf("downstream %#v: want probability and call_count_mean 1", d)
				}
				if strings.HasPrefix(d.To, "store:") && d.Mode == "async" && d.Kind == "db" {
					sawAsyncDB = true
				}
				if strings.HasPrefix(d.To, "app:") && d.Mode == "sync" && d.Kind == "rest" {
					sawSyncRest = true
				}
			}
		}
	}
	if !sawAsyncDB {
		t.Fatal("expected async downstream with kind db to database")
	}
	if !sawSyncRest {
		t.Fatal("expected sync downstream with kind rest to app")
	}

	if parsed.Workload[0].Arrival.RateRPS != 50 {
		t.Fatalf("workload rate: want 50, got %v", parsed.Workload[0].Arrival.RateRPS)
	}
}
