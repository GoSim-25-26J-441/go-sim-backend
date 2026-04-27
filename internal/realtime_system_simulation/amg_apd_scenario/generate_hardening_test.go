package amg_apd_scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// End-to-end checks for the trimmed real-sample shape (services[].name, topics/datastores empty).
func TestGenerate_RealSampleTrim_EndToEnd(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "real_sample_trim.yaml"))
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
	if MetadataSchemaVersion(parsed) != "0.2.0" {
		t.Fatalf("metadata.schema_version: want 0.2.0, got %#v", parsed.Metadata)
	}
	if strings.Contains(yamlStr, "<nil>") || strings.Contains(yamlStr, "nil>") {
		t.Fatalf("generated YAML must not contain nil artifacts:\n%s", yamlStr)
	}

	sc, _, err := GenerateFromAMGAPDYAML(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Workload) != 1 || sc.Workload[0].Arrival.RateRPS != 700 {
		t.Fatalf("workload rate: got %+v", sc.Workload)
	}

	wantNames := []string{"web-ui", "customer-bff", "catalog-service", "profile-service-db", "bff"}
	byID := map[string]ServiceDoc{}
	for _, s := range sc.Services {
		if s.ID == "" || strings.Contains(s.ID, "nil") {
			t.Fatalf("invalid service id: %#v", s)
		}
		byID[s.ID] = s
	}
	for _, w := range wantNames {
		if _, ok := byID[w]; !ok {
			t.Fatalf("missing normalized service %q, have %+v", w, sc.Services)
		}
	}

	if sc.Workload[0].To != "bff:/ingress" {
		t.Fatalf("expected workload to bff:/ingress (api_gateway entry), got %q", sc.Workload[0].To)
	}

	endpointIndex := map[string]map[string]struct{}{}
	for _, s := range sc.Services {
		paths := make(map[string]struct{})
		for _, ep := range s.Endpoints {
			paths[ep.Path] = struct{}{}
		}
		endpointIndex[s.ID] = paths
	}

	for id, s := range byID {
		switch id {
		case "profile-service-db":
			if s.Kind != "database" || s.Role != "datastore" {
				t.Fatalf("database %q: want kind database role datastore, got kind=%q role=%q", id, s.Kind, s.Role)
			}
			if s.Model != "db_latency" {
				t.Fatalf("database %q: want model db_latency, got %q", id, s.Model)
			}
			for _, need := range []string{"/query", "/write"} {
				if _, ok := endpointIndex[id][need]; !ok {
					t.Fatalf("database service %q missing %s", id, need)
				}
			}
		case "bff":
			if s.Kind != "api_gateway" || s.Role != "ingress" {
				t.Fatalf("bff: want kind api_gateway role ingress, got kind=%q role=%q", s.Kind, s.Role)
			}
			if s.Model != "cpu" {
				t.Fatalf("bff: want cpu model, got %q", s.Model)
			}
			if s.Routing == nil || s.Routing.Strategy != "least_queue" {
				t.Fatalf("bff: want service routing least_queue, got %#v", s.Routing)
			}
			if _, ok := endpointIndex[id]["/ingress"]; !ok {
				t.Fatalf("api_gateway %q missing /ingress", id)
			}
		case "customer-bff":
			if s.Kind == "api_gateway" {
				t.Fatalf("customer-bff must not be classified as api_gateway from name alone; got api_gateway")
			}
			if s.Kind != "service" || s.Role != "" {
				t.Fatalf("customer-bff: want plain service (not ingress api_gateway), got kind=%q role=%q", s.Kind, s.Role)
			}
			if s.Model != "cpu" {
				t.Fatalf("service %q: want cpu model, got %q", id, s.Model)
			}
			for _, need := range []string{"/create", "/read", "/update", "/delete"} {
				if _, ok := endpointIndex[id][need]; !ok {
					t.Fatalf("service %q missing %s", id, need)
				}
			}
		case "web-ui":
			if s.Kind != "service" || s.Role != "ingress" {
				t.Fatalf("web-ui: want kind service role ingress, got kind=%q role=%q", s.Kind, s.Role)
			}
			if s.Model != "cpu" {
				t.Fatalf("service %q: want cpu model, got %q", id, s.Model)
			}
			for _, need := range []string{"/create", "/read", "/update", "/delete"} {
				if _, ok := endpointIndex[id][need]; !ok {
					t.Fatalf("service %q missing %s", id, need)
				}
			}
		default:
			if s.Kind != "service" || s.Role != "" {
				t.Fatalf("%q: want plain service, got kind=%q role=%q", id, s.Kind, s.Role)
			}
			if s.Model != "cpu" {
				t.Fatalf("service %q: want cpu model, got %q", id, s.Model)
			}
			for _, need := range []string{"/create", "/read", "/update", "/delete"} {
				if _, ok := endpointIndex[id][need]; !ok {
					t.Fatalf("service %q missing %s", id, need)
				}
			}
		}
	}

	wl := sc.Workload[0].To
	colon := strings.Index(wl, ":")
	if colon <= 0 || colon == len(wl)-1 {
		t.Fatalf("invalid workload target %q", wl)
	}
	wlSvc, wlPath := wl[:colon], wl[colon+1:]
	if _, ok := endpointIndex[wlSvc][wlPath]; !ok {
		t.Fatalf("workload references missing endpoint %s:%s", wlSvc, wlPath)
	}

	for _, s := range sc.Services {
		for _, ep := range s.Endpoints {
			for _, d := range ep.Downstream {
				if !strings.Contains(d.To, ":") {
					t.Fatalf("downstream %q must be service:path", d.To)
				}
				parts := strings.SplitN(d.To, ":", 2)
				tgtSvc, tgtPath := parts[0], parts[1]
				paths, ok := endpointIndex[tgtSvc]
				if !ok {
					t.Fatalf("downstream references unknown service %q", tgtSvc)
				}
				if _, ok := paths[tgtPath]; !ok {
					t.Fatalf("downstream references missing endpoint %s:%s", tgtSvc, tgtPath)
				}
			}
		}
	}

	if os.Getenv("AMG_DEBUG_SCENARIO") != "" {
		t.Log(yamlStr)
	}
}

