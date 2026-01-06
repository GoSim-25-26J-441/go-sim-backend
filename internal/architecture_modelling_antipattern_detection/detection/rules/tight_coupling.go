package rules

import (
	"os"
	"strconv"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type tight struct{}
func (t tight) Name() string { return "tight_coupling" }

func countCalls(g *domain.Graph, from, to string) (cnt, totalOut int) {
	for _, e := range g.Out[from] {
		if e.Kind != domain.EdgeCalls { continue }
		if e.To == to { if c, ok := e.Attrs["count"].(int); ok { cnt += c } else { cnt++ } }
		if c, ok := e.Attrs["count"].(int); ok { totalOut += c } else { totalOut++ }
	}
	return
}

func (t tight) Detect(g *domain.Graph) ([]domain.Detection, error) {
	minBidir := 2
	ratio := 0.6
	if v := os.Getenv("DETECT_TIGHT_COUPLING_MIN_BIDIR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil { minBidir = n }
	}
	if v := os.Getenv("DETECT_TIGHT_COUPLING_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil { ratio = f }
	}
	var out []domain.Detection
	seen := map[string]bool{}
	for _, a := range g.Nodes {
		if a.Kind != domain.NodeService { continue }
		for _, e := range g.Out[a.ID] {
			if e.Kind != domain.EdgeCalls { continue }
			bid := e.To + "|" + a.ID
			fwd := a.ID + "|" + e.To
			if seen[fwd] || seen[bid] { continue }
			if b, ok := g.Nodes[e.To]; !ok || b.Kind != domain.NodeService { continue }

			ab, aOut := countCalls(g, a.ID, e.To)
			ba, bOut := countCalls(g, e.To, a.ID)
			if ab >= minBidir && ba >= minBidir {
				ra := float64(ab) / float64(max(1, aOut))
				rb := float64(ba) / float64(max(1, bOut))
				if ra >= ratio && rb >= ratio {
					out = append(out, domain.Detection{
						Kind: domain.APTightCoupling, Severity: domain.SeverityHigh,
						Title: "Tightly coupled services",
						Summary: "High bi-directional call concentration",
						Nodes: []string{a.ID, e.To},
						Evidence: domain.Attrs{"ab": ab, "ba": ba, "ra": ra, "rb": rb},
					})
				}
			}
			seen[fwd] = true; seen[bid] = true
		}
	}
	return out, nil
}
func max(a,b int) int { if a>b {return a}; return b }

func init(){ detection.Register(tight{}) }
