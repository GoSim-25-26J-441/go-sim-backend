package mapper

import (
	"fmt"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

func idify(kind domain.NodeKind, name string) string {
	return fmt.Sprintf("%s:%s", kind, strings.ToLower(name))
}

func ensureNode(g *domain.Graph, kind domain.NodeKind, name string) string {
	id := idify(kind, name)
	if _, ok := g.Nodes[id]; !ok {
		g.AddNode(&domain.Node{
			ID:   id,
			Name: name,
			Kind: kind,
		})
	}
	return id
}

func ToGraph(s *parser.YSpec) *domain.Graph {
	g := domain.NewGraph()


	for _, d := range s.Databases {
		_ = ensureNode(g, domain.NodeDB, d.Name)
	}


	for _, svc := range s.Services {
		_ = ensureNode(g, domain.NodeService, svc.Name)
	}


	for _, svc := range s.Services {
		from := ensureNode(g, domain.NodeService, svc.Name)


		for _, c := range svc.Calls {
			to := ensureNode(g, domain.NodeService, c.To)
			g.AddEdge(&domain.Edge{
				From: from,
				To:   to,
				Kind: domain.EdgeCalls,
				Attrs: domain.Attrs{
					"endpoints":    c.Endpoints,
					"rate_per_min": c.RatePerMin,
					"per_item":     c.PerItem,
					"count":        len(c.Endpoints),
				},
			})
		}

		for _, db := range svc.Databases.Reads {
			to := ensureNode(g, domain.NodeDB, db)
			g.AddEdge(&domain.Edge{
				From: from,
				To:   to,
				Kind: domain.EdgeReads,
			})
		}


		for _, db := range svc.Databases.Writes {
			to := ensureNode(g, domain.NodeDB, db)
			g.AddEdge(&domain.Edge{
				From: from,
				To:   to,
				Kind: domain.EdgeWrites,
				Attrs: domain.Attrs{
					"owner": true,
				},
			})

			if n, ok := g.Nodes[to]; ok {
				if n.Attrs == nil {
					n.Attrs = domain.Attrs{}
				}
				n.Attrs["owner"] = svc.Name
			}
		}
	}

	return g
}
