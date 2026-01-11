package strategies

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type reverseDependency struct{}

func (reverseDependency) Kind() domain.AntiPatternKind { return domain.APReverseDependency }

func (reverseDependency) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	return suggestion.Suggestion{
		Kind:  det.Kind,
		Title: "Remove reverse dependency",
		Bullets: []string{
			"Backend depends on UI/frontend (wrong direction).",
			"Fix: UI should depend on backend, not the other way.",
			"Auto-fix: remove backend → UI dependency edge.",
		},
	}
}

func (reverseDependency) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if spec == nil || len(det.Nodes) < 2 {
		return false, nil
	}
	from := det.Nodes[0]
	to := det.Nodes[1]

	if ok, note := removeDependencyOnce(spec, from, to); ok {
		return true, []string{note}
	}
	return false, nil
}

func init() { suggestion.Register(reverseDependency{}) }
