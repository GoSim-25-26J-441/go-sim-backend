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

	// Pull details if available
	rpm, eps, ok := findCallEdgeBetween(g, from, to)

	bullets := []string{}
	if from != "" && to != "" {
		if ok {
			bullets = append(bullets,
				fmt.Sprintf("Detected frequent small calls from %s → %s (%d endpoints, ~%d rpm).", from, to, eps, rpm),
			)
		} else {
			bullets = append(bullets,
				fmt.Sprintf("Detected frequent small calls from %s → %s.", from, to),
			)
		}
	}

	bullets = append(bullets,
		"Fix idea: batch requests (fewer, larger requests) instead of calling per item in a loop.",
		"Fix idea: cache results where possible so the same data isn’t requested repeatedly.",
		"Auto-fix preview: we will disable per-item calling and reduce rate_per_min for "+from+" → "+to+" (then re-run detection).",
	)

	return suggestion.Suggestion{
		Kind:    det.Kind,
		Title:   title,
		Bullets: bullets,
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
