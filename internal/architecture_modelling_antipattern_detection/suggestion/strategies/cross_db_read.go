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
	owner, _ := det.Evidence["owner"].(string)
	reader, _ := det.Evidence["reader"].(string)

	dbName := ""
	if len(det.Nodes) >= 2 {
		dbName = nodeName(g, det.Nodes[1])
	}

	title := "Do not read another service’s database"
	if reader != "" && owner != "" && dbName != "" {
		title = fmt.Sprintf("Replace %s reading %s (owned by %s)", reader, dbName, owner)
	}

	bullets := []string{}
	if reader != "" && owner != "" && dbName != "" {
		bullets = append(bullets,
			fmt.Sprintf("Detected cross-DB read: %s reads %s (owned by %s).", reader, dbName, owner),
			"Why it matters: schema changes in "+owner+" can break "+reader+" instantly (tight coupling).",
			fmt.Sprintf("Auto-fix preview: remove '%s' from %s.databases.reads and add API call %s → %s (GET /%s/read).", dbName, reader, reader, owner, dbName),
		)
	} else {
		bullets = append(bullets,
			"This service reads a database owned by another service.",
			"Auto-fix preview: remove the direct DB read and replace it with a call to the owning service.",
		)
	}

	bullets = append(bullets,
		"Alternative: consume "+owner+" events to build a local read model (CQRS) if you need fast reads.",
	)

	return suggestion.Suggestion{
		Kind:    det.Kind,
		Title:   title,
		Bullets: bullets,
	}
}


func (crossDBRead) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	owner, _ := det.Evidence["owner"].(string)
	reader, _ := det.Evidence["reader"].(string)
	if owner == "" || reader == "" || len(det.Nodes) < 2 {
		return false, nil
	}

	dbName := nodeName(g, det.Nodes[1])

	readerSvc := findService(spec, reader)
	if readerSvc == nil {
		return false, nil
	}

	var removed bool
	readerSvc.Databases.Reads, removed = removeString(readerSvc.Databases.Reads, dbName)
	if !removed {
		return false, nil
	}

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
