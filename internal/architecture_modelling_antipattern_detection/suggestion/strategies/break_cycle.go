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
	names := namesFromNodeIDs(g, det.Nodes)

	// Build a readable loop: A → B → C → A
	loop := ""
	if len(names) >= 2 {
		loop = strings.Join(names, " → ") + " → " + names[0]
	}

	// Preview what Apply() is likely to remove: first CALL edge we can find
	removeFrom := ""
	removeTo := ""
	if len(det.Nodes) >= 2 {
		for _, fromID := range det.Nodes {
			fromName := nodeName(g, fromID)
			for _, toID := range det.Nodes {
				if toID == fromID {
					continue
				}
				toName := nodeName(g, toID)

				// If a CALL edge exists in the graph, that's a good preview
				if _, _, ok := findCallEdgeBetween(g, fromName, toName); ok {
					removeFrom = fromName
					removeTo = toName
					break
				}
			}
			if removeFrom != "" {
				break
			}
		}
	}

	title := "Break the cycle (remove circular dependencies)"
	if loop != "" {
		title = "Break cycle: " + loop
	}

	bullets := []string{}
	if loop != "" {
		bullets = append(bullets, "Detected circular dependency: "+loop)
	} else if len(names) > 0 {
		bullets = append(bullets, "Detected a circular dependency among: "+joinNice(names))
	}

	bullets = append(bullets,
		"Why it matters: if one service slows down/fails, the entire loop can cascade and block requests.",
	)

	if removeFrom != "" && removeTo != "" {
		bullets = append(bullets,
			"Auto-fix preview: we will remove the direct call link "+removeFrom+" → "+removeTo+" to break the cycle.",
			"Recommended replacement: publish an event from "+removeFrom+" (queue/pub-sub) and let "+removeTo+" consume it asynchronously.",
		)
	} else {
		bullets = append(bullets,
			"Auto-fix preview: we will remove one call link inside the loop to break the cycle (then re-run detection).",
		)
	}

	return suggestion.Suggestion{
		Kind:    det.Kind,
		Title:   title,
		Bullets: bullets,
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
