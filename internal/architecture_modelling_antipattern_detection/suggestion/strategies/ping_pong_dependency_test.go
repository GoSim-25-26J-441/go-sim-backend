package strategies

import (
	"strings"
	"testing"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

func TestPingPongApply_Legacy_RemovesOneCall(t *testing.T) {
	y := `
services:
  - name: cart-svc
    calls:
      - to: catalog-svc
  - name: catalog-svc
    calls:
      - to: cart-svc
`
	spec, err := parser.ParseYAMLString(y)
	if err != nil {
		t.Fatal(err)
	}
	var det domain.Detection
	det.Nodes = []string{"SERVICE:cart-svc", "SERVICE:catalog-svc"}

	var p pingPongDependency
	changed, notes := p.Apply(spec, nil, det)
	if !changed {
		t.Fatal("expected Apply to remove one mutual call")
	}
	t.Log(notes)

	// One direction should be gone; the other should remain.
	cartCalls := 0
	catCalls := 0
	for _, svc := range spec.Services {
		n := strings.ToLower(strings.TrimSpace(svc.Name))
		switch n {
		case "cart-svc":
			cartCalls = len(svc.Calls)
		case "catalog-svc":
			catCalls = len(svc.Calls)
		}
	}
	if cartCalls+catCalls != 1 {
		t.Fatalf("expected exactly one remaining call edge, cart=%d catalog=%d", cartCalls, catCalls)
	}
}
