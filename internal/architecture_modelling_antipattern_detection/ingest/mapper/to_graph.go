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

// CanonicalServiceTypeForYAML maps diagram / UI aliases to the `services[].type` strings
// persisted in YAML (same vocabulary as the patterns graph export).
func CanonicalServiceTypeForYAML(raw string) string {
	return normalizeType(raw)
}

// NormalizeYAMLSpecInPlace rewrites services[].type to canonical strings (e.g. user → user_actor).
func NormalizeYAMLSpecInPlace(s *parser.YSpec) {
	if s == nil {
		return
	}
	for i := range s.Services {
		t := strings.TrimSpace(s.Services[i].Type)
		if t == "" {
			s.Services[i].Type = "service"
			continue
		}
		s.Services[i].Type = normalizeType(t)
	}
}

func normalizeType(t string) string {
	t = strings.TrimSpace(strings.ToLower(t))
	switch t {
	case "", "svc", "microservice", "micro-service", "ms", "service":
		return "service"
	case "db", "datastore", "data_store", "data-store", "database":
		return "database"
	case "api_gateway", "api-gateway", "gateway", "bff":
		return "api_gateway"
	case "client":
		return "client"
	case "user_actor", "user-actor", "user", "actor":
		return "user_actor"
	case "event_topic", "event-topic", "topic":
		return "event_topic"
	case "external_system", "external-system", "external":
		return "external_system"
	default:
		return "service"
	}
}

func nodeKindFromNormalizedYAMLType(norm string) domain.NodeKind {
	switch norm {
	case "database":
		return domain.NodeDB
	case "api_gateway":
		return domain.NodeAPIGateway
	case "client":
		return domain.NodeClient
	case "user_actor":
		return domain.NodeUserActor
	case "event_topic":
		return domain.NodeEventTopic
	case "external_system":
		return domain.NodeExternalSystem
	default:
		return domain.NodeService
	}
}

var dbNameLike = regexp.MustCompile(`(^db$)|(^database$)|(\bdb\b)|(\bdatabase\b)|(_db$)|(-db$)`)

func buildDatabaseNameSet(s *parser.YSpec) map[string]bool {
	dbSet := map[string]bool{}
	if s == nil {
		return dbSet
	}
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
		name := strings.ToLower(strings.TrimSpace(StripNodeNameRef(svc.Name)))
		if name == "" {
			continue
		}
		if normalizeType(svc.Type) == "database" {
			dbSet[name] = true
		}
	}
	return dbSet
}

func findServiceByName(s *parser.YSpec, name string) *parser.YService {
	if s == nil {
		return nil
	}
	name = strings.ToLower(strings.TrimSpace(StripNodeNameRef(name)))
	for i := range s.Services {
		n := strings.ToLower(strings.TrimSpace(StripNodeNameRef(s.Services[i].Name)))
		if n == name {
			return &s.Services[i]
		}
	}
	return nil
}

func getServiceType(s *parser.YSpec, svc *parser.YService) string {
	if svc != nil && svc.Type != "" {
		return svc.Type
	}
	return ""
}

func looksLikeDB(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return dbNameLike.MatchString(n)
}

// kindForNode maps a declared service entry (name + yaml type) to a graph node kind.
func kindForNode(name string, serviceType string, dbSet map[string]bool) domain.NodeKind {
	clean := StripNodeNameRef(name)
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
	return nodeKindFromNormalizedYAMLType(normalizeType(serviceType))
}

// kindForReference resolves kind for a dependency/call target that may or may not have a services[] entry.
func kindForReference(s *parser.YSpec, refName string, dbSet map[string]bool) domain.NodeKind {
	clean := StripNodeNameRef(refName)
	if strings.TrimSpace(clean) == "" {
		return domain.NodeService
	}
	if svc := findServiceByName(s, clean); svc != nil {
		return kindForNode(svc.Name, svc.Type, dbSet)
	}
	key := strings.ToLower(strings.TrimSpace(clean))
	if dbSet[key] {
		return domain.NodeDB
	}
	if looksLikeDB(clean) {
		return domain.NodeDB
	}
	return domain.NodeService
}

func ToGraph(s *parser.YSpec) *domain.Graph {
	g := domain.NewGraph()
	if s == nil {
		return g
	}

	dbSet := buildDatabaseNameSet(s)

	if isNewStyle(s) {
		for _, svc := range s.Services {
			name := StripNodeNameRef(svc.Name)
			if strings.TrimSpace(name) == "" {
				continue
			}
			k := kindForNode(name, svc.Type, dbSet)
			_ = ensureNode(g, k, name)
		}
		for _, ds := range s.Datastores {
			name := StripNodeNameRef(ds.Name)
			if strings.TrimSpace(name) == "" {
				continue
			}
			_ = ensureNode(g, domain.NodeDB, name)
		}
		for _, d := range s.Databases {
			name := StripNodeNameRef(d.Name)
			if strings.TrimSpace(name) == "" {
				continue
			}
			_ = ensureNode(g, domain.NodeDB, name)
		}

		for _, dep := range s.Dependencies {
			fromName := StripNodeNameRef(dep.From)
			toName := StripNodeNameRef(dep.To)

			if strings.TrimSpace(fromName) == "" || strings.TrimSpace(toName) == "" {
				continue
			}

			fromSvc := findServiceByName(s, fromName)
			toSvc := findServiceByName(s, toName)
			fromKind := kindForNode(fromName, getServiceType(s, fromSvc), dbSet)
			toKind := kindForNode(toName, getServiceType(s, toSvc), dbSet)
			from := ensureNode(g, fromKind, fromName)
			to := ensureNode(g, toKind, toName)

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
		_ = ensureNode(g, domain.NodeDB, StripNodeNameRef(d.Name))
	}

	for _, ds := range s.Datastores {
		if strings.TrimSpace(ds.Name) == "" {
			continue
		}
		_ = ensureNode(g, domain.NodeDB, StripNodeNameRef(ds.Name))
	}

	for _, svc := range s.Services {
		if strings.TrimSpace(svc.Name) == "" {
			continue
		}
		name := StripNodeNameRef(svc.Name)
		k := kindForNode(name, svc.Type, dbSet)
		_ = ensureNode(g, k, name)
	}

	for _, svc := range s.Services {
		if strings.TrimSpace(svc.Name) == "" {
			continue
		}
		fromName := StripNodeNameRef(svc.Name)
		fromKind := kindForNode(fromName, svc.Type, dbSet)
		from := ensureNode(g, fromKind, fromName)

		for _, c := range svc.Calls {
			if strings.TrimSpace(c.To) == "" {
				continue
			}
			toName := StripNodeNameRef(c.To)
			toKind := kindForReference(s, toName, dbSet)
			to := ensureNode(g, toKind, toName)

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
			dbClean := StripNodeNameRef(db)
			to := ensureNode(g, domain.NodeDB, dbClean)
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
			dbClean := StripNodeNameRef(db)
			to := ensureNode(g, domain.NodeDB, dbClean)
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
				n.Attrs["owner"] = fromName
			}
		}
	}

	return g
}
