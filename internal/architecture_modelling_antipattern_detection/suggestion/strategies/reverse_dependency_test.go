package strategies

import (
	"strings"
	"testing"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

func TestReverseDependencyApply_LegacyCalls_FlipsDirection(t *testing.T) {
	y := `
services:
  - name: orders-api
    calls:
      - to: web-ui
  - name: web-ui
`
	spec, err := parser.ParseYAMLString(y)
	if err != nil {
		t.Fatal(err)
	}
	var det domain.Detection
	det.Nodes = []string{"SERVICE:orders-api", "SERVICE:web-ui"}

	var s reverseDependency
	changed, notes := s.Apply(spec, nil, det)
	if !changed {
		t.Fatal("expected Apply to change legacy calls spec")
	}
	if len(notes) < 2 {
		t.Fatalf("expected flip notes, got %v", notes)
	}

	orders := -1
	ui := -1
	for i := range spec.Services {
		switch strings.ToLower(strings.TrimSpace(spec.Services[i].Name)) {
		case "orders-api":
			orders = i
		case "web-ui":
			ui = i
		}
	}
	if orders < 0 || ui < 0 {
		t.Fatal("services missing after apply")
	}
	if len(spec.Services[orders].Calls) != 0 {
		t.Fatalf("orders-api should have no outbound calls, got %+v", spec.Services[orders].Calls)
	}
	found := false
	for _, c := range spec.Services[ui].Calls {
		if strings.EqualFold(strings.TrimSpace(c.To), "orders-api") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("web-ui should call orders-api, calls=%+v", spec.Services[ui].Calls)
	}
}

func TestReverseDependencyApply_Dependencies_FlipsDirection(t *testing.T) {
	y := `
services:
  - name: orders-api
  - name: web-ui
dependencies:
  - from: orders-api
    to: web-ui
    kind: rest
    sync: true
`
	spec, err := parser.ParseYAMLString(y)
	if err != nil {
		t.Fatal(err)
	}
	var det domain.Detection
	det.Nodes = []string{"SERVICE:orders-api", "SERVICE:web-ui"}

	var s reverseDependency
	changed, notes := s.Apply(spec, nil, det)
	if !changed {
		t.Fatal("expected Apply to change dependencies spec")
	}
	t.Logf("notes: %v", notes)

	if len(spec.Dependencies) != 1 {
		t.Fatalf("expected one dependency, got %+v", spec.Dependencies)
	}
	d := spec.Dependencies[0]
	if !strings.EqualFold(d.From, "web-ui") || !strings.EqualFold(d.To, "orders-api") {
		t.Fatalf("expected web-ui → orders-api, got %s → %s", d.From, d.To)
	}
}
