package strategies

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type reverseDependency struct{}

func (reverseDependency) Kind() domain.AntiPatternKind { return domain.APReverseDependency }

func (reverseDependency) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	from, to := "", ""
	if len(det.Nodes) >= 2 {
		from, to = det.Nodes[0], det.Nodes[1]
	}
	return suggestion.Suggestion{
		Kind:        det.Kind,
		Title:       "Fix reverse dependency",
		PreviewFrom: from,
		PreviewTo:   to,
		Bullets: []string{
			"Backend depends on UI/frontend (wrong direction).",
			"Fix: dependency should go UI → backend (presentation calls APIs), not backend → UI.",
			"Auto-fix: flips the dependency — removes backend → UI and adds UI → backend (works for top-level dependencies and legacy services[].calls).",
		},
	}
}

func (reverseDependency) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if spec == nil || len(det.Nodes) < 2 {
		return false, nil
	}
	from := det.Nodes[0]
	to := det.Nodes[1]

	if ok, notes := flipDependencyDirection(spec, from, to); ok {
		return true, notes
	}
	if ok, notes := flipLegacyCallDirection(spec, from, to); ok {
		return true, notes
	}
	return false, nil
}

func init() { suggestion.Register(reverseDependency{}) }
