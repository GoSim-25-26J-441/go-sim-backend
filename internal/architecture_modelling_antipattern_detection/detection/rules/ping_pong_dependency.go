package rules

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type pingPong struct{}

func (p pingPong) Name() string { return "ping_pong_dependency" }

func (p pingPong) Detect(g *domain.Graph) ([]domain.Detection, error) {
	isSvc := func(id string) bool {
		n, ok := g.Nodes[id]
		return ok && n != nil && n.Kind == domain.NodeService
	}

	has := map[string]bool{}
	for from, edges := range g.Out {
		for _, e := range edges {
			if e == nil || e.Kind != domain.EdgeCalls {
				continue
			}
			if !isSvc(from) || !isSvc(e.To) {
				continue
			}
			has[from+"|"+e.To] = true
		}
	}

	var out []domain.Detection
	seen := map[string]bool{}

	for k := range has {
		var a, b string
		for i := 0; i < len(k); i++ {
			if k[i] == '|' {
				a = k[:i]
				b = k[i+1:]
				break
			}
		}
		if a == "" || b == "" {
			continue
		}
		if !has[b+"|"+a] {
			continue
		}

		u1, u2 := a, b
		if u2 < u1 {
			u1, u2 = u2, u1
		}
		key := u1 + "|" + u2
		if seen[key] {
			continue
		}
		seen[key] = true

		out = append(out, domain.Detection{
			Kind:     domain.APPingPongDependency,
			Severity: domain.SeverityMedium,
			Title:    "Ping-pong dependency",
			Summary:  "Two services depend on each other (mutual calls)",
			Nodes:    []string{a, b},
			Evidence: domain.Attrs{"a": a, "b": b},
		})
	}

	return out, nil
}

func init() { detection.Register(pingPong{}) }
