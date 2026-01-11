package rules

import (
	"os"
	"strconv"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type uiOrchestrator struct{}

func (u uiOrchestrator) Name() string { return "ui_orchestrator" }

func (u uiOrchestrator) Detect(g *domain.Graph) ([]domain.Detection, error) {
	minOut := 2
	if v := os.Getenv("DETECT_UI_ORCH_MIN_OUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minOut = n
		}
	}

	isSvc := func(id string) bool {
		n, ok := g.Nodes[id]
		return ok && n != nil && n.Kind == domain.NodeService
	}

	var out []domain.Detection
	for id, n := range g.Nodes {
		if n == nil || n.Kind != domain.NodeService {
			continue
		}
		if !isUIName(id) {
			continue
		}

		targetsSet := map[string]bool{}
		syncTargets := 0

		for _, e := range g.Out[id] {
			if e == nil || e.Kind != domain.EdgeCalls {
				continue
			}
			if !isSvc(e.To) {
				continue
			}
			if e.To == id {
				continue
			}
			if !targetsSet[e.To] {
				targetsSet[e.To] = true
				if edgeIsSync(e) {
					syncTargets++
				}
			}
		}

		if len(targetsSet) >= minOut {
			targets := make([]string, 0, len(targetsSet))
			for t := range targetsSet {
				targets = append(targets, t)
			}
			nodes := append([]string{id}, targets...)

			out = append(out, domain.Detection{
				Kind:     domain.APUIOrchestrator,
				Severity: domain.SeverityMedium,
				Title:    "UI orchestrator",
				Summary:  "UI directly orchestrates multiple backend services",
				Nodes:    nodes,
				Evidence: domain.Attrs{
					"ui":           id,
					"targets":      len(targetsSet),
					"sync_targets": syncTargets,
					"min_out":      minOut,
				},
			})
		}
	}
	return out, nil
}

func init() { detection.Register(uiOrchestrator{}) }
