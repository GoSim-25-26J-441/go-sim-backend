package strategies

import (
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type crossDBRead struct{}

func (crossDBRead) Kind() domain.AntiPatternKind { return domain.APCrossDBRead }

func (crossDBRead) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	return suggestion.Suggestion{
		Kind:  det.Kind,
		Title: "Do not read another service’s database",
		Bullets: []string{
			"This service reads a database owned by another service.",
			"Instead, call the owning service API (or consume its events).",
			"Keep database ownership per service to avoid tight coupling.",
		},
	}
}

func (crossDBRead) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	owner, _ := det.Evidence["owner"].(string)
	reader, _ := det.Evidence["reader"].(string)
	if owner == "" || reader == "" || len(det.Nodes) < 2 {
		return false, nil
	}

	// nodes: [service, db] (from rule it adds {e.From, e.To})
	dbName := nodeName(g, det.Nodes[1])

	readerSvc := findService(spec, reader)
	if readerSvc == nil {
		return false, nil
	}

	// remove direct DB read
	var removed bool
	readerSvc.Databases.Reads, removed = removeString(readerSvc.Databases.Reads, dbName)
	if !removed {
		return false, nil
	}

	// add a call to the owner service as a replacement
	call := parser.YCall{
		To:         owner,
		Endpoints:  []string{fmt.Sprintf("GET /%s/read", dbName)},
		RatePerMin: 60,
		PerItem:    false,
	}
	if !hasCallTo(readerSvc, owner) {
		readerSvc.Calls = append(readerSvc.Calls, call)
	}

	return true, []string{
		fmt.Sprintf("Removed cross-DB read (%s reads %s). Added API call from %s → %s instead.", reader, dbName, reader, owner),
	}
}

func hasCallTo(svc *parser.YService, to string) bool {
	for _, c := range svc.Calls {
		if stringsEqualFold(c.To, to) {
			return true
		}
	}
	return false
}

func init() { suggestion.Register(crossDBRead{}) }
