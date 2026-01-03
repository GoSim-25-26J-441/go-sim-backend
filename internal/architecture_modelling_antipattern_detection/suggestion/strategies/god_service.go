package strategies

import (
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type godService struct{}

func (godService) Kind() domain.AntiPatternKind { return domain.APGodService }

func (godService) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	name := ""
	if len(det.Nodes) >= 1 {
		name = nodeName(g, det.Nodes[0])
	}
	title := "Split the god service"
	if name != "" {
		title = fmt.Sprintf("Split the god service (%s)", name)
	}
	return suggestion.Suggestion{
		Kind:  det.Kind,
		Title: title,
		Bullets: []string{
			"This service has too many dependencies (it is doing too much).",
			"Split it into smaller services by feature (example: Orders → Orders + Payments + Shipping).",
			"Keep one facade/API and move internal work to other services.",
		},
	}
}

func (godService) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if len(det.Nodes) < 1 {
		return false, nil
	}
	mainName := nodeName(g, det.Nodes[0])
	mainSvc := findService(spec, mainName)
	if mainSvc == nil {
		return false, nil
	}

	// heuristic: split HALF of outgoing calls into a new service and add a single delegate call
	if len(mainSvc.Calls) < 2 {
		return false, nil
	}

	newName := mainSvc.Name + "_split"
	splitSvc := ensureService(spec, newName)

	half := len(mainSvc.Calls) / 2
	moved := mainSvc.Calls[half:]
	mainSvc.Calls = mainSvc.Calls[:half]
	splitSvc.Calls = append(splitSvc.Calls, moved...)

	// add one delegate call from main -> split (reduces edge fan-out)
	mainSvc.Calls = append(mainSvc.Calls, parser.YCall{
		To:         newName,
		Endpoints:  []string{"POST /delegate"},
		RatePerMin: 30,
		PerItem:    false,
	})

	return true, []string{
		fmt.Sprintf("Split %s into %s (moved %d calls). Added delegate call %s → %s.", mainSvc.Name, newName, len(moved), mainSvc.Name, newName),
	}
}

func init() { suggestion.Register(godService{}) }