func TestPickEntrypoint_PrefersAppOverIsolatedDB(t *testing.T) {
	amg := []byte(`services:
  - name: db
    type: database
  - name: app
    type: service
dependencies:
  - from: app
    to: db
    kind: rest
    sync: true
configs:
  slo:
    target_rps: 1
`)
	sc, _, err := GenerateFromAMGAPDYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sc.Workload[0].To, "app:") {
		t.Fatalf("expected entrypoint app, got workload to %q", sc.Workload[0].To)
	}
}

func TestDownstream_ToDatabaseUsesQueryPath(t *testing.T) {
	amg := []byte(`services:
  - id: gw
    type: api_gateway
  - id: db
    type: database
dependencies:
  - from: gw
    to: db
    kind: rest
    sync: true
configs:
  slo:
    target_rps: 1
`)
	sc, _, err := GenerateFromAMGAPDYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range sc.Services {
		for _, ep := range s.Endpoints {
			for _, d := range ep.Downstream {
				if d.To == "db:/query" && d.Kind == "db" && d.Mode == "sync" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected downstream to database at db:/query")
	}
}

func TestDatabaseVsAPIGateway_ModelAndEndpoints(t *testing.T) {
	amg := []byte(`services:
  - id: db
    type: database
  - id: gw
    type: api_gateway
dependencies: []
configs:
  slo:
    target_rps: 1
`)
	sc, _, err := GenerateFromAMGAPDYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sc.Services {
		switch s.ID {
		case "db":
			if s.Kind != "database" || s.Model != "db_latency" {
				t.Fatalf("db: want kind database model db_latency, got kind=%q model=%q", s.Kind, s.Model)
			}
		case "gw":
			if s.Kind != "api_gateway" || s.Role != "ingress" {
				t.Fatalf("gw: want kind api_gateway role ingress, got kind=%q role=%q", s.Kind, s.Role)
			}
			if s.Model != "cpu" {
				t.Fatalf("gw: want cpu model, got %q", s.Model)
			}
			var hasIngress bool
			for _, ep := range s.Endpoints {
				if ep.Path == "/ingress" {
					hasIngress = true
				}
			}
			if !hasIngress {
				t.Fatal("api_gateway missing /ingress")
			}
		}
	}
}

func TestDuplicateAfterNormalization(t *testing.T) {
	amg := []byte(`services:
  - name: Customer BFF
    type: service
  - name: Customer-BFF
    type: service
dependencies: []
`)
	_, _, err := GenerateFromAMGAPDYAML(amg)
	if err == nil {
		t.Fatal("expected duplicate error after normalization")
	}
	if !strings.Contains(err.Error(), "duplicate AMG/APD service name/id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownstreamCall_CallCountMean(t *testing.T) {
	amg := []byte(`services:
  - id: x
    type: api_gateway
  - id: y
    type: service
dependencies:
  - from: x
    to: y
    kind: grpc
    sync: true
configs:
  slo:
    target_rps: 1
`)
	sc, _, err := GenerateFromAMGAPDYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sc.Services {
		for _, ep := range s.Endpoints {
			for _, d := range ep.Downstream {
				if d.CallCountMean != 1 || d.Probability != 1 {
					t.Fatalf("want call_count_mean and probability 1, got %#v", d)
				}
				if d.Kind != "grpc" {
					t.Fatalf("grpc dependency: want downstream kind grpc, got %q", d.Kind)
				}
			}
		}
	}
}
