package strategies

import (
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type sharedDatabase struct{}

func (sharedDatabase) Kind() domain.AntiPatternKind { return domain.APSharedDatabase }

func (sharedDatabase) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	title := "Reduce shared database"
	bullets := []string{
		"Multiple services depend on the same database component.",
		"Fix: split DB per bounded context or access via one owning service.",
		"Auto-fix: create per-service DB nodes and retarget edges.",
	}
	return suggestion.Suggestion{Kind: det.Kind, Title: title, Bullets: bullets}
}

func (sharedDatabase) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if spec == nil || len(det.Nodes) < 3 {
		return false, nil
	}

	db := ""
	var clients []string
	for _, n := range det.Nodes {
		if db == "" && isDatabaseLikeName(n) {
			db = n
		} else {
			clients = append(clients, n)
		}
	}

	if db == "" {
		db = det.Nodes[0]
		clients = det.Nodes[1:]
	}
	if db == "" || len(clients) < 2 {
		return false, nil
	}

	changed := false
	var notes []string

	for _, c := range clients {
		if findDepIndex(spec, c, db) < 0 {
			continue
		}
		newDB := uniqueServiceName(spec, strings.ToLower(strings.TrimSpace(c))+"-db")
		ensureService(spec, newDB)

		if ok, note := retargetDependency(spec, c, db, newDB); ok {
			changed = true
			notes = append(notes, note)
		}
	}

	if changed {
		notes = append(notes, fmt.Sprintf("Split shared DB %s into per-service DB nodes.", db))
	}
	return changed, notes
}

func init() { suggestion.Register(sharedDatabase{}) }
