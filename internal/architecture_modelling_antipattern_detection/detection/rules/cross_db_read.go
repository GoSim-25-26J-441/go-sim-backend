package rules

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type crossRead struct{}
func (c crossRead) Name() string { return "cross_db_read" }

func (c crossRead) Detect(g *domain.Graph) ([]domain.Detection, error) {
	owner := map[string]string{}
	for _, e := range g.Edges {
		if e.Kind == domain.EdgeWrites {
			if n, ok := g.Nodes[e.From]; ok {
				owner[e.To] = n.Name
			}
		}
	}
	var out []domain.Detection
	for i, e := range g.Edges {
		if e.Kind != domain.EdgeReads { continue }
		dbOwner := owner[e.To]
		if dbOwner == "" { continue }
		reader := g.Nodes[e.From].Name
		if reader != dbOwner {
			out = append(out, domain.Detection{
				Kind: domain.APCrossDBRead, Severity: domain.SeverityMedium,
				Title: "Cross-DB read",
				Summary: "Service reads DB owned by another service",
				Nodes: []string{e.From, e.To},
				Edges: []int{i},
				Evidence: domain.Attrs{"owner": dbOwner, "reader": reader},
			})
		}
	}
	return out, nil
}

func init(){ detection.Register(crossRead{}) }
