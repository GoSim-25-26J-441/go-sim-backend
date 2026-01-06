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
	name := ""
	if len(det.Nodes) >= 1 {
		name = nodeName(g, det.Nodes[0])
	}

	title := "Split the god service"
	if name != "" {
		title = fmt.Sprintf("Split the god service (%s)", name)
	}

	outCalls := 0
	inCalls := 0
	dbTouches := 0

	if g != nil && name != "" {
		for _, e := range g.Edges {
			from := nodeName(g, e.From)
			to := nodeName(g, e.To)

			if e.Kind == domain.EdgeCalls {
				if strings.EqualFold(from, name) {
					outCalls++
				}
				if strings.EqualFold(to, name) {
					inCalls++
				}
			}
			if e.Kind == domain.EdgeReads || e.Kind == domain.EdgeWrites {
				if strings.EqualFold(from, name) {
					dbTouches++
				}
			}
		}
	}

	bullets := []string{
		"This service has too many responsibilities and dependencies (hard to change and scale).",
	}
	if name != "" {
		bullets = append(bullets,
			fmt.Sprintf("%s summary: outgoing calls=%d, incoming calls=%d, DB touches=%d.", name, outCalls, inCalls, dbTouches),
			"Auto-fix preview: we will move about half of "+name+"'s outgoing calls into a new service ("+name+"_split) and add one delegate call "+name+" → "+name+"_split.",
		)
	}

	bullets = append(bullets,
		"Better long-term fix: split by bounded context (Orders / Payments / Shipping style split).",
	)

	return suggestion.Suggestion{
		Kind:    det.Kind,
		Title:   title,
		Bullets: bullets,
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

	if len(mainSvc.Calls) < 2 {
		return false, nil
	}

	newName := mainSvc.Name + "_split"
	splitSvc := ensureService(spec, newName)

	half := len(mainSvc.Calls) / 2
	moved := mainSvc.Calls[half:]
	mainSvc.Calls = mainSvc.Calls[:half]
	splitSvc.Calls = append(splitSvc.Calls, moved...)

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
