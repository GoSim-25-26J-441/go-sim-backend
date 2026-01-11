package rules

import (
	"os"
	"strconv"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type tight struct{}

func (t tight) Name() string { return "tight_coupling" }

func edgeWeight(e *domain.Edge) int {
	if e == nil {
		return 1
	}
	if e.Attrs != nil {
		if c, ok := e.Attrs["count"].(int); ok && c > 0 {
			return c
		}
		if f, ok := e.Attrs["count"].(float64); ok && f > 0 {
			return int(f)
		}
	}
	return 1
}

func edgeIsSync(e *domain.Edge) bool {
	if e == nil {
		return true
	}
	if e.Attrs != nil {
		if b, ok := e.Attrs["sync"].(bool); ok {
			return b
		}
	}
	return true
}

func countSync(gr *domain.Graph, from, to string) (cnt, totalOut int) {
	for _, e := range gr.Out[from] {
		if e == nil || e.Kind != domain.EdgeCalls {
			continue
		}
		if !edgeIsSync(e) {
			continue
		}
		w := edgeWeight(e)
		if e.To == to {
			cnt += w
		}
		totalOut += w
	}
	return
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (t tight) Detect(g *domain.Graph) ([]domain.Detection, error) {
	minBidir := 1
	ratio := 0.7

	if v := os.Getenv("DETECT_TIGHT_COUPLING_MIN_BIDIR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minBidir = n
		}
	}
	if v := os.Getenv("DETECT_TIGHT_COUPLING_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			ratio = f
		}
	}

	var out []domain.Detection
	seen := map[string]bool{}

	isSvc := func(id string) bool {
		n, ok := g.Nodes[id]
		return ok && n != nil && n.Kind == domain.NodeService
	}

	for _, a := range g.Nodes {
		if a == nil || a.Kind != domain.NodeService {
			continue
		}
		for _, e := range g.Out[a.ID] {
			if e == nil || e.Kind != domain.EdgeCalls || !edgeIsSync(e) {
				continue
			}
			if !isSvc(e.To) {
				continue
			}

			fwd := a.ID + "|" + e.To
			rev := e.To + "|" + a.ID
			if seen[fwd] || seen[rev] {
				continue
			}

			ab, aOut := countSync(g, a.ID, e.To)
			ba, bOut := countSync(g, e.To, a.ID)

			if ab >= minBidir && ba >= minBidir {
				ra := float64(ab) / float64(max(1, aOut))
				rb := float64(ba) / float64(max(1, bOut))
				if ra >= ratio && rb >= ratio {
					out = append(out, domain.Detection{
						Kind:     domain.APTightCoupling,
						Severity: domain.SeverityHigh,
						Title:    "Tight coupling (synchronous mutual dependency)",
						Summary:  "Services rely heavily on each other via synchronous calls",
						Nodes:    []string{a.ID, e.To},
						Evidence: domain.Attrs{
							"ab": ab, "ba": ba,
							"ra": ra, "rb": rb,
							"min_bidir": minBidir,
							"ratio":     ratio,
						},
					})
				}
			}

			seen[fwd] = true
			seen[rev] = true
		}
	}

	return out, nil
}

func init() { detection.Register(tight{}) }
