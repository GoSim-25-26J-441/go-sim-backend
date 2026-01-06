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
	a := ""
	b := ""
	if len(det.Nodes) >= 2 {
		a = nodeName(g, det.Nodes[0])
		b = nodeName(g, det.Nodes[1])
	}

	title := "Reduce tight coupling"
	if a != "" && b != "" {
		title = fmt.Sprintf("Reduce tight coupling (%s ↔ %s)", a, b)
	}

	rpmAB, epsAB, okAB := findCallEdgeBetween(g, a, b)
	rpmBA, epsBA, okBA := findCallEdgeBetween(g, b, a)

	bullets := []string{
		fmt.Sprintf("Detected strong two-way dependency between %s and %s.", a, b),
	}

	if okAB {
		bullets = append(bullets, fmt.Sprintf("%s → %s: %d endpoints, ~%d rpm.", a, b, epsAB, rpmAB))
	}
	if okBA {
		bullets = append(bullets, fmt.Sprintf("%s → %s: %d endpoints, ~%d rpm.", b, a, epsBA, rpmBA))
	}

	bullets = append(bullets,
		"Fix idea: remove bidirectional calls where possible — keep one direction as events (pub/sub) or use a single owning service.",
		"Fix idea: extract shared logic into a separate service/module so changes don’t ripple between both services.",
		"Auto-fix preview: we will shrink call edges by keeping 1 endpoint and reducing rate_per_min on "+a+" → "+b+" and "+b+" → "+a+" (then re-run detection).",
	)

	return suggestion.Suggestion{
		Kind:    det.Kind,
		Title:   title,
		Bullets: bullets,
	}
}


func (tightCoupling) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if len(det.Nodes) < 2 {
		return false, nil
	}
	a := nodeName(g, det.Nodes[0])
	b := nodeName(g, det.Nodes[1])

	changedA, noteA := shrinkEdge(spec, a, b)
	changedB, noteB := shrinkEdge(spec, b, a)

	changed := changedA || changedB
	var notes []string
	if changedA {
		notes = append(notes, noteA)
	}
	if changedB {
		notes = append(notes, noteB)
	}
	return changed, notes
}

func shrinkEdge(spec *parser.YSpec, from, to string) (bool, string) {
	svc := findService(spec, from)
	if svc == nil {
		return false, ""
	}
	for i := range svc.Calls {
		if !stringsEqualFold(svc.Calls[i].To, to) {
			continue
		}

		oldEps := len(svc.Calls[i].Endpoints)
		oldRpm := svc.Calls[i].RatePerMin

		if len(svc.Calls[i].Endpoints) > 1 {
			svc.Calls[i].Endpoints = svc.Calls[i].Endpoints[:1]
		}
		if svc.Calls[i].RatePerMin > 0 {
			svc.Calls[i].RatePerMin = maxInt(1, svc.Calls[i].RatePerMin/5)
		}

		return true, fmt.Sprintf("Reduced coupling on %s → %s: endpoints %d → %d, rate_per_min %d → %d",
			from, to, oldEps, len(svc.Calls[i].Endpoints), oldRpm, svc.Calls[i].RatePerMin)
	}
	return false, ""
}

func init() { suggestion.Register(tightCoupling{}) }
