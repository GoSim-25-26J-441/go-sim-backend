package mapper

import (
	"testing"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

func TestToGraph_LegacyYAML_RespectsServiceTypeDatabase(t *testing.T) {
	y := `
services:
  - name: db-1
    type: database
  - name: web-ui
    type: service
`
	spec, err := parser.ParseYAMLString(y)
	if err != nil {
		t.Fatal(err)
	}
	g := ToGraph(spec)
	if g == nil || len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %#v", g)
	}
	dbID := idify(domain.NodeDB, "db-1")
	n := g.Nodes[dbID]
	if n == nil || n.Kind != domain.NodeDB {
		t.Fatalf("expected DATABASE node db-1, got %#v", n)
	}
}

func TestToGraph_NewStyle_RespectsClientType(t *testing.T) {
	y := `
services:
  - name: spa
    type: client
  - name: api
    type: service
dependencies:
  - from: spa
    to: api
    kind: rest
    sync: true
`
	spec, err := parser.ParseYAMLString(y)
	if err != nil {
		t.Fatal(err)
	}
	g := ToGraph(spec)
	spaID := idify(domain.NodeClient, "spa")
	if n := g.Nodes[spaID]; n == nil || n.Kind != domain.NodeClient {
		t.Fatalf("expected CLIENT spa, got %#v", n)
	}
}

func TestNormalizeYAMLSpecInPlace_UserTypeBecomesUserActor(t *testing.T) {
	y := `
services:
  - name: user-1
    type: user
  - name: api
    type: service
dependencies:
  - from: user-1
    to: api
    kind: rest
    sync: true
`
	spec, err := parser.ParseYAMLString(y)
	if err != nil {
		t.Fatal(err)
	}
	NormalizeYAMLSpecInPlace(spec)
	if spec.Services[0].Type != "user_actor" {
		t.Fatalf("expected user_actor, got %q", spec.Services[0].Type)
	}
	g := ToGraph(spec)
	id := idify(domain.NodeUserActor, "user-1")
	if n := g.Nodes[id]; n == nil || n.Kind != domain.NodeUserActor {
		t.Fatalf("expected USER_ACTOR node user-1, got %#v", n)
	}
}

func TestStripNodeNameRef(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SERVICE:foo", "foo"},
		{"service:foo", "foo"},
		{"DATABASE:db-1", "db-1"},
		{"CLIENT:web", "web"},
		{"API_GATEWAY:gw", "gw"},
	}
	for _, c := range cases {
		if got := StripNodeNameRef(c.in); got != c.want {
			t.Errorf("StripNodeNameRef(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
