package strategies

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type breakCycle struct{}

func (breakCycle) Kind() domain.AntiPatternKind { return domain.APCycles }

func (breakCycle) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	return suggestion.Suggestion{
		Kind:  det.Kind,
		Title: "Break the cycle (remove circular dependencies)",
		Bullets: []string{
			"Services should not call each other in a loop (A → B → C → A).",
			"Pick one link and change it to async events (queue/pub-sub) OR remove the direct call.",
			"If two services always depend on each other, consider merging them or extracting shared logic.",
		},
	}
}

func (breakCycle) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	// Heuristic: remove the first call edge we can find among the cycle nodes
	if len(det.Nodes) < 2 {
		return false, nil
	}

	for _, fromID := range det.Nodes {
		fromName := nodeName(g, fromID)
		svc := findService(spec, fromName)
		if svc == nil {
			continue
		}

		for _, toID := range det.Nodes {
			if toID == fromID {
				continue
			}
			toName := nodeName(g, toID)

			// remove call from fromName -> toName if exists
			changed := false
			newCalls := make([]parser.YCall, 0, len(svc.Calls))
			for _, c := range svc.Calls {
				if equalFold(c.To, toName) && !changed {
					changed = true
					continue
				}
				newCalls = append(newCalls, c)
			}
			if changed {
				svc.Calls = newCalls
				return true, []string{"Removed one call link to break the cycle: " + fromName + " → " + toName}
			}
		}
	}

	return false, nil
}

func equalFold(a, b string) bool { return len(a) == len(b) && (a == b || (a != "" && b != "" && stringsEqualFold(a, b))) }

// tiny wrapper to avoid importing strings in multiple strategy files
func stringsEqualFold(a, b string) bool {
	// local minimal equal-fold
	if a == b {
		return true
	}
	// fallback: use case-insensitive compare via byte lowering (ASCII only is fine for service names)
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		aa := a[i]
		bb := b[i]
		if aa >= 'A' && aa <= 'Z' {
			aa = aa - 'A' + 'a'
		}
		if bb >= 'A' && bb <= 'Z' {
			bb = bb - 'A' + 'a'
		}
		if aa != bb {
			return false
		}
	}
	return true
}

func init() { suggestion.Register(breakCycle{}) }
