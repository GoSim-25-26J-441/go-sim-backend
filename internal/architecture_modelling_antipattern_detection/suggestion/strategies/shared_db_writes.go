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
	dbName := ""
	if len(det.Nodes) >= 1 {
		dbName = nodeName(g, det.Nodes[0])
	}

	writers := []string{}
	if len(det.Nodes) >= 2 {
		for _, w := range det.Nodes[1:] {
			writers = append(writers, nodeName(g, w))
		}
	}

	title := "Give each service its own database"
	if dbName != "" && len(writers) > 0 {
		title = fmt.Sprintf("Stop multiple writers to %s (%d writers)", dbName, len(writers))
	}

	bullets := []string{}
	if dbName != "" && len(writers) > 0 {
		bullets = append(bullets,
			fmt.Sprintf("Detected shared DB writes: %s is written by %s.", dbName, joinNice(writers)),
			"Why it matters: multi-writes can cause data corruption, conflicting transactions, and unclear ownership.",
			"Auto-fix preview: create a dedicated DB per writer and move writes away from "+dbName+".",
		)
		// preview names
		for _, w := range writers {
			bullets = append(bullets, fmt.Sprintf("• %s will write to: %s_%s", w, dbName, w))
		}
	} else {
		bullets = append(bullets,
			"More than one service is writing to the same database.",
			"Auto-fix preview: create dedicated DBs and move writes per service.",
		)
	}

	bullets = append(bullets,
		"Recommended: share data via APIs/events instead of allowing many services to write to one DB.",
	)

	return suggestion.Suggestion{
		Kind:    det.Kind,
		Title:   title,
		Bullets: bullets,
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
