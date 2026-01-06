package rules

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/detection"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
)

type sharedWrite struct{}
func (s sharedWrite) Name() string { return "shared_db_writes" }

func (s sharedWrite) Detect(g *domain.Graph) ([]domain.Detection, error) {
	writers := map[string][]string{}
	for _, e := range g.Edges {
		if e.Kind == domain.EdgeWrites {
			writers[e.To] = append(writers[e.To], e.From)
		}
	}
	var out []domain.Detection
	for db, ws := range writers {
		if len(ws) > 1 {
			out = append(out, domain.Detection{
				Kind: domain.APSharedDBWrites, Severity: domain.SeverityHigh,
				Title: "Multiple writers to DB",
				Summary: "More than one service writes to same database",
				Nodes: append([]string{db}, ws...),
				Evidence: domain.Attrs{"writers": ws},
			})
		}
	}
	return out, nil
}

func init(){ detection.Register(sharedWrite{}) }
