package strategies

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type pingPongDependency struct{}

func (pingPongDependency) Kind() domain.AntiPatternKind { return domain.APPingPongDependency }

func (pingPongDependency) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	sug := suggestion.Suggestion{
		Kind:  det.Kind,
		Title: "Reduce ping-pong dependency",
		Bullets: []string{
			"Two services call each other (back-and-forth).",
			"Fix: remove one direction so they no longer mutually depend, or replace with events.",
			"Auto-fix: removes one call/dependency — prefers dropping backend → UI if detected, else the second → first leg, else the other direction.",
		},
	}
	if len(det.Nodes) < 2 {
		return sug
	}
	a, b := det.Nodes[0], det.Nodes[1]
	sug.PreviewFrom, sug.PreviewTo = a, b
	seq := pingPongRemovalSequence(a, b)
	if len(seq) > 0 {
		first := seq[0]
		sug.PreviewRemoveLeg = pingPongPreviewRemoveLeg(a, b, first[0], first[1])
	}
	return sug
}

func (pingPongDependency) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if spec == nil || len(det.Nodes) < 2 {
		return false, nil
	}
	a, b := det.Nodes[0], det.Nodes[1]
	for _, pair := range pingPongRemovalSequence(a, b) {
		if ok, note := removeDependencyOrLegacyCall(spec, pair[0], pair[1]); ok {
			return true, []string{note}
		}
	}
	return false, nil
}

func init() { suggestion.Register(pingPongDependency{}) }
