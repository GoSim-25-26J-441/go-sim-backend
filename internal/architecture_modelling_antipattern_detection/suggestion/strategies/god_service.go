package strategies

import (
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type godService struct{}

func (godService) Kind() domain.AntiPatternKind { return domain.APGodService }

func (godService) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	main := ""
	if len(det.Nodes) > 0 {
		main = det.Nodes[0]
	}

	title := "Split god service"
	if main != "" {
		title = fmt.Sprintf("Split god service (%s)", main)
	}

	bullets := []string{
		"One service has too many dependencies (high centrality).",
		"Fix: split responsibilities into smaller services.",
		"Auto-fix: move some outgoing dependencies to a new service and add a delegate edge.",
	}

	return suggestion.Suggestion{Kind: det.Kind, Title: title, Bullets: bullets}
}

func (godService) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if spec == nil || len(det.Nodes) < 1 {
		return false, nil
	}
	main := det.Nodes[0]

	var outIdx []int
	for i := range spec.Dependencies {
		if spec.Dependencies[i].From == "" || spec.Dependencies[i].To == "" {
			continue
		}
		if stringsEqualFold(spec.Dependencies[i].From, main) {
			outIdx = append(outIdx, i)
		}
	}
	if len(outIdx) < 2 {
		return false, nil
	}

	newName := uniqueServiceName(spec, main+"_split")
	ensureService(spec, newName)

	half := len(outIdx) / 2
	if half == 0 {
		half = 1
	}

	moved := 0
	for j := half; j < len(outIdx); j++ {
		i := outIdx[j]
		spec.Dependencies[i].From = newName
		moved++
	}

	_, note := addDependencyIfMissing(spec, parser.YDependency{
		From: main,
		To:   newName,
		Kind: "rest",
		Sync: true,
	})

	notes := []string{fmt.Sprintf("Moved %d outgoing dependencies from %s to %s.", moved, main, newName)}
	if note != "" {
		notes = append(notes, note)
	}

	return true, notes
}

func stringsEqualFold(a, b string) bool {
	if a == b {
		return true
	}
	if len(a) != len(b) {
		return strings.EqualFold(a, b)
	}
	return strings.EqualFold(a, b)
}

func init() { suggestion.Register(godService{}) }
