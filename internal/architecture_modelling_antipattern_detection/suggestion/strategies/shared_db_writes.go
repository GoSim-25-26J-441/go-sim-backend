package strategies

import (
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type sharedDBWrites struct{}

func (sharedDBWrites) Kind() domain.AntiPatternKind { return domain.APSharedDBWrites }

func (sharedDBWrites) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	return suggestion.Suggestion{
		Kind:  det.Kind,
		Title: "Give each service its own database",
		Bullets: []string{
			"More than one service is writing to the same database.",
			"Give each service its own database (database ownership per service).",
			"Share data via APIs/events instead of direct multi-writes to one DB.",
		},
	}
}

func (sharedDBWrites) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	// Detection nodes: [db, writer1, writer2, ...] (writer IDs)
	if len(det.Nodes) < 3 {
		return false, nil
	}

	dbName := nodeName(g, det.Nodes[0])
	changed := false
	var notes []string

	// For each writer, create a dedicated DB and move writes to it
	for _, writerID := range det.Nodes[1:] {
		writer := nodeName(g, writerID)
		svc := findService(spec, writer)
		if svc == nil {
			continue
		}
		newDB := fmt.Sprintf("%s_%s", dbName, writer)

		ensureDB(spec, newDB)

		// remove old db from writes, add new db to writes
		var removed bool
		svc.Databases.Writes, removed = removeString(svc.Databases.Writes, dbName)
		if removed {
			svc.Databases.Writes = appendIfMissingFold(svc.Databases.Writes, newDB)
			changed = true
			notes = append(notes, fmt.Sprintf("Moved writer %s from DB %s to its own DB %s", writer, dbName, newDB))
		}
	}

	return changed, notes
}

func appendIfMissingFold(xs []string, v string) []string {
	for _, x := range xs {
		if stringsEqualFold(x, v) {
			return xs
		}
	}
	return append(xs, v)
}

func init() { suggestion.Register(sharedDBWrites{}) }
