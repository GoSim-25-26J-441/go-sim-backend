package strategies

import (
	"fmt"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/suggestion"
)

type syncCallChain struct{}

func (syncCallChain) Kind() domain.AntiPatternKind { return domain.APSyncCallChain }

func (syncCallChain) Suggest(g *domain.Graph, det domain.Detection) suggestion.Suggestion {
	title := "Shorten sync call chain"
	bullets := []string{
		"Long synchronous chains increase latency and failure blast radius.",
		"Fix: make one hop async (sync=false) or add a BFF/cache.",
		"Auto-fix: set sync=false on a middle hop.",
	}
	return suggestion.Suggestion{Kind: det.Kind, Title: title, Bullets: bullets}
}

func (syncCallChain) Apply(spec *parser.YSpec, g *domain.Graph, det domain.Detection) (bool, []string) {
	if spec == nil || len(det.Nodes) < 2 {
		return false, nil
	}

	mid := (len(det.Nodes) - 2) / 2
	if mid < 0 {
		mid = 0
	}
	from := det.Nodes[mid]
	to := det.Nodes[mid+1]

	if ok, note := setDependencySync(spec, from, to, false); ok {
		return true, []string{note, fmt.Sprintf("Converted one hop to async: %s → %s", from, to)}
	}
	return false, nil
}

func init() { suggestion.Register(syncCallChain{}) }
