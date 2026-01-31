package mapper

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/domain"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/architecture_modelling_antipattern_detection/ingest/parser"
)

func idify(kind domain.NodeKind, name string) string {
	return fmt.Sprintf("%s:%s", kind, strings.ToLower(strings.TrimSpace(name)))
}

func ensureNode(g *domain.Graph, kind domain.NodeKind, name string) string {
	name = strings.TrimSpace(name)
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

func isNewStyle(s *parser.YSpec) bool {
	return s != nil && len(s.Dependencies) > 0
}

func stripEntityPrefix(s string) string {
	t := strings.TrimSpace(s)
	up := strings.ToUpper(t)

	if strings.HasPrefix(up, "SERVICE:") {
		return strings.TrimSpace(t[len("SERVICE:"):])
	}
	if strings.HasPrefix(up, "DATABASE:") {
		return strings.TrimSpace(t[len("DATABASE:"):])
	}

	noSpaceUp := strings.ReplaceAll(up, " ", "")
	noSpaceRaw := strings.ReplaceAll(t, " ", "")
	if strings.HasPrefix(noSpaceUp, "SERVICE:") {
		return strings.TrimSpace(noSpaceRaw[len("SERVICE:"):])
	}
	if strings.HasPrefix(noSpaceUp, "DATABASE:") {
		return strings.TrimSpace(noSpaceRaw[len("DATABASE:"):])
	}

	return t
}

func normalizeType(t string) string {
	t = strings.TrimSpace(strings.ToLower(t))
	switch t {
	case "db", "datastore", "data_store", "data-store", "database":
		return "database"
	default:
		if t == "" {
			return "service"
		}
		return "service"
	}
}

var dbNameLike = regexp.MustCompile(`(^db$)|(^database$)|(\bdb\b)|(\bdatabase\b)|(_db$)|(-db$)`)

func looksLikeDB(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return dbNameLike.MatchString(n)
}

func ToGraph(s *parser.YSpec) *domain.Graph {
	g := domain.NewGraph()
	if s == nil {
		return g
	}


	if isNewStyle(s) {

		dbSet := map[string]bool{}

		for _, ds := range s.Datastores {
			n := strings.ToLower(strings.TrimSpace(ds.Name))
			if n != "" {
				dbSet[n] = true
			}
		}

		for _, d := range s.Databases {
			n := strings.ToLower(strings.TrimSpace(d.Name))
			if n != "" {
				dbSet[n] = true
			}
		}

		for _, svc := range s.Services {
			name := stripEntityPrefix(svc.Name)
			if strings.TrimSpace(name) == "" {
				continue
			}

			typ := normalizeType(svc.Type)
			if typ == "database" {
				dbSet[strings.ToLower(strings.TrimSpace(name))] = true
			}
		}

		kindFor := func(name string) domain.NodeKind {
			clean := stripEntityPrefix(name)
			key := strings.ToLower(strings.TrimSpace(clean))
			if key == "" {
				return domain.NodeService
			}
			if dbSet[key] {
				return domain.NodeDB
			}

			if looksLikeDB(clean) {
				return domain.NodeDB
			}
			return domain.NodeService
		}

		for _, svc := range s.Services {
			name := stripEntityPrefix(svc.Name)
			if strings.TrimSpace(name) == "" {
				continue
			}
			_ = ensureNode(g, kindFor(name), name)
		}
		for _, ds := range s.Datastores {
			name := stripEntityPrefix(ds.Name)
			if strings.TrimSpace(name) == "" {
				continue
			}
			_ = ensureNode(g, domain.NodeDB, name)
		}
		for _, d := range s.Databases {
			name := stripEntityPrefix(d.Name)
			if strings.TrimSpace(name) == "" {
				continue
			}
			_ = ensureNode(g, domain.NodeDB, name)
		}

		for _, dep := range s.Dependencies {
			fromName := stripEntityPrefix(dep.From)
			toName := stripEntityPrefix(dep.To)

			if strings.TrimSpace(fromName) == "" || strings.TrimSpace(toName) == "" {
				continue
			}

			from := ensureNode(g, kindFor(fromName), fromName)
			to := ensureNode(g, kindFor(toName), toName)

			attrs := domain.Attrs{
				"sync":     dep.Sync,
				"dep_kind": strings.ToLower(strings.TrimSpace(dep.Kind)),
			}

			g.AddEdge(&domain.Edge{
				From:  from,
				To:    to,
				Kind:  domain.EdgeCalls,
				Attrs: attrs,
			})
		}

		return g
	}

	for _, d := range s.Databases {
		if strings.TrimSpace(d.Name) == "" {
			continue
		}
		_ = ensureNode(g, domain.NodeDB, d.Name)
	}


	for _, ds := range s.Datastores {
		if strings.TrimSpace(ds.Name) == "" {
			continue
		}
		_ = ensureNode(g, domain.NodeDB, ds.Name)
	}


	for _, svc := range s.Services {
		if strings.TrimSpace(svc.Name) == "" {
			continue
		}
		_ = ensureNode(g, domain.NodeService, svc.Name)
	}

	for _, svc := range s.Services {
		if strings.TrimSpace(svc.Name) == "" {
			continue
		}
		from := ensureNode(g, domain.NodeService, svc.Name)

		for _, c := range svc.Calls {
			if strings.TrimSpace(c.To) == "" {
				continue
			}
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
					"sync":         true,
				},
			})
		}

		for _, db := range svc.Databases.Reads {
			if strings.TrimSpace(db) == "" {
				continue
			}
			to := ensureNode(g, domain.NodeDB, db)
			g.AddEdge(&domain.Edge{
				From: from,
				To:   to,
				Kind: domain.EdgeReads,
			})
		}

		for _, db := range svc.Databases.Writes {
			if strings.TrimSpace(db) == "" {
				continue
			}
			to := ensureNode(g, domain.NodeDB, db)
			g.AddEdge(&domain.Edge{
				From: from,
				To:   to,
				Kind: domain.EdgeWrites,
				Attrs: domain.Attrs{
					"owner": true,
				},
			})

			if n, ok := g.Nodes[to]; ok && n != nil {
				if n.Attrs == nil {
					n.Attrs = domain.Attrs{}
				}
				n.Attrs["owner"] = svc.Name
			}
		}
	}

	return g
}
