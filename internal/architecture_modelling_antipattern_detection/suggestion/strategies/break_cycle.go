package strategies

import (
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type breakCycle struct{}

func (breakCycle) Kind() domain.AntiPatternKind { return domain.APCycles }

func (breakCycle) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	names := det.Nodes
	loop := ""
	if len(names) >= 2 {
		loop = strings.Join(names, " → ") + " → " + names[0]
	}

	title := "Break cyclic dependency"
	if loop != "" {
		title = "Break cycle: " + loop
	}

	bullets := []string{
		"Services form a loop of dependencies.",
		"Fix: remove one edge in the cycle or convert it to async/event-based.",
		"Auto-fix: remove one dependency edge inside the loop.",
	}

	return suggestion.Suggestion{Kind: det.Kind, Title: title, Bullets: bullets}
}

func (breakCycle) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if spec == nil || len(det.Nodes) < 2 {
		return false, nil
	}

	for _, from := range det.Nodes {
		for _, to := range det.Nodes {
			if to == from {
				continue
			}
			if ok, note := removeDependencyOnce(spec, from, to); ok {
				return true, []string{note}
			}
		}
	}
	return false, nil
}

func init() { suggestion.Register(breakCycle{}) }
