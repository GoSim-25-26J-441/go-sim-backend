package rules

import (
	"os"
	"strconv"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"
)

type god struct{}
func (g god) Name() string { return "god_service" }

func deg(gra *domain.Graph, id string) int {
	return len(gra.Out[id]) + len(gra.In[id])
}

func (g god) Detect(gr *domain.Graph) ([]domain.Detection, error) {
	thr := 10
	if v := os.Getenv("DETECT_GOD_DEGREE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil { thr = n }
	}
	var out []domain.Detection
	for id, n := range gr.Nodes {
		if n.Kind != domain.NodeService { continue }
		if deg(gr, id) >= thr {
			out = append(out, domain.Detection{
				Kind: domain.APGodService, Severity: domain.SeverityMedium,
				Title: "God service (high centrality)",
				Summary: "Excessive incoming/outgoing dependencies",
				Nodes: []string{id},
				Evidence: domain.Attrs{"degree": deg(gr, id)},
			})
		}
	}
	return out, nil
}

func init(){ detection.Register(god{}) }
