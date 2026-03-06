package strategies

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type pingPongDependency struct{}

func (pingPongDependency) Kind() domain.AntiPatternKind { return domain.APPingPongDependency }

func (pingPongDependency) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	return suggestion.Suggestion{
		Kind:  det.Kind,
		Title: "Reduce ping-pong dependency",
		Bullets: []string{
			"Two services call each other (back-and-forth).",
			"Fix: keep one direction async or remove one direction.",
			"Auto-fix: set sync=false on one direction.",
		},
	}
}

func (pingPongDependency) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if spec == nil || len(det.Nodes) < 2 {
		return false, nil
	}
	a := det.Nodes[0]
	b := det.Nodes[1]

	if idx := findDepIndex(spec, b, a); idx >= 0 {
		if ok, note := setDependencySync(spec, b, a, false); ok {
			return true, []string{note}
		}
	}
	if idx := findDepIndex(spec, a, b); idx >= 0 {
		if ok, note := setDependencySync(spec, a, b, false); ok {
			return true, []string{note}
		}
	}
	return false, nil
}

func init() { suggestion.Register(pingPongDependency{}) }
