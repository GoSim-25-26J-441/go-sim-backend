package strategies

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type uiOrchestratorStrategy struct{}

func (uiOrchestratorStrategy) Kind() domain.AntiPatternKind { return domain.APUIOrchestrator }

func (uiOrchestratorStrategy) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	return suggestion.Suggestion{
		Kind:  det.Kind,
		Title: "Introduce BFF for UI",
		Bullets: []string{
			"UI calls multiple backend services directly.",
			"Fix: add a BFF (backend-for-frontend) or gateway so UI calls one endpoint.",
			"Auto-fix: insert BFF node and reroute UI → targets through BFF.",
		},
	}
}

func (uiOrchestratorStrategy) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if spec == nil || len(det.Nodes) < 2 {
		return false, nil
	}

	ui := det.Nodes[0]
	targets := det.Nodes[1:]

	bff := uniqueServiceName(spec, "bff")
	ensureService(spec, bff)

	changed := false
	var notes []string


	if ok, note := addDependencyIfMissing(spec, parser.YDependency{
		From: ui,
		To:   bff,
		Kind: "rest",
		Sync: true,
	}); ok {
		changed = true
		notes = append(notes, note)
	}


	for _, t := range targets {
		if ok, note := removeDependencyOnce(spec, ui, t); ok {
			changed = true
			notes = append(notes, note)
		}
		if ok, note := addDependencyIfMissing(spec, parser.YDependency{
			From: bff,
			To:   t,
			Kind: "rest",
			Sync: true,
		}); ok {
			changed = true
			notes = append(notes, note)
		}
	}

	return changed, notes
}

func init() { suggestion.Register(uiOrchestratorStrategy{}) }
