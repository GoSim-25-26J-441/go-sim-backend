package strategies

import (
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type chattyCalls struct{}

func (chattyCalls) Kind() domain.AntiPatternKind { return domain.APChattyCalls }

func (chattyCalls) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	from := ""
	to := ""
	if len(det.Nodes) >= 2 {
		from = nodeName(g, det.Nodes[0])
		to = nodeName(g, det.Nodes[1])
	}
	title := "Reduce chatty calls"
	if from != "" && to != "" {
		title = fmt.Sprintf("Reduce chatty calls (%s → %s)", from, to)
	}
	return suggestion.Suggestion{
		Kind:  det.Kind,
		Title: title,
		Bullets: []string{
			"Too many small calls are happening between these services.",
			"Batch requests (send fewer, bigger requests) instead of per-item calls.",
			"Add caching where possible and avoid calling for each item in a loop.",
		},
	}
}

func (chattyCalls) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if len(det.Nodes) < 2 {
		return false, nil
	}
	from := nodeName(g, det.Nodes[0])
	to := nodeName(g, det.Nodes[1])

	svc := findService(spec, from)
	if svc == nil {
		return false, nil
	}

	for i := range svc.Calls {
		if svc.Calls[i].To == "" {
			continue
		}
		if !stringsEqualFold(svc.Calls[i].To, to) {
			continue
		}

		oldRpm := svc.Calls[i].RatePerMin
		oldPerItem := svc.Calls[i].PerItem

		// heuristic fix: disable per-item and reduce RPM
		svc.Calls[i].PerItem = false
		if svc.Calls[i].RatePerMin > 0 {
			svc.Calls[i].RatePerMin = maxInt(1, svc.Calls[i].RatePerMin/10)
		}

		return true, []string{
			fmt.Sprintf("Changed %s → %s: per_item %v → %v, rate_per_min %d → %d",
				from, to, oldPerItem, svc.Calls[i].PerItem, oldRpm, svc.Calls[i].RatePerMin),
		}
	}

	return false, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func init() { suggestion.Register(chattyCalls{}) }
