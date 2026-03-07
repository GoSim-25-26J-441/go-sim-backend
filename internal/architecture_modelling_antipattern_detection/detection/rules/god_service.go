package rules

import (
	"os"
	"strconv"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type god struct{}

func (g god) Name() string { return "god_service" }

func isDatastoreLike(n *domain.Node) bool {
	if n == nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(n.ID))
	return s == "database" ||
		strings.Contains(s, "database") ||
		strings.HasSuffix(s, "-db") ||
		strings.Contains(s, " db")
}

func degreeSvcOnly(gr *domain.Graph, id string) int {
	isSvc := func(nodeID string) bool {
		n, ok := gr.Nodes[nodeID]
		return ok && n.Kind == domain.NodeService && !isDatastoreLike(n)
	}

	cnt := 0
	for _, e := range gr.Out[id] {
		if e == nil || e.Kind != domain.EdgeCalls {
			continue
		}
		if isSvc(e.To) {
			cnt++
		}
	}
	for _, e := range gr.In[id] {
		if e == nil || e.Kind != domain.EdgeCalls {
			continue
		}
		if isSvc(e.From) {
			cnt++
		}
	}
	return cnt
}

func (g god) Detect(gr *domain.Graph) ([]domain.Detection, error) {
	thr := 4
	if v := os.Getenv("DETECT_GOD_DEGREE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			thr = n
		}
	}

	var out []domain.Detection
	for id, n := range gr.Nodes {
		if n == nil || n.Kind != domain.NodeService {
			continue
		}
		if isDatastoreLike(n) {
			continue
		}

		d := degreeSvcOnly(gr, id)
		if d >= thr {
			out = append(out, domain.Detection{
				Kind:     domain.APGodService,
				Severity: domain.SeverityMedium,
				Title:    "God service (high centrality)",
				Summary:  "Service has unusually high incoming/outgoing dependencies",
				Nodes:    []string{id},
				Evidence: domain.Attrs{"degree": d, "threshold": thr},
			})
		}
	}
	return out, nil
}

func init() { detection.Register(god{}) }
