package strategies

import (
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type tightCoupling struct{}

func (tightCoupling) Kind() domain.AntiPatternKind { return domain.APTightCoupling }

func (tightCoupling) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	a, b := "", ""
	if len(det.Nodes) >= 2 {
		a, b = det.Nodes[0], det.Nodes[1]
	}

	title := "Reduce tight coupling"
	if a != "" && b != "" {
		title = fmt.Sprintf("Reduce tight coupling (%s ↔ %s)", a, b)
	}

	bullets := []string{
		"Two services heavily depend on each other (often sync both ways).",
		"Fix: make one direction async or remove one direction.",
		"Auto-fix: set sync=false on one direction.",
	}

	return suggestion.Suggestion{Kind: det.Kind, Title: title, Bullets: bullets}
}

func (tightCoupling) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
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

func init() { suggestion.Register(tightCoupling{}) }
