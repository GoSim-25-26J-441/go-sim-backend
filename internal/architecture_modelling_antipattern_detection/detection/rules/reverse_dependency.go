package rules

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type reverseDep struct{}

func (r reverseDep) Name() string { return "reverse_dependency" }

func (r reverseDep) Detect(g *domain.Graph) ([]domain.Detection, error) {
	isSvc := func(id string) bool {
		n, ok := g.Nodes[id]
		return ok && n != nil && n.Kind == domain.NodeService
	}

	var out []domain.Detection
	for from, edges := range g.Out {
		if !isSvc(from) {
			continue
		}
		fromIsUI := isUIName(from)

		for _, e := range edges {
			if e == nil || e.Kind != domain.EdgeCalls {
				continue
			}
			if !isSvc(e.To) {
				continue
			}
			toIsUI := isUIName(e.To)

			if !fromIsUI && toIsUI {
				out = append(out, domain.Detection{
					Kind:     domain.APReverseDependency,
					Severity: domain.SeverityHigh,
					Title:    "Reverse dependency (backend → UI)",
					Summary:  "Backend service depends on the UI/frontend layer",
					Nodes:    []string{from, e.To},
					Evidence: domain.Attrs{"from": from, "to": e.To},
				})
			}
		}
	}
	return out, nil
}

func init() { detection.Register(reverseDep{}) }
