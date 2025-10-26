package mapper

import (
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattaren_detection/ingest/parser"
)

func idify(kind domain.NodeKind, name string) string {
	return fmt.Sprintf("%s:%s", kind, strings.ToLower(name))
}

func ToGraph(s *parser.YSpec) *domain.Graph {
	g := domain.NewGraph()

	// DB nodes
	for _, d := range s.Databases {
		g.AddNode(&domain.Node{
			ID:   idify(domain.NodeDB, d.Name),
			Name: d.Name, Kind: domain.NodeDB,
		})
	}

	// Service nodes
	for _, svc := range s.Services {
		g.AddNode(&domain.Node{
			ID: idify(domain.NodeService, svc.Name),
			Name: svc.Name, Kind: domain.NodeService,
		})
	}

	// Service edges (calls)
	for _, svc := range s.Services {
		from := idify(domain.NodeService, svc.Name)
		for _, c := range svc.Calls {
			to := idify(domain.NodeService, c.To)
			g.AddEdge(&domain.Edge{
				From: from, To: to, Kind: domain.EdgeCalls,
				Attrs: domain.Attrs{
					"endpoints":     c.Endpoints,
					"rate_per_min":  c.RatePerMin,
					"per_item":      c.PerItem,
					"count":         len(c.Endpoints),
				},
			})
		}
		// DB reads
		for _, db := range svc.Databases.Reads {
			to := idify(domain.NodeDB, db)
			g.AddEdge(&domain.Edge{
				From: from, To: to, Kind: domain.EdgeReads,
			})
		}
		// DB writes
		for _, db := range svc.Databases.Writes {
			to := idify(domain.NodeDB, db)
			g.AddEdge(&domain.Edge{
				From: from, To: to, Kind: domain.EdgeWrites,
				Attrs: domain.Attrs{"owner": true},
			})
			// mark DB node owner=true for convenience
			if n, ok := g.Nodes[to]; ok {
				if n.Attrs == nil { n.Attrs = domain.Attrs{} }
				n.Attrs["owner"] = svc.Name
			}
		}
	}
	return g
}
